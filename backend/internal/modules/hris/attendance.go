package hris

import (
	"context"
	"math"
	"time"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/httpx"
)

// Attendance: lib/hris/attendance-repo, attendance-roster, daily-roster.
// Clock-in/out is in attendance_clock.go.

// AttendanceRepo is the attendance storage.
type AttendanceRepo interface {
	AttendancePage(ctx context.Context, p attendanceListParams) ([]*Row, int64, error)
	AttendanceDetail(ctx context.Context, id string) (*Row, error)
	UpdateAttendance(ctx context.Context, id string, f fields) (*Row, error)
	DeleteAttendance(ctx context.Context, id string) error
	// RosterRows: employeeIDs nil = every active employee, else only those.
	RosterRows(ctx context.Context, date string, dow int, superAdmins, employeeIDs []string) ([]*Row, error)
	ActiveShifts(ctx context.Context) ([]*Row, error)
	HolidaysOn(ctx context.Context, date string) ([]*Row, error)
	AttendanceMonth(ctx context.Context, month, year float64, employeeID *string) (*Row, error)
	AttendanceToday(ctx context.Context, employeeID *string) (*Row, error)
	ActiveEmployeeCount(ctx context.Context, superAdmins []string) (int64, error)
	ScheduleRows(ctx context.Context, employeeID string) ([]*Row, error)
}

// attendanceListParams is AttendanceListParams.
type attendanceListParams struct {
	EmployeeID, Date, StartDate, EndDate, Status *string
	// BranchID filters by store; "none" is the rows without a store.
	BranchID    *string
	LateOnly    bool
	Page, Limit int
}

// resolveAttendanceListParams: employee_id=me is the actor's record; non-HR
// are forced to their own record. nil (no error) means an empty list.
func resolveAttendanceListParams(q domain.Query, a *Actor) (*attendanceListParams, error) {
	get := func(k string) *string {
		if v, ok := q(k); ok {
			return &v
		}
		return nil
	}
	employeeID := get("employee_id")
	if employeeID != nil && *employeeID == "me" {
		if a.EmployeeID == nil {
			return nil, nil
		}
		employeeID = a.EmployeeID
	}
	if !a.IsHR {
		if a.EmployeeID == nil {
			return nil, httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
		}
		employeeID = a.EmployeeID
	}
	page, _ := q("page")
	limit, _ := q("limit")
	late, _ := q("is_late")
	return &attendanceListParams{
		EmployeeID: employeeID, Date: get("date"), StartDate: get("start_date"), EndDate: get("end_date"),
		BranchID: get("branch_id"),
		Status:   get("status"), LateOnly: late == "true",
		Page: max(1, domain.IntOr(page, 1)), Limit: domain.Clamp(domain.IntOr(limit, 20), 1, 1000),
	}, nil
}

func (s *Service) ListAttendance(ctx context.Context, p attendanceListParams) ([]*Row, int64, error) {
	return s.repo.AttendancePage(ctx, p)
}

// Attendance is getAttendance: non-HR only see their own (404 otherwise, so
// other ids do not leak).
func (s *Service) Attendance(ctx context.Context, id string, a *Actor) (*Row, error) {
	row, err := s.repo.AttendanceDetail(ctx, id)
	if err != nil || row == nil {
		return nil, errOr(ignoreBadInput(err), httpx.NotFound("Attendance not found"))
	}
	if !a.IsHR && (a.EmployeeID == nil || row.Str("employee_id") != *a.EmployeeID) {
		return nil, httpx.NotFound("Attendance not found")
	}
	return row, nil
}

/* ── Daily roster ────────────────────────────────────────────────────── */

var rosterOrder = []string{"scheduled", domain.RosterHadir, domain.RosterTerlambat, domain.RosterBelumAbsen,
	domain.RosterAbsen, domain.RosterCuti, domain.RosterLiburNasional, domain.RosterLibur, domain.RosterTanpaJadwal}

