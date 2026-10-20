package hris

import (
	"fmt"
	"net/http"
	"strconv"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/validate"
)

// coerceInt is z.coerce.number().int().min(lo).max(hi).default(def) on a
// query form (values are strings).
func coerceInt(f *validate.Form, key string, lo, hi *float64, def int) int {
	v, present := f.Fields()[key]
	if !present {
		return def
	}
	n := domain.JSNumber(fmt.Sprint(v))
	if !domain.IsFinite(n) {
		f.Fail(key, "invalid_type", "Invalid input: expected number, received NaN")
		return def
	}
	if n != float64(int64(n)) {
		f.Fail(key, "invalid_type", "Invalid input: expected int, received number")
		return def
	}
	switch {
	case lo != nil && n < *lo:
		f.Fail(key, "too_small", fmt.Sprintf("Too small: expected number to be >=%v", *lo))
	case hi != nil && n > *hi:
		f.Fail(key, "too_big", fmt.Sprintf("Too big: expected number to be <=%v", *hi))
	}
	return int(n)
}

func leaveFilterOf(f *validate.Form) leaveFilter {
	s := func(k string) *string { return f.Str(k, optional, validate.StrOpts{}) }
	return leaveFilter{EmployeeID: s("employee_id"), Status: s("status"), LeaveType: s("leave_type"),
		StartDate: s("start_date"), EndDate: s("end_date")}
}

func (h handlers) listLeaves(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f := queryForm(r)
	filter := leaveFilterOf(f)
	page := coerceInt(f, "page", validate.Bound(1), nil, 1)
	limit := coerceInt(f, "limit", validate.Bound(1), validate.Bound(500), 20)
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.ListLeaves(r.Context(), a, filter, page, limit)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) createLeave(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	var in leaveRequest
	in.EmployeeID = f.UUID("employee_id", optional)
	in.LeaveType = strOr(f.Enum("leave_type", validate.Rule{}, domain.LeaveTypeCodes()))
	in.StartDate = strOr(f.Str("start_date", validate.Rule{}, validate.StrOpts{Check: matches(domain.DateRe, "Tanggal mulai harus berformat YYYY-MM-DD")}))
	in.EndDate = strOr(f.Str("end_date", validate.Rule{}, validate.StrOpts{Check: matches(domain.DateRe, "Tanggal selesai harus berformat YYYY-MM-DD")}))
	in.Reason = strOr(f.Str("reason", validate.Rule{}, validate.StrOpts{Check: minLen(10, "Reason must be at least 10 characters")}))
	in.AttachmentURL = f.Str("attachment_url", optional, validate.StrOpts{})
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.CreateLeave(r.Context(), a, in)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) getLeave(w http.ResponseWriter, r *http.Request, a *Actor) error {
	row, err := h.svc.Leave(r.Context(), a, r.PathValue("id"))
	if err != nil {
		return err
	}
	return ok(w, dataOf(row))
}

func (h handlers) updateLeave(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	in := leaveUpdate{
		Status:        f.Str("status", optional, validate.StrOpts{}),
		Reason:        str(f, "reason", nullish, validate.StrOpts{}),
		AttachmentURL: str(f, "attachment_url", nullish, validate.StrOpts{}),
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	out, err := h.svc.UpdateLeave(r.Context(), a, r.PathValue("id"), in)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) deleteLeave(w http.ResponseWriter, r *http.Request, a *Actor) error {
	if err := h.svc.DeleteLeave(r.Context(), a, r.PathValue("id")); err != nil {
		return err
	}
	return ok(w, msgOf("Leave request deleted successfully"))
}

func (h handlers) decideLeave(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	in := leaveDecision{
		LeaveID:   strOr(f.UUID("leave_id", validate.Rule{})),
		Action:    strOr(f.Enum("action", validate.Rule{}, []string{"approve", "reject"})),
		Rejection: f.Str("rejection_reason", optional, validate.StrOpts{}),
	}
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.DecideLeave(r.Context(), a, in)
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (h handlers) exportLeaves(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	f := queryForm(r)
	filter := leaveFilterOf(f)
	if err := formErr(f, ""); err != nil {
		return err
	}
	csv, err := h.svc.LeavesCSV(r.Context(), filter)
	if err != nil {
		return err
	}
	name := "leave_export_" + h.svc.now().UTC().Format(domain.DateLayout) + ".csv"
	w.Header().Set("Content-Type", "text/csv;charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(csv)))
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(csv))
	return err
}

/* ── Leave balances ──────────────────────────────────────────────────── */

// balanceYear is yearOf in the route: ?year (2000..2100), else this year.
func (h handlers) balanceYear(r *http.Request) (int, error) {
	f := queryForm(r)
	year := coerceInt(f, "year", validate.Bound(2000), validate.Bound(2100), 0)
	if err := formErr(f, "Tahun tidak valid"); err != nil {
		return 0, err
	}
	if year == 0 {
		year = domain.NowWIB(h.svc.now()).Year()
	}
	return year, nil
}

func (h handlers) getLeaveBalance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	year, err := h.balanceYear(r)
	if err != nil {
		return err
	}
	row, err := h.svc.LeaveBalance(r.Context(), a, r.PathValue("employee_id"), year)
	if err != nil {
		return err
	}
	return ok(w, dataOf(row))
}

func (h handlers) putLeaveBalance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	f, err := readForm(r, "")
	if err != nil {
		return err
	}
	var in fields
	for _, col := range []string{"annual_leave_total", "annual_leave_used", "sick_leave_used", "unpaid_leave_used"} {
		if v := number(f, col, optional, validate.NumOpts{Min: validate.Bound(0)}); v.Sent {
			in.add(col, v.Val)
		}
	}
	if err := formErr(f, ""); err != nil {
		return err
	}
	year, err := h.balanceYear(r)
	if err != nil {
		return err
	}
	row, err := h.svc.UpdateLeaveBalance(r.Context(), a, r.PathValue("employee_id"), year, in)
	if err != nil {
		return err
	}
	return ok(w, obj("message", "Leave balance updated successfully", "data", row))
}
