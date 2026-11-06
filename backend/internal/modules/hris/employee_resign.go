package hris

import (
	"context"
	"net/http"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Resign (Spec §1.1): one transaction sets the status, closes placements and
// schedules, blocks the login account and records history + audit.

type resignInput struct {
	EffectiveDate string
	Type          string // resign | terminated | end_of_contract
	Reason        *string
}

var resignStatus = map[string]string{"resign": "resigned", "end_of_contract": "resigned", "terminated": "terminated"}

// maxResignAheadDays is how far ahead a resign may be scheduled (EPIC-095 R2).
const maxResignAheadDays = 180

// resignDateText is dd-mm-yyyy for messages.
func resignDateText(iso string) string {
	if t, ok := domain.ParseDate(iso); ok {
		return t.Format("02-01-2006")
	}
	return iso
}

// pendingResignDate is the effective date of id's pending scheduled resign,
// "" when there is none.
func pendingResignDate(ctx context.Context, q database.Querier, id string) (string, error) {
	var d string
	err := q.QueryRow(ctx, `SELECT effective_date::text FROM hris.scheduled_resignations
		WHERE employee_id = $1 AND applied_at IS NULL AND cancelled_at IS NULL`, id).Scan(&d)
	if database.IsNoRows(err) {
		return "", nil
	}
	return d, err
}

// ResignEmployee resigns id at once when the effective date (the last working
// day) is today or earlier, or schedules it when the date is later (EPIC-095):
// the employee stays active and RunScheduledResigns applies it the day after.
func (s *Service) ResignEmployee(ctx context.Context, actor *auth.User, id string, in resignInput) (map[string]any, bool, error) {
	today := domain.TodayWIB(s.clock())
	if in.EffectiveDate > domain.AddDaysISO(today, maxResignAheadDays) {
		return nil, false, httpx.BadRequest("Tanggal efektif paling jauh 180 hari dari hari ini")
	}
	scheduled := in.EffectiveDate > today
	var out map[string]any
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		pending, err := pendingResignDate(ctx, q, id)
		if err != nil {
			return err
		}
		if pending != "" {
			return httpx.BadRequest("Sudah ada resign terjadwal efektif " + resignDateText(pending) + " — batalkan jadwal itu dulu")
		}
		if !scheduled {
			out, err = s.applyResign(ctx, q, actor, nil, id, in)
			return err
		}
		out, err = scheduleResign(ctx, q, actor, id, in)
		return err
	})
	return out, scheduled, err
}

// scheduleResign stores a resign for a future effective date (EPIC-095 R3):
// nothing else changes until RunScheduledResigns applies it.
func scheduleResign(ctx context.Context, q database.Querier, actor *auth.User, id string, in resignInput) (map[string]any, error) {
	var name, join string
	var active bool
	err := q.QueryRow(ctx, `SELECT full_name, join_date::text, is_active FROM hris.employees WHERE id = $1 FOR UPDATE`, id).Scan(&name, &join, &active)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound("Karyawan tidak ditemukan")
	}
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, httpx.BadRequest("Karyawan sudah nonaktif")
	}
	if in.EffectiveDate < join {
		return nil, httpx.BadRequest("Tanggal efektif tidak boleh sebelum tanggal masuk (" + domain.FormatDateID(join) + ")")
	}
	var sid string
	if err := q.QueryRow(ctx, `INSERT INTO hris.scheduled_resignations (employee_id, effective_date, resign_type, reason, created_by)
		VALUES ($1, $2::date, $3, $4, NULLIF($5, '')::uuid) RETURNING id::text`,
		id, in.EffectiveDate, in.Type, in.Reason, actor.ID).Scan(&sid); err != nil {
		return nil, err
	}
	if err := writeEmployeeAudit(ctx, q, actor, "employee.resign_schedule", id, name, nil,
		map[string]any{"effective_date": in.EffectiveDate, "resign_type": in.Type, "reason": in.Reason}); err != nil {
		return nil, err
	}
	return map[string]any{"id": sid, "scheduled": true, "effective_date": in.EffectiveDate, "resign_type": in.Type}, nil
}

