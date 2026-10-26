package hris

import (
	"context"
	"net/http"
	"sort"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
)

// Ringkasan absen terlewat (EPIC-067 R4, R5, R9): for one date D, every
// employee's gaps by the rules of the ESS scan (MyAttendanceGaps), loaded
// set-based for all employees at once.

// AttendanceGapSummaryRepo loads the gap scan's inputs of one date for
// every employee.
type AttendanceGapSummaryRepo interface {
	// GapSummaryEmployees: id, full_name, nip, department_name, join_date,
	// end_date of the active employees employed on date (R4), super admin
	// account shells excluded.
	GapSummaryEmployees(ctx context.Context, date string, superAdmins []string) ([]*Row, error)
	// GapSummarySchedules: employee_id, day_of_week, shift_id,
	// effective_from, effective_to of the schedule rows that can apply on
	// date (weekday dow, effective range holding date).
	GapSummarySchedules(ctx context.Context, date string, dow int) ([]*Row, error)
	// AttendanceOn: employee_id, clock_in, clock_out, status of the
	// attendance rows of date.
	AttendanceOn(ctx context.Context, date string) ([]*Row, error)
	// GapSummaryExcused: the employees with an approved leave or Perjalanan
	// Dinas covering date.
	GapSummaryExcused(ctx context.Context, date string) ([]string, error)
	// PendingAttendanceRequestsOn: the employees with a pending attendance
	// request for date.
	PendingAttendanceRequestsOn(ctx context.Context, date string) ([]string, error)
}

// GapSummaryEmployee is one employee's gap on the summary date.
type GapSummaryEmployee struct {
	EmployeeID string  `json:"employee_id"`
	Name       string  `json:"name"`
	NIP        *string `json:"nip"`
	Department *string `json:"department"`
	// Kind is domain.GapMissing or domain.GapNoClockOut.
	Kind string `json:"kind"`
	// ClockIn is the recorded clock-in, "HH:MM" WIB (no_clock_out only).
	ClockIn *string `json:"clock_in"`
}

// GapSummary is the HR summary of one date. Unscheduled counts the
// employees without a schedule row in force (R5); they are never missing.
type GapSummary struct {
	Date         string               `json:"date"`
	Missing      int                  `json:"missing"`
	NoClockOut   int                  `json:"no_clock_out"`
	Unscheduled  int                  `json:"unscheduled"`
	PeriodClosed bool                 `json:"period_closed"`
	Employees    []GapSummaryEmployee `json:"employees"`
}

// AttendanceGapSummary summarizes the gaps of date ("YYYY-MM-DD") on r, the
// module's store or the Repository of the caller's InTx. PeriodClosed comes
// from periodContaining, which materializes the period and auto-closes it
// past its closing date like every other period read.
func (s *Service) AttendanceGapSummary(ctx context.Context, r Repository, date string) (*GapSummary, error) {
	q := r.querier()
	admins, err := s.directory.SuperAdminUserIDs(ctx, q)
	if err != nil {
		return nil, err
	}
	employees, err := r.GapSummaryEmployees(ctx, date, admins)
	if err != nil {
		return nil, err
	}
	scheduleRows, err := r.GapSummarySchedules(ctx, date, domain.IsoDayOfWeek(date))
	if err != nil {
		return nil, err
	}
	attendance, err := r.AttendanceOn(ctx, date)
	if err != nil {
		return nil, err
	}
	excused, err := r.GapSummaryExcused(ctx, date)
	if err != nil {
		return nil, err
	}
	pending, err := r.PendingAttendanceRequestsOn(ctx, date)
	if err != nil {
		return nil, err
	}
	holidays, err := r.ActiveHolidays(ctx, date, date)
	if err != nil {
		return nil, err
	}
	period, err := periodContaining(ctx, q, date, domain.TodayWIB(s.now()))
	if err != nil {
		return nil, err
	}

	schedules := map[string][]domain.ScheduleRow{}
	for _, row := range scheduleRows {
		id := row.Str("employee_id")
		schedules[id] = append(schedules[id], domain.ScheduleRow{DayOfWeek: int(row.Num("day_of_week")),
			ShiftID: row.StrPtr("shift_id"), EffectiveFrom: row.Str("effective_from"), EffectiveTo: row.StrPtr("effective_to")})
	}
	recorded := map[string]domain.RecordedDay{}
	for _, row := range attendance {
		rec := domain.RecordedDay{Absent: row.Str("status") == "absent"}
		if t, found := row.Time("clock_in"); found {
			rec.ClockIn = &t
		}
		if t, found := row.Time("clock_out"); found {
			rec.ClockOut = &t
		}
		recorded[row.Str("employee_id")] = rec
	}
	onLeave := idSet(excused)
	waiting := idSet(pending)
	isHoliday := len(holidays) > 0

	out := &GapSummary{Date: date, PeriodClosed: period.Closed(), Employees: []GapSummaryEmployee{}}
	for _, e := range employees {
		id := e.Str("id")
		in := domain.GapInput{Start: date, End: date, Schedule: schedules[id],
			Holidays: map[string]bool{date: isHoliday}, OnLeave: map[string]bool{date: onLeave[id]},
			Recorded: map[string]domain.RecordedDay{}, Pending: map[string]bool{date: waiting[id]}}
		if rec, found := recorded[id]; found {
			in.Recorded[date] = rec
		}
		if domain.ResolveScheduleRow(in.Schedule, date) == -1 {
			out.Unscheduled++
		}
		for _, g := range withinEmployment(domain.AttendanceGaps(in), e) {
			switch g.Kind {
			case domain.GapMissing:
				out.Missing++
			case domain.GapNoClockOut:
				out.NoClockOut++
			}
			out.Employees = append(out.Employees, GapSummaryEmployee{EmployeeID: id, Name: e.Str("full_name"),
				NIP: e.StrPtr("nip"), Department: e.StrPtr("department_name"), Kind: g.Kind, ClockIn: g.ClockIn})
		}
	}
	rank := map[string]int{domain.GapMissing: 0, domain.GapNoClockOut: 1}
	sort.SliceStable(out.Employees, func(i, j int) bool {
		a, b := out.Employees[i], out.Employees[j]
		if rank[a.Kind] != rank[b.Kind] {
			return rank[a.Kind] < rank[b.Kind]
		}
		return a.Name < b.Name
	})
	return out, nil
}

