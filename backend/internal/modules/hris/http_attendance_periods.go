package hris

import (
	"net/http"
	"strconv"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/validate"
)

// reopenMenu is the menu whose "approve" action marks an HR manager who
// may reopen a closed attendance period (besides super_admin).
var reopenMenu = []string{"hris.workforce.attendance"}

// periodReaders may read periods and the period settings: HR roles, or a
// role granted the attendance or payroll menus.
var periodReaders = append(append([]string{}, iam.HrisWorkforce...), iam.HrisCompensation...)

// withPeriodActor resolves the actor and its account for the period routes.
func (h handlers) withPeriodActor(fn func(w http.ResponseWriter, r *http.Request, a *Actor, u *auth.User) error) http.Handler {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		u, err := h.auth.RequireUser(r)
		if err != nil {
			return err
		}
		a, err := h.actorFor(r.Context(), u)
		if err != nil {
			return err
		}
		if !a.IsHR {
			allowed, err := h.auth.HasMenuPrefix(r.Context(), u, periodReaders...)
			if err != nil {
				return err
			}
			if !allowed {
				return httpx.Forbidden("Insufficient permissions")
			}
		}
		return fn(w, r, a, u)
	})
}

// canReopen: super_admin, or the approve action on the attendance menu.
func (h handlers) canReopen(r *http.Request, a *Actor, u *auth.User) (bool, error) {
	if a.Role == "super_admin" {
		return true, nil
	}
	return h.auth.HasMenuAction(r.Context(), u, "approve", reopenMenu...)
}

// GET /api/hris/attendance-periods?year=YYYY
func (h handlers) listAttendancePeriods(w http.ResponseWriter, r *http.Request, a *Actor, u *auth.User) error {
	year := 0
	if v := r.URL.Query().Get("year"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2000 || n > 2100 {
			return httpx.BadRequest("Tahun tidak valid")
		}
		year = n
	}
	reopen, err := h.canReopen(r, a, u)
	if err != nil {
		return err
	}
	out, err := h.svc.ListPeriods(r.Context(), a, year, reopen)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

// GET /api/hris/attendance-periods/{id}
func (h handlers) getAttendancePeriod(w http.ResponseWriter, r *http.Request, a *Actor, u *auth.User) error {
	out, err := h.svc.Period(r.Context(), r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

// POST /api/hris/attendance-periods/{id}/close
func (h handlers) closeAttendancePeriod(w http.ResponseWriter, r *http.Request, a *Actor, _ *auth.User) error {
	out, err := h.svc.ClosePeriod(r.Context(), a, r.PathValue("id"), auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, dataMsg(out, "Periode absensi "+out.Str("label")+" sudah di-closing"))
}

// POST /api/hris/attendance-periods/{id}/reopen { reason, closing_date? }
func (h handlers) reopenAttendancePeriod(w http.ResponseWriter, r *http.Request, a *Actor, u *auth.User) error {
	allowed, err := h.canReopen(r, a, u)
	if err != nil {
		return err
	}
	if !allowed {
		return httpx.Forbidden("Hanya super admin atau HR manager (izin approve menu Absensi) yang bisa membuka ulang periode absensi")
	}
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	reason := f.Str("reason", nullish, validate.StrOpts{Max: 1000})
	closing := f.Str("closing_date", nullish, validate.StrOpts{Check: matches(domain.DateRe, "Format tanggal closing tidak valid")})
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.ReopenPeriod(r.Context(), a, r.PathValue("id"), strOr(reason), strOr(closing), auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, dataMsg(out, "Periode absensi "+out.Str("label")+" dibuka ulang"))
}

// PATCH /api/hris/attendance-periods/{id} { closing_date, reason? }
func (h handlers) patchAttendancePeriod(w http.ResponseWriter, r *http.Request, a *Actor, _ *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	closing := reqStr(f, "closing_date", "Tanggal closing wajib diisi", validate.StrOpts{Check: matches(domain.DateRe, "Format tanggal closing tidak valid")})
	reason := f.Str("reason", nullish, validate.StrOpts{Max: 1000})
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.SetClosingDate(r.Context(), a, r.PathValue("id"), strOr(closing), reason, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, dataMsg(out, "Tanggal closing diperbarui"))
}

// GET /api/hris/attendance-settings[?preview_cutoff=N&preview_offset=N]:
// the stored settings and current period, plus the current/next period
// under the proposed values when previewing.
func (h handlers) getAttendanceSettings(w http.ResponseWriter, r *http.Request, a *Actor, _ *auth.User) error {
	q := r.URL.Query()
	if q.Has("preview_cutoff") {
		cutoff, err1 := strconv.Atoi(q.Get("preview_cutoff"))
		offset, err2 := strconv.Atoi(q.Get("preview_offset"))
		if err1 != nil || err2 != nil {
			return httpx.BadRequest("Pratinjau pengaturan tidak valid")
		}
		preview, err := h.svc.PreviewPeriodSettings(r.Context(), cutoff, offset)
		if err != nil {
			return err
		}
		return ok(w, dataOf(obj("preview", preview)))
	}
	out, err := h.svc.PeriodSettings(r.Context())
	if err != nil {
		return err
	}
	out.Set("can_manage", canManageAttendance(a))
	return ok(w, dataOf(out))
}

// PUT /api/hris/attendance-settings { cutoff_day, closing_offset_days }
func (h handlers) putAttendanceSettings(w http.ResponseWriter, r *http.Request, a *Actor, _ *auth.User) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	cutoff := f.Int("cutoff_day", validate.Rule{}, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(31)})
	offset := f.Int("closing_offset_days", validate.Rule{}, validate.NumOpts{Min: validate.Bound(0), Max: validate.Bound(31)})
	if err := formErr(f, "Tanggal cut-off (0/1–31) dan jarak closing (0–31 hari) wajib diisi"); err != nil {
		return err
	}
	out, err := h.svc.SavePeriodSettings(r.Context(), a, *cutoff, *offset, auditMeta(r))
	if err != nil {
		return err
	}
	out.Set("can_manage", canManageAttendance(a))
	return ok(w, dataMsg(out, "Pengaturan periode absensi tersimpan"))
}
