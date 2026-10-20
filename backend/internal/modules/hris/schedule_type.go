package hris

import (
	"context"
	"fmt"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Tipe jadwal karyawan (Spec §2.2–2.3): shift = ditugaskan di Jadwal Kerja,
// non_shift = jadwal tetap Sen–Jum NS 09.00–18.00, Sab–Min libur.

const (
	scheduleShift    = "shift"
	scheduleNonShift = "non_shift"
	msgFromPast      = "Tanggal mulai tidak boleh sebelum hari ini"
)

func fixedPatternDays(ns string) []patternDay {
	days := make([]patternDay, 7)
	for i := range days {
		days[i].DayOfWeek = i + 1
		if i < 5 {
			id := ns
			days[i].ShiftID = &id
		}
	}
	return days
}

func shiftIDByCode(ctx context.Context, q database.Querier, code string) (string, error) {
	var id string
	err := q.QueryRow(ctx, `SELECT id::text FROM hris.shifts WHERE code = $1`, code).Scan(&id)
	return id, err
}

// applyFixedSchedule clears per-date schedules from `from` on and saves the
// fixed weekly pattern from `from`; it returns the cleared per-date rows.
// Clearing rows needs the clear guard and is audited (EPIC-061).
func applyFixedSchedule(ctx context.Context, r Repository, employeeID, from string, actor *auth.User, g clearGuard) (int, error) {
	q := r.querier()
	tag, err := q.Exec(ctx, `DELETE FROM hris.employee_shift_days WHERE employee_id = $1 AND work_date >= $2::date`, employeeID, from)
	if err != nil {
		return 0, err
	}
	n := int(tag.RowsAffected())
	if err := g.check(n); err != nil {
		return 0, err
	}
	if err := g.audit(ctx, q, actor, "hris.schedule.clear", employeeID, map[string]any{"cleared_days": n, "from": from}, n); err != nil {
		return 0, err
	}
	ns, err := shiftIDByCode(ctx, q, "NS")
	if err != nil {
		return 0, err
	}
	return n, r.SaveShiftPattern(ctx, employeeID, from, fixedPatternDays(ns), actor.FullName)
}

// stopWeeklyPattern ends running weekly rows the day before `from` and
// drops rows starting on/after it (Non-Shift → Shift).
func stopWeeklyPattern(ctx context.Context, q database.Querier, employeeID, from string) error {
	if _, err := q.Exec(ctx, `DELETE FROM hris.employee_shifts WHERE employee_id = $1 AND effective_from >= $2::date`, employeeID, from); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `UPDATE hris.employee_shifts SET effective_to = $2::date - 1
		WHERE employee_id = $1 AND (effective_to IS NULL OR effective_to >= $2::date)`, employeeID, from)
	return err
}

type scheduleTypeResult struct {
	Changed     int      `json:"changed"`
	ClearedDays int      `json:"cleared_days"`
	Warnings    []string `json:"warnings"`
}

type scheduleTarget struct {
	ID, Name, Type string
	Active         bool
}