// applyResign is the resign itself, inside the caller's transaction: status,
// inactive, end_date, placements and schedules closed, history, login blocked
// and audit. recordedBy is the configuration.users id behind
// employment_history.recorded_by; nil means actor.
func (s *Service) applyResign(ctx context.Context, q database.Querier, actor *auth.User, recordedBy *string, id string, in resignInput) (map[string]any, error) {
	recorder := actor.ID
	if recordedBy != nil {
		recorder = *recordedBy
	}
	var name, status, join string
	var active bool
	var userID *string
	err := q.QueryRow(ctx, `SELECT full_name, employment_status, join_date::text, is_active, user_id::text
		FROM hris.employees WHERE id = $1 FOR UPDATE`, id).Scan(&name, &status, &join, &active, &userID)
	if database.IsNoRows(err) {
		return nil, httpx.NotFound("Karyawan tidak ditemukan")
	}
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, httpx.BadRequest("Karyawan sudah nonaktif")
	}
	if in.EffectiveDate < join {
		return nil, httpx.BadRequest("Tanggal efektif tidak boleh sebelum tanggal masuk (" + domain.FormatDateID(join) + ")")
	}
	newStatus := resignStatus[in.Type]
	d := in.EffectiveDate
	steps := []struct {
		sql  string
		args []any
	}{
		{`UPDATE hris.employees SET employment_status = $2, is_active = false, end_date = $3::date, is_access_app = false, updated_at = now() WHERE id = $1`, []any{id, newStatus, d}},
		{`DELETE FROM hris.employee_placements WHERE employee_id = $1 AND start_date > $2::date`, []any{id, d}},
		{`UPDATE hris.employee_placements SET end_date = $2::date WHERE employee_id = $1 AND (end_date IS NULL OR end_date > $2::date)`, []any{id, d}},
		{`DELETE FROM hris.employee_shifts WHERE employee_id = $1 AND effective_from > $2::date`, []any{id, d}},
		{`UPDATE hris.employee_shifts SET effective_to = $2::date WHERE employee_id = $1 AND (effective_to IS NULL OR effective_to > $2::date)`, []any{id, d}},
		{`DELETE FROM hris.employee_shift_days WHERE employee_id = $1 AND work_date > $2::date`, []any{id, d}},
		{`INSERT INTO hris.employment_history (employee_id, change_type, effective_date, prev_employment_status, new_employment_status, reason, recorded_by)
		  VALUES ($1, $2, $3::date, $4, $5, $6, (SELECT e.id FROM hris.employees e WHERE e.user_id = NULLIF($7, '')::uuid LIMIT 1))`,
			[]any{id, in.Type, d, status, newStatus, in.Reason, recorder}}, // recorded_by → hris.employees(id)
	}
	if userID != nil {
		// Same effect as accounts.revokeAccount (ban_duration 876000h).
		steps = append(steps,
			struct {
				sql  string
				args []any
			}{`UPDATE auth.users SET banned_until = now() + interval '876000 hours' WHERE id = $1`, []any{*userID}},
			struct {
				sql  string
				args []any
			}{`UPDATE configuration.users SET status = 'inactive', updated_at = now() WHERE id = $1`, []any{*userID}})
	}
	for _, st := range steps {
		if _, err := q.Exec(ctx, st.sql, st.args...); err != nil {
			return nil, err
		}
	}
	if err := writeEmployeeAudit(ctx, q, actor, "employee.resign", id, name,
		map[string]any{"employment_status": status, "is_active": true},
		map[string]any{"employment_status": newStatus, "is_active": false, "end_date": d, "resign_type": in.Type, "reason": in.Reason}); err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "employment_status": newStatus, "end_date": d}, nil
}

func (h handlers) resignEmployee(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	const dateMsg = "Tanggal efektif wajib diisi"
	in := resignInput{
		EffectiveDate: strOr(reqStr(f, "effective_date", dateMsg, validate.StrOpts{Check: matches(domain.DateRe, dateMsg)})),
		Type:          strOr(f.Enum("resign_type", validate.Rule{}, []string{"resign", "terminated", "end_of_contract"})),
		Reason:        f.Str("reason", nullish, validate.StrOpts{Trim: true, Max: 500}),
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	row, scheduled, err := h.svc.ResignEmployee(r.Context(), u, id, in)
	if err != nil {
		return err
	}
	if scheduled {
		return ok(w, dataMsg(row, "Resign dijadwalkan efektif "+resignDateText(in.EffectiveDate)))
	}
	return ok(w, dataMsg(row, "Karyawan diresign"))
}

// GET /api/hris/employees/{id}/resign: the pending scheduled resign, or null.
func (h handlers) getScheduledResign(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	var sid, date, typ string
	var reason, by *string
	var at time.Time
	err = h.svc.repo.querier().QueryRow(r.Context(), `SELECT s.id::text, s.effective_date::text, s.resign_type, s.reason, u.full_name, s.created_at
		FROM hris.scheduled_resignations s LEFT JOIN configuration.users u ON u.id = s.created_by
		WHERE s.employee_id = $1 AND s.applied_at IS NULL AND s.cancelled_at IS NULL`, id).Scan(&sid, &date, &typ, &reason, &by, &at)
	if database.IsNoRows(err) {
		return ok(w, dataOf(nil))
	}
	if err != nil {
		return err
	}
	return ok(w, dataOf(obj("id", sid, "effective_date", date, "resign_type", typ, "reason", reason,
		"created_by_name", by, "created_at", at)))
}

// DELETE /api/hris/employees/{id}/resign {reason?}: cancels the pending
// scheduled resign (EPIC-095 R6); the row stays as history.
func (h handlers) cancelScheduledResign(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	f := optionalForm(r)
	reason := f.Str("reason", nullish, validate.StrOpts{Trim: true, Max: 500})
	if err := formErr(f, ""); err != nil {
		return err
	}
	ctx := r.Context()
	err = h.svc.repo.InTx(ctx, func(repo Repository) error {
		q := repo.querier()
		var sid, date, typ, name string
		err := q.QueryRow(ctx, `SELECT s.id::text, s.effective_date::text, s.resign_type, e.full_name
			FROM hris.scheduled_resignations s JOIN hris.employees e ON e.id = s.employee_id
			WHERE s.employee_id = $1 AND s.applied_at IS NULL AND s.cancelled_at IS NULL
			FOR UPDATE OF s`, id).Scan(&sid, &date, &typ, &name)
		if database.IsNoRows(err) {
			return httpx.NotFound("Tidak ada resign terjadwal")
		}
		if err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `UPDATE hris.scheduled_resignations
			SET cancelled_at = now(), cancelled_by = $2, cancel_reason = $3 WHERE id = $1`, sid, u.ID, nilIfEmpty(reason)); err != nil {
			return err
		}
		return writeEmployeeAudit(ctx, q, u, "employee.resign_schedule_cancel", id, name,
			map[string]any{"effective_date": date, "resign_type": typ}, map[string]any{"cancel_reason": nilIfEmpty(reason)})
	})
	if err != nil {
		return err
	}
	return ok(w, dataMsg(obj("id", id), "Jadwal resign dibatalkan"))
}