func idSet(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

func (s *store) GapSummaryEmployees(ctx context.Context, date string, superAdmins []string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT e.id::text AS id, e.full_name, e.nip, d.name AS department_name,
		       e.join_date::text AS join_date, e.end_date::text AS end_date
		FROM hris.employees e
		LEFT JOIN hris.departments d ON d.id = e.department_id
		WHERE e.is_active
		  AND e.join_date <= $1::date
		  AND (e.end_date IS NULL OR e.end_date >= $1::date)
		  AND (e.user_id IS NULL OR NOT (e.user_id = ANY($2::uuid[])))
		ORDER BY e.full_name, e.id`, date, nonNil(superAdmins))
}

func (s *store) GapSummarySchedules(ctx context.Context, date string, dow int) ([]*Row, error) {
	// The ESS scan's ShiftPattern rows narrowed to the candidates of
	// ResolveScheduleRow, in the same per-employee order.
	return queryRows(ctx, s.db, `SELECT es.employee_id::text AS employee_id, es.day_of_week, es.shift_id::text AS shift_id,
		       es.effective_from::text AS effective_from, es.effective_to::text AS effective_to
		FROM hris.v_employee_schedule es
		WHERE es.day_of_week = $2
		  AND es.effective_from <= $1::date
		  AND (es.effective_to IS NULL OR es.effective_to >= $1::date)
		ORDER BY es.employee_id, es.priority`, date, dow)
}

func (s *store) AttendanceOn(ctx context.Context, date string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT employee_id::text AS employee_id, clock_in, clock_out, status::text AS status
		FROM hris.attendance WHERE date = $1::date`, date)
}

func (s *store) GapSummaryExcused(ctx context.Context, date string) ([]string, error) {
	return queryIDs(ctx, s, `SELECT employee_id::text AS employee_id FROM hris.leaves
		WHERE status = 'approved' AND start_date <= $1::date AND end_date >= $1::date
		UNION
		SELECT employee_id::text AS employee_id FROM hris.business_trips
		WHERE status = 'approved' AND start_date <= $1::date AND end_date >= $1::date`, date)
}

func (s *store) PendingAttendanceRequestsOn(ctx context.Context, date string) ([]string, error) {
	return queryIDs(ctx, s, `SELECT employee_id::text AS employee_id FROM hris.attendance_requests
		WHERE status = 'pending' AND date = $1::date`, date)
}

// queryIDs runs a query whose rows carry an employee_id.
func queryIDs(ctx context.Context, s *store, sql string, args ...any) ([]string, error) {
	rows, err := queryRows(ctx, s.db, sql, args...)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Str("employee_id"))
	}
	return out, nil
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/attendance/gap-summary?date=YYYY-MM-DD (R9): yesterday (WIB)
// when date is empty.
func (h handlers) attendanceGapSummary(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	today := domain.TodayWIB(h.svc.now())
	date := r.URL.Query().Get("date")
	if date == "" {
		date = domain.AddDaysISO(today, -1)
	}
	if _, valid := domain.ParseDate(date); !valid {
		return httpx.BadRequest("Tanggal tidak valid")
	}
	if date > today {
		return httpx.BadRequest("Tanggal tidak boleh di masa depan")
	}
	out, err := h.svc.AttendanceGapSummary(r.Context(), h.svc.repo, date)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}
