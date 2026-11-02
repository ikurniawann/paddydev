package hris

import (
	"math"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

func pagination(page, limit int, total int64) *Row {
	return obj("page", page, "limit", limit, "total", total,
		"totalPages", int64(math.Ceil(float64(total)/float64(limit))))
}

func (h handlers) listAttendance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	p, err := resolveAttendanceListParams(queryOf(r), a)
	if err != nil {
		return err
	}
	if p == nil {
		return ok(w, obj("data", []any{}, "pagination", obj("page", 1, "limit", 0, "total", 0, "totalPages", 0)))
	}
	rows, total, err := h.svc.ListAttendance(r.Context(), *p)
	if err != nil {
		return err
	}
	return ok(w, obj("data", rows, "pagination", pagination(p.Page, p.Limit, total)))
}

func (h handlers) getAttendance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	row, err := h.svc.Attendance(r.Context(), r.PathValue("id"), a)
	if err != nil {
		return err
	}
	return ok(w, dataOf(row))
}

// PUT /api/hris/attendance/{id}: HR validation (validated,
// validation_notes) and manual adjustment (clock_in/clock_out "HH:MM" WIB,
// status, branch_id, late_minutes, overtime_hours, notes) with a reason.
func (h handlers) updateAttendance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	if !a.IsHR {
		return httpx.Forbidden("Forbidden: Only HRD or managers can update attendance")
	}
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	var c fields
	if v := str(f, "validation_notes", nullish, validate.StrOpts{Max: 5000}); v.Sent {
		c.add("validation_notes", v.Val)
	}
	validated := f.Bool("validated", optional)
	in := parseAdjustment(f, false)
	if err := formErr(f, ""); err != nil {
		return err
	}
	row, err := h.svc.AdjustAttendance(r.Context(), a, r.PathValue("id"), in, c, validated != nil && *validated, auditMeta(r))
	if err != nil {
		return err
	}
	return ok(w, obj("message", "Attendance updated successfully", "data", row))
}

// DELETE /api/hris/attendance/{id}?reason=… (or {reason} body)
func (h handlers) deleteAttendance(w http.ResponseWriter, r *http.Request, a *Actor) error {
	if err := h.svc.DeleteAttendanceWithReason(r.Context(), a, r.PathValue("id"), deleteReason(r), auditMeta(r)); err != nil {
		return err
	}
	return ok(w, msgOf("Attendance deleted successfully"))
}

func (h handlers) dailyRoster(w http.ResponseWriter, r *http.Request, a *Actor) error {
	if !a.IsHR {
		return httpx.Forbidden("Forbidden")
	}
	requested := r.URL.Query().Get("date")
	if requested != "" && !domain.DateRe.MatchString(requested) {
		return httpx.BadRequest("Format tanggal tidak valid")
	}
	today := domain.TodayWIB(h.svc.now())
	date := requested
	if date == "" {
		date = today
	}
	out, err := h.svc.DailyRoster(r.Context(), date, today)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

func (h handlers) attendanceSchedule(w http.ResponseWriter, r *http.Request, a *Actor) error {
	var employeeID string
	switch requested := r.URL.Query().Get("employee_id"); {
	case !a.IsHR:
		if a.EmployeeID == nil {
			return httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
		}
		employeeID = *a.EmployeeID
	case requested == "" || requested == "me":
		employeeID = strOr(a.EmployeeID)
	default:
		employeeID = requested
	}
	if !domain.IsUUID(employeeID) {
		return httpx.BadRequest("ID karyawan tidak valid")
	}
	rows, err := h.svc.ScheduleRows(r.Context(), employeeID)
	if err != nil {
		return err
	}
	// meta.schedule_type lets the calendar decide which red dates are a day
	// off for this employee (EPIC-079 R1-R3); null when the employee is unknown.
	var scheduleType any
	if emp, err := h.svc.repo.LeaveEmployee(r.Context(), employeeID); err != nil {
		return err
	} else if emp != nil {
		scheduleType = emp.Str("schedule_type")
	}
	return ok(w, obj("data", rows, "meta", obj("schedule_type", scheduleType)))
}

func (h handlers) attendanceStats(w http.ResponseWriter, r *http.Request, a *Actor) error {
	q := r.URL.Query()
	month, year := statsPeriod(q.Get("month"), q.Get("year"), h.svc.now())
	out, err := h.svc.AttendanceStats(r.Context(), a, month, year)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}