func loadScheduleTargets(ctx context.Context, q database.Querier, ids []string) ([]scheduleTarget, error) {
	rows, err := q.Query(ctx, `SELECT id::text, full_name, schedule_type, is_active FROM hris.employees
		WHERE id = ANY($1::uuid[]) ORDER BY full_name`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []scheduleTarget
	for rows.Next() {
		var t scheduleTarget
		if err := rows.Scan(&t.ID, &t.Name, &t.Type, &t.Active); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(ids) {
		return nil, httpx.BadRequest("Sebagian karyawan tidak ditemukan")
	}
	for _, t := range out {
		if !t.Active {
			return nil, httpx.BadRequest(t.Name + " sudah nonaktif/resign")
		}
	}
	return out, nil
}

func (s *Service) checkScheduleFrom(ctx context.Context, from string) error {
	if _, ok := domain.ParseDate(from); !ok {
		return httpx.BadRequest("Tanggal mulai wajib diisi (YYYY-MM-DD)")
	}
	if from < domain.TodayWIB(s.clock()) {
		return httpx.BadRequest(msgFromPast)
	}
	return s.ensureAttendanceOpen(ctx, from)
}

func (s *Service) ChangeScheduleType(ctx context.Context, actor *auth.User, ids []string, to, from string, dryRun bool, g clearGuard) (*scheduleTypeResult, error) {
	if to != scheduleShift && to != scheduleNonShift {
		return nil, httpx.BadRequest("Tipe jadwal harus Shift atau Non-Shift")
	}
	if err := s.checkScheduleFrom(ctx, from); err != nil {
		return nil, err
	}
	var res *scheduleTypeResult
	err := s.repo.InTx(ctx, func(r Repository) error {
		var err error
		res, err = changeScheduleTypeTx(ctx, r, actor, ids, to, from, dryRun, g)
		return err
	})
	return res, err
}

// namePrefix names the employee in a warning only for bulk changes.
func namePrefix(n int, name string) string {
	if n > 1 {
		return name + ": "
	}
	return ""
}

func (s *Service) ApplyFixedSchedule(ctx context.Context, actor *auth.User, branchID string, ids []string, from string, g clearGuard) (*scheduleTypeResult, error) {
	if err := s.checkScheduleFrom(ctx, from); err != nil {
		return nil, err
	}
	res := &scheduleTypeResult{Warnings: []string{}}
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		if branchID != "" {
			rows, err := q.Query(ctx, `SELECT DISTINCT e.id::text FROM hris.employees e
				JOIN hris.employee_placements p ON p.employee_id = e.id AND p.branch_id = $1::uuid
				  AND p.start_date <= $2::date AND (p.end_date IS NULL OR p.end_date >= $2::date)
				WHERE e.is_active AND e.schedule_type = 'non_shift'`, branchID, from)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				ids = append(ids, id)
			}
			rows.Close()
			if len(ids) == 0 {
				return httpx.BadRequest("Tidak ada karyawan Non-Shift aktif di cabang ini")
			}
		}
		targets, err := loadScheduleTargets(ctx, q, ids)
		if err != nil {
			return err
		}
		for _, t := range targets {
			if t.Type != scheduleNonShift {
				return httpx.BadRequest(t.Name + " bertipe Shift — jadwal tetap hanya untuk karyawan Non-Shift")
			}
		}
		for _, t := range targets {
			n, err := applyFixedSchedule(ctx, r, t.ID, from, actor, g)
			if err != nil {
				return err
			}
			res.Changed++
			res.ClearedDays += n
			if err := writeEmployeeAudit(ctx, q, actor, "schedule.fixed_apply", t.ID, t.Name, nil,
				map[string]any{"effective_from": from, "pattern": "Sen–Jum NS 09.00–18.00, Sab–Min libur"}); err != nil {
				return err
			}
		}
		return nil
	})
	return res, err
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

type scheduleTypeBody struct {
	ids    []string
	to     string
	from   string
	dryRun bool
	reason string
}

func readScheduleTypeBody(r *http.Request, bulk bool) (scheduleTypeBody, error) {
	f, err := readForm(r, "")
	if err != nil {
		return scheduleTypeBody{}, err
	}
	var b scheduleTypeBody
	b.to = strOr(f.Enum("schedule_type", validate.Rule{}, []string{scheduleShift, scheduleNonShift}))
	b.from = strOr(reqStr(f, "effective_from", "Tanggal mulai wajib diisi (YYYY-MM-DD)", validate.StrOpts{Check: matches(domain.DateRe, "Tanggal mulai wajib diisi (YYYY-MM-DD)")}))
	b.reason = strOr(f.Str("reason", optional, validate.StrOpts{}))
	if bulk {
		b.ids = f.Strings("employee_ids", validate.Rule{}, 1000, validate.StrOpts{Check: validate.UUIDCheck})
		if v := f.Bool("dry_run", optional); v != nil {
			b.dryRun = *v
		}
		if len(b.ids) == 0 {
			f.Fail("employee_ids", "custom", "Pilih minimal satu karyawan")
		}
	}
	return b, formErr(f, "")
}

