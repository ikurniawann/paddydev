package hris

import (
	"context"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
)

// Absen terlewat (EPIC-065 R1, MOM AB-06, K-9): the employee's own days of
// the open attendance periods without a complete attendance, each an
// invitation to file a request (AB-04).

// AttendanceGapRepo reads what the gap scan needs besides the schedule and
// holidays.
type AttendanceGapRepo interface {
	// AttendanceDaysInRange: date, clock_in, clock_out, status of the
	// employee's rows in [start, end] (every row type, absent included).
	AttendanceDaysInRange(ctx context.Context, employeeID, start, end string) ([]*Row, error)
	// ApprovedLeaveRanges: start_date, end_date of approved leaves
	// overlapping [start, end].
	ApprovedLeaveRanges(ctx context.Context, employeeID, start, end string) ([]*Row, error)
	// ApprovedTripRanges: start_date, end_date of approved Perjalanan Dinas
	// overlapping [start, end] (EPIC-065 P4).
	ApprovedTripRanges(ctx context.Context, employeeID, start, end string) ([]*Row, error)
	// EmploymentSpan: join_date and end_date (nullable) of the employee.
	EmploymentSpan(ctx context.Context, employeeID string) (*Row, error)
	PendingAttendanceRequestDates(ctx context.Context, employeeID, start, end string) ([]string, error)
}

// openAttendanceWindow is [start, today]: start is the first day of the
// current attendance period, or of the previous one while that is not
// closed yet (between its end and its closing date). Closing goes through
// periodContaining, which also auto-closes a period past its closing date.
func (s *Service) openAttendanceWindow(ctx context.Context) (start, today string, err error) {
	q := s.repo.querier()
	today = domain.TodayWIB(s.now())
	cur, err := periodContaining(ctx, q, today, today)
	if err != nil {
		return "", "", err
	}
	start = cur.Start
	prev, err := periodContaining(ctx, q, domain.AddDaysISO(cur.Start, -1), today)
	if err != nil {
		return "", "", err
	}
	if !prev.Closed() {
		start = prev.Start
	}
	return start, today, nil
}

// MyAttendanceGaps is GET /api/hris/me/attendance-gaps: the window up to
// yesterday (today is still running) and its gaps.
func (s *Service) MyAttendanceGaps(ctx context.Context, a *Actor) (*Row, error) {
	none := []domain.AttendanceGap{}
	if a.EmployeeID == nil {
		return obj("start", nil, "end", nil, "days", none), nil
	}
	start, today, err := s.openAttendanceWindow(ctx)
	if err != nil {
		return nil, err
	}
	end := domain.AddDaysISO(today, -1)
	if end < start {
		return obj("start", start, "end", end, "days", none), nil
	}
	id := *a.EmployeeID
	pattern, err := s.repo.ShiftPattern(ctx, id)
	if err != nil {
		return nil, err
	}
	holidays, err := s.repo.ActiveHolidays(ctx, start, end)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.AttendanceDaysInRange(ctx, id, start, end)
	if err != nil {
		return nil, err
	}
	leaves, err := s.repo.ApprovedLeaveRanges(ctx, id, start, end)
	if err != nil {
		return nil, err
	}
	trips, err := s.repo.ApprovedTripRanges(ctx, id, start, end)
	if err != nil {
		return nil, err
	}
	span, err := s.repo.EmploymentSpan(ctx, id)
	if err != nil {
		return nil, err
	}
	pending, err := s.repo.PendingAttendanceRequestDates(ctx, id, start, end)
	if err != nil {
		return nil, err
	}
	in := domain.GapInput{Start: start, End: end, Holidays: map[string]bool{}, OnLeave: map[string]bool{},
		Recorded: map[string]domain.RecordedDay{}, Pending: map[string]bool{}}
	for _, r := range pattern {
		in.Schedule = append(in.Schedule, domain.ScheduleRow{DayOfWeek: int(r.Num("day_of_week")), ShiftID: r.StrPtr("shift_id"),
			EffectiveFrom: r.Str("effective_from"), EffectiveTo: r.StrPtr("effective_to")})
	}
	for _, hd := range holidays {
		in.Holidays[hd.Date] = true
	}
	for _, r := range rows {
		rec := domain.RecordedDay{Absent: r.Str("status") == "absent"}
		if t, found := r.Time("clock_in"); found {
			rec.ClockIn = &t
		}
		if t, found := r.Time("clock_out"); found {
			rec.ClockOut = &t
		}
		in.Recorded[r.Str("date")] = rec
	}
	// An approved leave or Perjalanan Dinas day is accounted for.
	for _, ranges := range [][]*Row{leaves, trips} {
		for _, l := range ranges {
			for _, d := range domain.EachDateISO(l.Str("start_date"), l.Str("end_date")) {
				in.OnLeave[d] = true
			}
		}
	}
	for _, d := range pending {
		in.Pending[d] = true
	}
	return obj("start", start, "end", end, "days", withinEmployment(domain.AttendanceGaps(in), span)), nil
}

// withinEmployment drops the working days that fall before the employee's
// join date or after the end date: nobody missed an attendance there. A
// recorded clock-in without a clock-out stays, whatever the dates say.
func withinEmployment(gaps []domain.AttendanceGap, span *Row) []domain.AttendanceGap {
	if span == nil {
		return gaps
	}
	join, last := span.Str("join_date"), span.StrPtr("end_date")
	out := make([]domain.AttendanceGap, 0, len(gaps))
	for _, g := range gaps {
		if g.Kind == domain.GapMissing && ((join != "" && g.Date < join) || (last != nil && *last != "" && g.Date > *last)) {
			continue
		}
		out = append(out, g)
	}
	return out
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

func (s *store) AttendanceDaysInRange(ctx context.Context, employeeID, start, end string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT date::text AS date, clock_in, clock_out, status::text AS status
		FROM hris.attendance WHERE employee_id = $1 AND date BETWEEN $2::date AND $3::date`, employeeID, start, end)
}

func (s *store) ApprovedLeaveRanges(ctx context.Context, employeeID, start, end string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT start_date::text AS start_date, end_date::text AS end_date
		FROM hris.leaves WHERE employee_id = $1 AND status = 'approved'
		  AND start_date <= $3::date AND end_date >= $2::date`, employeeID, start, end)
}

func (s *store) ApprovedTripRanges(ctx context.Context, employeeID, start, end string) ([]*Row, error) {
	return queryRows(ctx, s.db, `SELECT start_date::text AS start_date, end_date::text AS end_date
		FROM hris.business_trips WHERE employee_id = $1 AND status = 'approved'
		  AND start_date <= $3::date AND end_date >= $2::date`, employeeID, start, end)
}

func (s *store) EmploymentSpan(ctx context.Context, employeeID string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT join_date::text AS join_date, end_date::text AS end_date
		FROM hris.employees WHERE id = $1`, employeeID)
	return row, ignoreBadInput(err)
}

func (s *store) PendingAttendanceRequestDates(ctx context.Context, employeeID, start, end string) ([]string, error) {
	rows, err := queryRows(ctx, s.db, `SELECT date::text AS date FROM hris.attendance_requests
		WHERE employee_id = $1 AND status = 'pending' AND date BETWEEN $2::date AND $3::date`, employeeID, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Str("date"))
	}
	return out, nil
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/me/attendance-gaps
func (h handlers) myAttendanceGaps(w http.ResponseWriter, r *http.Request, a *Actor) error {
	out, err := h.svc.MyAttendanceGaps(r.Context(), a)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}
