package hris

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

// EPIC-095: menjalankan resign terjadwal sehari setelah hari terakhir kerja.

// ScheduledResignEvery is how often the background loop looks for due schedules.
const ScheduledResignEvery = 5 * time.Minute

// scheduledResignActor is the audit actor of a resign applied by the job.
var scheduledResignActor = &auth.User{FullName: "Sistem (resign terjadwal)"}

// RunScheduledResigns applies every pending scheduled resign whose effective
// date (the last working day) is before today (WIB) at now, each in its own
// transaction. A schedule claimed by another worker is skipped (SKIP LOCKED);
// an employee already inactive gets the schedule cancelled (R7). A failing
// schedule is logged and does not stop the others; the first error is
// returned after all were tried.
func (s *Service) RunScheduledResigns(ctx context.Context, now time.Time) (applied, cancelled int, err error) {
	today := domain.TodayWIB(now)
	rows, err := s.repo.querier().Query(ctx, `SELECT id::text FROM hris.scheduled_resignations
		WHERE applied_at IS NULL AND cancelled_at IS NULL AND effective_date < $1::date
		ORDER BY effective_date, id`, today)
	if err != nil {
		return 0, 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	var firstErr error
	for _, id := range ids {
		outcome := ""
		e := s.repo.InTx(ctx, func(r Repository) error {
			outcome = ""
			q := r.querier()
			var emp, date, typ string
			var reason, createdBy *string
			err := q.QueryRow(ctx, `SELECT employee_id::text, effective_date::text, resign_type, reason, created_by::text
				FROM hris.scheduled_resignations
				WHERE id = $1 AND applied_at IS NULL AND cancelled_at IS NULL
				FOR UPDATE SKIP LOCKED`, id).Scan(&emp, &date, &typ, &reason, &createdBy)
			if database.IsNoRows(err) {
				return nil
			}
			if err != nil {
				return err
			}
			var active bool
			if err := q.QueryRow(ctx, `SELECT is_active FROM hris.employees WHERE id = $1 FOR UPDATE`, emp).Scan(&active); err != nil {
				return err
			}
			if !active {
				const why = "Karyawan sudah nonaktif"
				if _, err := q.Exec(ctx, `UPDATE hris.scheduled_resignations
					SET cancelled_at = now(), cancel_reason = $2 WHERE id = $1`, id, why); err != nil {
					return err
				}
				if err := auditJobResignCancel(ctx, q, emp, date, typ, why); err != nil {
					return err
				}
				outcome = "cancelled"
				return nil
			}
			// applied_at first: the deactivation trigger only cancels schedules
			// still pending, so it leaves the one being applied alone.
			if _, err := q.Exec(ctx, `UPDATE hris.scheduled_resignations SET applied_at = now() WHERE id = $1`, id); err != nil {
				return err
			}
			_, err = s.applyResign(ctx, q, scheduledResignActor, createdBy, emp,
				resignInput{EffectiveDate: date, Type: typ, Reason: reason})
			var he *httpx.Error
			if errors.As(err, &he) && he.Status < http.StatusInternalServerError {
				// Validation refused it before anything was written (review I2):
				// cancel with that message instead of retrying every tick.
				if _, err := q.Exec(ctx, `UPDATE hris.scheduled_resignations
					SET applied_at = NULL, cancelled_at = now(), cancel_reason = $2 WHERE id = $1`, id, he.Message); err != nil {
					return err
				}
				if err := auditJobResignCancel(ctx, q, emp, date, typ, he.Message); err != nil {
					return err
				}
				outcome = "cancelled"
				return nil
			}
			if err != nil {
				return err
			}
			outcome = "applied"
			return nil
		})
		switch {
		case e != nil:
			s.log.Error("hris resign terjadwal gagal", "schedule", id, "error", e)
			if firstErr == nil {
				firstErr = e
			}
		case outcome == "applied":
			applied++
		case outcome == "cancelled":
			cancelled++
		}
	}
	return applied, cancelled, firstErr
}

// ScheduledResignLoop runs RunScheduledResigns once at start and then every
// interval until ctx ends. Errors and panics are logged; the loop never stops
// because of them.
func (s *Service) ScheduledResignLoop(ctx context.Context, every time.Duration) {
	s.scheduledResignTick(ctx)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scheduledResignTick(ctx)
		}
	}
}

func (s *Service) scheduledResignTick(ctx context.Context) {
	defer func() {
		if p := recover(); p != nil {
			s.log.Error("hris resign terjadwal panicked", "panic", fmt.Sprint(p))
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	applied, cancelled, err := s.RunScheduledResigns(ctx, s.now())
	if err != nil {
		s.log.Error("hris resign terjadwal failed", "error", err)
	}
	if applied > 0 || cancelled > 0 {
		s.log.Info("hris resign terjadwal", "applied", applied, "cancelled", cancelled)
	}
}

// auditJobResignCancel records a cancellation by the job (EPIC-105 R4) with
// the system actor, like the manual "Batalkan jadwal" does for HR. The
// label is the employee's name, or the NIP when the name is empty.
func auditJobResignCancel(ctx context.Context, q database.Querier, employeeID, effectiveDate, resignType, reason string) error {
	var label string
	if err := q.QueryRow(ctx, `SELECT coalesce(nullif(btrim(full_name), ''), nip, '') FROM hris.employees WHERE id = $1`,
		employeeID).Scan(&label); err != nil {
		return err
	}
	return writeEmployeeAudit(ctx, q, scheduledResignActor, "employee.resign_schedule_cancel", employeeID, label,
		map[string]any{"effective_date": effectiveDate, "resign_type": resignType},
		map[string]any{"cancel_reason": reason})
}
