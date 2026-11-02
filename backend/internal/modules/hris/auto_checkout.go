package hris

import (
	"context"
	"fmt"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
)

// Checkout otomatis di jam selesai shift (EPIC-080 R1, R3, R4, R5).

const (
	// AutoCheckoutEvery is how often the background loop closes open
	// attendance rows whose shift has ended.
	AutoCheckoutEvery = 5 * time.Minute
	// autoCheckoutLookbackDays is how many WIB days back a still-open row is
	// still closed (an overnight shift is dated the day it started).
	autoCheckoutLookbackDays = 2
	autoCheckoutActor        = "Sistem (checkout otomatis)"
	autoCheckoutAction       = "attendance.auto_checkout"
	attendanceEntity         = "hris.attendance"
)

// autoCheckoutSQL closes every open row whose employee holds a job title with
// an active rule (all locations, or the row's own branch) once scheduled_end
// has passed. A row the HR adjusted at or after scheduled_end (for example
// reopened by clearing clock_out) is left alone. Rows of a closed attendance period are left out with the same
// predicate as hris.attendance_closed_period_guard, so one closed row cannot
// abort the whole statement. A clock-in at or after scheduled_end is left for
// the HR to review (R3).
const autoCheckoutSQL = `UPDATE hris.attendance a
	SET clock_out = a.scheduled_end,
	    clock_out_auto = true,
	    clock_out_geofence = 'unchecked',
	    notes = concat_ws(' · ', nullif(a.notes, ''),
	      'Checkout otomatis pukul ' || to_char(a.scheduled_end AT TIME ZONE 'Asia/Jakarta', 'HH24.MI') || ' (jam selesai shift)')
	FROM hris.employees e
	WHERE e.id = a.employee_id
	  AND a.clock_out IS NULL
	  AND a.scheduled_end IS NOT NULL
	  AND a.scheduled_end <= $1::timestamptz
	  AND a.clock_in < a.scheduled_end
	  AND (a.adjusted_at IS NULL OR a.adjusted_at < a.scheduled_end)
	  AND a.date >= $2::date
	  AND EXISTS (SELECT 1 FROM hris.auto_checkout_rules r
	               WHERE r.is_active
	                 AND r.position_id = e.job_title_id
	                 AND (r.branch_id IS NULL OR r.branch_id = a.branch_id))
	  AND NOT EXISTS (SELECT 1 FROM hris.attendance_periods p
	                   WHERE p.status = 'closed' AND a.date BETWEEN p.start_date AND p.end_date)
	RETURNING a.id::text, a.scheduled_end, e.full_name, a.date::text`

// RunAutoCheckout closes the open attendance rows whose shift ended at or
// before now (R3). The update and its audit rows commit together; it returns
// how many rows were closed.
func (s *Service) RunAutoCheckout(ctx context.Context, now time.Time) (closed int, err error) {
	since := domain.AddDaysISO(domain.TodayWIB(now), -autoCheckoutLookbackDays)
	err = s.repo.InTx(ctx, func(r Repository) error {
		closed = 0
		q := r.querier()
		rows, err := q.Query(ctx, autoCheckoutSQL, now.UTC(), since)
		if err != nil {
			return err
		}
		type closedRow struct {
			id, name, date string
			end            time.Time
		}
		var done []closedRow
		for rows.Next() {
			var c closedRow
			if err := rows.Scan(&c.id, &c.end, &c.name, &c.date); err != nil {
				rows.Close()
				return err
			}
			done = append(done, c)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		actor := autoCheckoutActor
		for _, c := range done {
			id, label := c.id, c.name+" · "+c.date
			if err := audit.Write(ctx, q, audit.Entry{
				ActorName:   &actor,
				Action:      autoCheckoutAction,
				Entity:      attendanceEntity,
				EntityID:    &id,
				EntityLabel: &label,
				After:       map[string]any{"clock_out": c.end.UTC().Format(time.RFC3339), "clock_out_auto": true},
			}); err != nil {
				return err
			}
		}
		closed = len(done)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return closed, nil
}

// AutoCheckoutLoop runs RunAutoCheckout once at start and then every interval
// until ctx ends. Errors and panics are logged; the loop never stops because
// of them.
func (s *Service) AutoCheckoutLoop(ctx context.Context, every time.Duration) {
	s.autoCheckoutTick(ctx)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.autoCheckoutTick(ctx)
		}
	}
}

func (s *Service) autoCheckoutTick(ctx context.Context) {
	defer func() {
		if p := recover(); p != nil {
			s.log.Error("hris auto checkout panicked", "panic", fmt.Sprint(p))
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	closed, err := s.RunAutoCheckout(ctx, s.now())
	if err != nil {
		s.log.Error("hris auto checkout failed", "error", err)
		return
	}
	if closed > 0 {
		s.log.Info("hris auto checkout", "closed", closed)
	}
}
