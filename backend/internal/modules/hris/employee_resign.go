package hris

import (
	"context"
	"net/http"

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

func (s *Service) ResignEmployee(ctx context.Context, actor *auth.User, id string, in resignInput) (map[string]any, error) {
	var out map[string]any
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		var name, status, join string
		var active bool
		var userID *string
		err := q.QueryRow(ctx, `SELECT full_name, employment_status, join_date::text, is_active, user_id::text
			FROM hris.employees WHERE id = $1 FOR UPDATE`, id).Scan(&name, &status, &join, &active, &userID)
		if database.IsNoRows(err) {
			return httpx.NotFound("Karyawan tidak ditemukan")
		}
		if err != nil {
			return err
		}
		if !active {
			return httpx.BadRequest("Karyawan sudah nonaktif")
		}
		if in.EffectiveDate < join {
			return httpx.BadRequest("Tanggal efektif tidak boleh sebelum tanggal masuk (" + domain.FormatDateID(join) + ")")
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
			  VALUES ($1, $2, $3::date, $4, $5, $6, (SELECT e.id FROM hris.employees e WHERE e.user_id = $7::uuid LIMIT 1))`,
				[]any{id, in.Type, d, status, newStatus, in.Reason, actor.ID}}, // recorded_by → hris.employees(id)
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
				return err
			}
		}
		if err := writeEmployeeAudit(ctx, q, actor, "employee.resign", id, name,
			map[string]any{"employment_status": status, "is_active": true},
			map[string]any{"employment_status": newStatus, "is_active": false, "end_date": d, "resign_type": in.Type, "reason": in.Reason}); err != nil {
			return err
		}
		out = map[string]any{"id": id, "employment_status": newStatus, "end_date": d}
		return nil
	})
	return out, err
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
	row, err := h.svc.ResignEmployee(r.Context(), u, id, in)
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Karyawan diresign"))
}
