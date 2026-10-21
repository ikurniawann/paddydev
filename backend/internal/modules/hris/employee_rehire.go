package hris

import (
	"context"
	"net/http"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Rehire (EPIC-064): one transaction reactivates a resigned employee on the
// same record (same NIP, KTP, NPWP, history): new status and join date, a
// new primary placement, the fixed schedule for Non-Shift staff, and — when
// asked — the login account. It is the inverse of ResignEmployee.

const (
	msgRehireShiftNote = "Atur jadwal shift karyawan ini di Jadwal Kerja"
)

type rehireInput struct {
	EffectiveDate    string
	EmploymentStatus string
	BranchID         string
	ReactivateLogin  bool
	Email            *string
	Reason           *string
}

// rehireStatusAllowed is the status list the import offers (loadEmployeeLists:
// built-in names plus active master rows, without resigned/terminated).
func rehireStatusAllowed(ctx context.Context, q database.Querier, code string) (bool, error) {
	l, err := loadEmployeeLists(ctx, q)
	if err != nil {
		return false, err
	}
	for _, c := range l.statusCodes {
		if c == code {
			return true, nil
		}
	}
	return false, nil
}

// RehireEmployee reactivates a resigned employee. superAdmin tells whether
// the caller holds the IAM super_admin role (needed to restore the login of
// an account holding a system role); nil means no.
func (s *Service) RehireEmployee(ctx context.Context, actor *auth.User, id string, in rehireInput,
	superAdmin func(context.Context) (bool, error)) (map[string]any, error) {
	if _, ok := domain.ParseDate(in.EffectiveDate); !ok {
		return nil, httpx.BadRequest("Tanggal efektif tidak valid")
	}
	// Seperti resign: karyawan langsung aktif, jadi tanggalnya tidak di masa depan.
	today := domain.TodayWIB(s.clock())
	if in.EffectiveDate > today {
		return nil, httpx.BadRequest("Tanggal efektif tidak boleh setelah hari ini")
	}
	var out map[string]any
	err := s.repo.InTx(ctx, func(r Repository) error {
		q := r.querier()
		var name, status, join, scheduleType string
		var end, userID *string
		var email string
		var active bool
		err := q.QueryRow(ctx, `SELECT full_name, employment_status, join_date::text, end_date::text, is_active,
			user_id::text, email, schedule_type FROM hris.employees WHERE id = $1 FOR UPDATE`, id).
			Scan(&name, &status, &join, &end, &active, &userID, &email, &scheduleType)
		if database.IsNoRows(err) {
			return httpx.NotFound("Karyawan tidak ditemukan")
		}
		if err != nil {
			return err
		}
		if active {
			return httpx.BadRequest("Karyawan masih aktif")
		}
		// Penempatan memakai rentang inklusif: tanggal yang sama dengan hari
		// keluar akan bentrok dengan penempatan lama.
		if end != nil && in.EffectiveDate <= *end {
			return httpx.BadRequest("Tanggal efektif harus setelah tanggal keluar (" + domain.FormatDateID(*end) + ")")
		}
		// Nonaktif tanpa tanggal keluar (tidak lewat resign): pakai tanggal masuk lama.
		if end == nil && in.EffectiveDate <= join {
			return httpx.BadRequest("Tanggal efektif harus setelah tanggal masuk sebelumnya (" + domain.FormatDateID(join) + ")")
		}
		if ok, err := rehireStatusAllowed(ctx, q, in.EmploymentStatus); err != nil {
			return err
		} else if !ok {
			return httpx.BadRequest("Status kepegawaian tidak valid untuk karyawan aktif")
		}
		if branchOK, err := r.BranchExists(ctx, in.BranchID); err != nil {
			return err
		} else if !branchOK {
			return httpx.BadRequest("Cabang tidak ditemukan")
		}
		newEmail, err := rehireEmail(ctx, q, id, email, userID != nil, in.Email)
		if err != nil {
			return err
		}
		restoreLogin := userID != nil && in.ReactivateLogin
		if restoreLogin {
			if err := guardSystemReactivation(ctx, q, *userID, msgSystemReactivation, superAdmin); err != nil {
				return err
			}
		}
		nonShift := scheduleType == scheduleNonShift
		from := max(in.EffectiveDate, today)
		if nonShift {
			if err := s.checkScheduleFrom(ctx, from); err != nil {
				return err
			}
		}

		if _, err := q.Exec(ctx, `UPDATE hris.employees SET is_active = true, end_date = NULL, employment_status = $2,
			join_date = $3::date, is_access_app = $4, email = COALESCE($5, email), updated_at = now() WHERE id = $1`,
			id, in.EmploymentStatus, in.EffectiveDate, restoreLogin, newEmail); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, notes, created_by)
			VALUES ($1, $2, true, $3::date, 'Aktif kembali', $4)`, id, in.BranchID, in.EffectiveDate, actor.ID); err != nil {
			return placementError(err)
		}
		if nonShift {
			if _, err := applyFixedSchedule(ctx, r, id, from, actor, clearGuard{}); err != nil {
				return err
			}
		}
		if restoreLogin {
			if _, err := q.Exec(ctx, `UPDATE auth.users SET banned_until = NULL WHERE id = $1`, *userID); err != nil {
				return err
			}
			if _, err := q.Exec(ctx, `UPDATE configuration.users SET status = 'active', updated_at = now() WHERE id = $1`, *userID); err != nil {
				return err
			}
		}
		if _, err := q.Exec(ctx, `INSERT INTO hris.employment_history
			(employee_id, change_type, effective_date, prev_employment_status, new_employment_status, reason, notes, recorded_by)
			VALUES ($1, 'rehire', $2::date, $3, $4, $5, $6, (SELECT e.id FROM hris.employees e WHERE e.user_id = $7::uuid LIMIT 1))`,
			id, in.EffectiveDate, status, in.EmploymentStatus, in.Reason, "Tanggal masuk sebelumnya: "+domain.FormatDateID(join), actor.ID); err != nil {
			return err
		}
		before := map[string]any{"is_active": false, "employment_status": status, "join_date": join, "end_date": end}
		after := map[string]any{"is_active": true, "employment_status": in.EmploymentStatus, "join_date": in.EffectiveDate,
			"branch_id": in.BranchID, "reactivate_login": restoreLogin, "reason": in.Reason}
		if newEmail != nil {
			before["email"], after["email"] = email, *newEmail
		}
		if err := writeEmployeeAudit(ctx, q, actor, "employee.rehire", id, name, before, after); err != nil {
			return err
		}
		out = map[string]any{"id": id, "employment_status": in.EmploymentStatus, "join_date": in.EffectiveDate}
		if !nonShift {
			out["schedule_note"] = msgRehireShiftNote
		}
		return nil
	})
	return out, err
}

// rehireEmail applies ruling R7: an email may only be set for an employee
// without a login (the login's email is managed in Manajemen User) and must
// be unused by any other employee or login account. Nil = leave unchanged.
func rehireEmail(ctx context.Context, q database.Querier, id, current string, hasLogin bool, want *string) (*string, error) {
	if want == nil || *want == "" {
		return nil, nil
	}
	if hasLogin {
		return nil, httpx.BadRequest("Email akun login diubah lewat Manajemen User")
	}
	if strings.EqualFold(*want, current) {
		return nil, nil
	}
	var taken bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.employees WHERE lower(email) = lower($1) AND id <> $2)
		OR EXISTS (SELECT 1 FROM auth.users WHERE lower(email) = lower($1))`, *want, id).Scan(&taken); err != nil {
		return nil, err
	}
	if taken {
		return nil, httpx.BadRequest("Email sudah digunakan")
	}
	return want, nil
}

func (h handlers) rehireEmployee(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID karyawan tidak valid")
	if err != nil {
		return err
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	const dateMsg = "Tanggal efektif wajib diisi"
	const branchMsg = "Cabang penempatan wajib diisi"
	in := rehireInput{
		EffectiveDate: strOr(reqStr(f, "effective_date", dateMsg, validate.StrOpts{Trim: true, Check: matches(domain.DateRe, dateMsg)})),
		EmploymentStatus: strOr(reqStr(f, "employment_status", "Status kepegawaian wajib diisi",
			validate.StrOpts{Trim: true, Max: 50, Check: minLen(1, "Status kepegawaian wajib diisi")})),
		BranchID: strOr(reqStr(f, "branch_id", branchMsg, validate.StrOpts{Trim: true, Check: both(minLen(1, branchMsg),
			func(s string) (string, string, bool) {
				return "invalid_format", "Cabang tidak ditemukan", domain.IsUUID(s)
			})})),
		ReactivateLogin: f.BoolDefault("reactivate_login", true),
		Email:           f.Str("email", nullish, validate.StrOpts{Trim: true, Max: 255}),
		Reason:          f.Str("reason", nullish, validate.StrOpts{Trim: true, Max: 500}),
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	if in.Email != nil && *in.Email != "" && !zodEmail.MatchString(*in.Email) {
		return httpx.BadRequest("Format email tidak valid")
	}
	row, err := h.svc.RehireEmployee(r.Context(), u, id, in,
		func(ctx context.Context) (bool, error) { return h.auth.HasRoleCode(ctx, u, "super_admin") })
	if err != nil {
		return err
	}
	return ok(w, dataMsg(row, "Karyawan diaktifkan kembali"))
}