func (h handlers) getScheduleType(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	var typ string
	var future int
	err = h.svc.repo.querier().QueryRow(r.Context(), `SELECT e.schedule_type,
		(SELECT count(*) FROM hris.employee_shift_days d WHERE d.employee_id = e.id AND d.work_date >= $2::date)
		FROM hris.employees e WHERE e.id = $1`, id, domain.TodayWIB(h.svc.clock())).Scan(&typ, &future)
	if database.IsNoRows(err) {
		return httpx.NotFound("Karyawan tidak ditemukan")
	}
	if err != nil {
		return err
	}
	return ok(w, dataOf(map[string]any{"schedule_type": typ, "future_days": future}))
}

func (h handlers) putScheduleType(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	b, err := readScheduleTypeBody(r, false)
	if err != nil {
		return err
	}
	g, err := h.clearGuardFor(r, u, b.reason)
	if err != nil {
		return err
	}
	res, err := h.svc.ChangeScheduleType(r.Context(), u, []string{id}, b.to, b.from, false, g)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(res, "Tipe jadwal disimpan"))
}

func (h handlers) bulkScheduleType(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	b, err := readScheduleTypeBody(r, true)
	if err != nil {
		return err
	}
	g, err := h.clearGuardFor(r, u, b.reason)
	if err != nil {
		return err
	}
	res, err := h.svc.ChangeScheduleType(r.Context(), u, b.ids, b.to, b.from, b.dryRun, g)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(res, fmt.Sprintf("Tipe jadwal %d karyawan disimpan", res.Changed)))
}

func (h handlers) applyFixedSchedule(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	branch := f.UUID("branch_id", nullish)
	ids := f.Strings("employee_ids", optional, 1000, validate.StrOpts{Check: validate.UUIDCheck})
	from := strOr(reqStr(f, "effective_from", "Tanggal mulai wajib diisi (YYYY-MM-DD)", validate.StrOpts{Check: matches(domain.DateRe, "Tanggal mulai wajib diisi (YYYY-MM-DD)")}))
	reason := strOr(f.Str("reason", optional, validate.StrOpts{}))
	if branch == nil && len(ids) == 0 {
		f.Fail("branch_id", "custom", "Pilih cabang atau karyawan")
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	g, err := h.clearGuardFor(r, u, reason)
	if err != nil {
		return err
	}
	res, err := h.svc.ApplyFixedSchedule(r.Context(), u, strOr(branch), ids, from, g)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(res, fmt.Sprintf("Jadwal tetap diterapkan ke %d karyawan", res.Changed)))
}

// changeScheduleTypeTx is ChangeScheduleType on r (callers own the
// transaction and the from-date check); the import reuses it.
func changeScheduleTypeTx(ctx context.Context, r Repository, actor *auth.User, ids []string, to, from string, dryRun bool, g clearGuard) (*scheduleTypeResult, error) {
	res := &scheduleTypeResult{Warnings: []string{}}
	q := r.querier()
	targets, err := loadScheduleTargets(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if t.Type == to {
			continue
		}
		res.Changed++
		if dryRun {
			var n int
			if to == scheduleNonShift {
				if err := q.QueryRow(ctx, `SELECT count(*) FROM hris.employee_shift_days WHERE employee_id = $1 AND work_date >= $2::date`, t.ID, from).Scan(&n); err != nil {
					return nil, err
				}
			}
			res.ClearedDays += n
			continue
		}
		if to == scheduleNonShift {
			n, err := applyFixedSchedule(ctx, r, t.ID, from, actor, g)
			if err != nil {
				return nil, err
			}
			res.ClearedDays += n
		} else {
			if err := stopWeeklyPattern(ctx, q, t.ID, from); err != nil {
				return nil, err
			}
			res.Warnings = append(res.Warnings, fmt.Sprintf("%sKaryawan belum punya jadwal shift mulai %s; tugaskan di Jadwal Kerja",
				namePrefix(len(targets), t.Name), domain.FormatDateID(from)))
		}
		if _, err := q.Exec(ctx, `UPDATE hris.employees SET schedule_type = $2, updated_at = now() WHERE id = $1`, t.ID, to); err != nil {
			return nil, err
		}
		if err := writeEmployeeAudit(ctx, q, actor, "employee.schedule_type", t.ID, t.Name,
			map[string]any{"schedule_type": t.Type}, map[string]any{"schedule_type": to, "effective_from": from}); err != nil {
			return nil, err
		}
	}
	return res, nil
}