// buildRoster turns roster rows into entries and per-status counts.
func buildRoster(rows []*Row, date, today string, isPublicHoliday bool, now time.Time) (*Row, []*Row) {
	counts := map[string]int{}
	isPast := date < today
	employees := make([]*Row, 0, len(rows))
	for _, row := range rows {
		var shift any
		var times *domain.ShiftTimes
		if id := row.StrPtr("scheduled_shift_id"); id != nil && *id != "" && row.Str("shift_start") != "" && row.Str("shift_end") != "" {
			tol := int(row.Num("late_tolerance_minutes"))
			times = &domain.ShiftTimes{StartTime: row.Str("shift_start"), EndTime: row.Str("shift_end"),
				IsOvernight: row.Bool("is_overnight"), LateToleranceMinutes: tol}
			shift = obj("id", *id, "name", row.Str("shift_name"), "start_time", times.StartTime, "end_time", times.EndTime,
				"late_tolerance_minutes", tol, "is_overnight", times.IsOvernight)
		}
		status := domain.DeriveRosterStatus(domain.RosterStatusInput{
			HasAttendance:   row.Get("attendance_id") != nil,
			IsLate:          row.Bool("is_late"),
			OnApprovedLeave: row.Get("leave_id") != nil,
			HasSchedule:     row.Bool("has_schedule"),
			ShiftID:         row.StrPtr("scheduled_shift_id"),
			IsPastDate:      isPast,
			IsPublicHoliday: isPublicHoliday,
		})
		counts[status]++
		if shift != nil {
			counts["scheduled"]++
		}
		var attendance, leave any
		if row.Get("attendance_id") != nil {
			var hours, minHours any
			if wh := row.Str("work_hours"); wh != "" {
				hours = domain.JSNumber(wh)
			}
			if mh := row.Str("min_work_hours"); mh != "" {
				minHours = domain.JSNumber(mh)
			}
			attendance = obj("id", row.Get("attendance_id"), "clock_in", row.Get("clock_in"), "clock_out", row.Get("clock_out"),
				"work_hours", hours, "is_late", row.Bool("is_late"), "late_minutes", int64(row.Num("late_minutes")),
				"clock_in_photo_url", row.Get("clock_in_photo_url"), "clock_out_photo_url", row.Get("clock_out_photo_url"),
				"shift_id", row.Get("attendance_shift_id"),
				"clock_in_geofence", row.Get("clock_in_geofence"), "clock_in_distance_m", row.Get("clock_in_distance_m"),
				"clock_out_geofence", row.Get("clock_out_geofence"), "clock_out_distance_m", row.Get("clock_out_distance_m"),
				"notes", row.Get("attendance_notes"),
				"is_short_hours", row.Bool("is_short_hours"), "min_work_hours", minHours)
		}
		if row.Get("leave_id") != nil {
			leave = obj("id", row.Get("leave_id"), "leave_type", row.Get("leave_type"))
		}
		employees = append(employees, obj(
			"employee_id", row.Get("employee_id"), "full_name", row.Get("full_name"), "nip", row.Get("nip"),
			"photo_url", row.Get("photo_url"), "department_name", row.Get("department_name"), "job_title", row.Get("job_title"),
			"shift", shift, "status", status, "is_overdue", domain.IsOverdue(now, date, times, status),
			"attendance", attendance, "leave", leave,
		))
	}
	summary := newRow()
	for _, k := range rosterOrder {
		summary.Set(k, counts[k])
	}
	return summary, employees
}

// DailyRoster is loadDailyRoster for the HR monitoring page.
func (s *Service) DailyRoster(ctx context.Context, date, today string) (*Row, error) {
	return s.rosterFor(ctx, date, today, nil)
}

// rosterFor is DailyRoster over employeeIDs (nil = every active employee).
func (s *Service) rosterFor(ctx context.Context, date, today string, employeeIDs []string) (*Row, error) {
	dow := domain.IsoDayOfWeek(date)
	admins, err := s.directory.SuperAdminUserIDs(ctx, s.repo.querier())
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.RosterRows(ctx, date, dow, admins, employeeIDs)
	if err != nil {
		return nil, err
	}
	shifts, err := s.repo.ActiveShifts(ctx)
	if err != nil {
		return nil, err
	}
	holidays, err := s.repo.HolidaysOn(ctx, date)
	if err != nil {
		return nil, err
	}
	summary, employees := buildRoster(rows, date, today, len(holidays) > 0, s.now())
	return obj("date", date, "day_of_week", dow, "is_today", date == today, "summary", summary,
		"shifts", shifts, "holidays", holidays, "employees", employees), nil
}

/* ── Stats and schedule ──────────────────────────────────────────────── */

// statsPeriod is resolveStatsPeriod: Number(param) || the WIB month/year.
func statsPeriod(month, year string, now time.Time) (float64, float64) {
	wib := domain.NowWIB(now)
	m, y := domain.JSNumber(month), domain.JSNumber(year)
	if m == 0 || math.IsNaN(m) {
		m = float64(wib.Month())
	}
	if y == 0 || math.IsNaN(y) {
		y = float64(wib.Year())
	}
	return m, y
}

// AttendanceStats is loadAttendanceStats; non-HR see only their own rows.
func (s *Service) AttendanceStats(ctx context.Context, a *Actor, month, year float64) (*Row, error) {
	var filter *string
	if !a.IsHR {
		if a.EmployeeID == nil {
			return nil, httpx.Forbidden("Akun ini tidak terhubung ke data karyawan")
		}
		filter = a.EmployeeID
	}
	monthly, err := s.repo.AttendanceMonth(ctx, month, year, filter)
	if err != nil {
		return nil, err
	}
	today, err := s.repo.AttendanceToday(ctx, filter)
	if err != nil {
		return nil, err
	}
	var active any
	if a.IsHR {
		admins, err := s.directory.SuperAdminUserIDs(ctx, s.repo.querier())
		if err != nil {
			return nil, err
		}
		n, err := s.repo.ActiveEmployeeCount(ctx, admins)
		if err != nil {
			return nil, err
		}
		active = n
	}
	var avg any
	if v := monthly.Str("avg_work_hours"); v != "" {
		avg = domain.JSNumber(v)
	}
	return obj(
		"month", month, "year", year,
		"present_today", today.Num("present_today"),
		"late_today", today.Num("late_today"),
		"active_employees", active,
		"month_records", monthly.Num("total_records"),
		"month_late", monthly.Num("late_count"),
		"month_off_schedule", monthly.Num("off_schedule"),
		"avg_work_hours", avg,
	), nil
}

func (s *Service) ScheduleRows(ctx context.Context, employeeID string) ([]*Row, error) {
	return s.repo.ScheduleRows(ctx, employeeID)
}
