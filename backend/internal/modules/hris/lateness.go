package hris

import (
	"context"
	"net/http"
	"sort"
	"strconv"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
)

/* ── Keterlambatan (EPIC-077) ────────────────────────────────────────── */

// latenessPeriodCount is how many attendance periods ESS can browse: the
// current one and five before it (R3).
const latenessPeriodCount = 6

// latenessBack reads ?back=0..5 (default 0).
func latenessBack(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("back")
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n >= latenessPeriodCount {
		return 0, httpx.BadRequest("Periode tidak tersedia")
	}
	return n, nil
}

// latenessPeriods is the current attendance period and the five before it,
// newest first; each step back is the period holding the day before.
func latenessPeriods(ctx context.Context, q database.Querier, today string) ([]*AttendancePeriod, error) {
	p, err := periodContaining(ctx, q, today, today)
	if err != nil {
		return nil, err
	}
	out := []*AttendancePeriod{p}
	for len(out) < latenessPeriodCount {
		if p, err = periodContaining(ctx, q, domain.AddDaysISO(p.Start, -1), today); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// latenessEnvelope is the period, the browsable periods and the ladder
// every lateness response carries.
func latenessEnvelope(periods []*AttendancePeriod, back int, today string) *Row {
	options := make([]*Row, len(periods))
	for i, p := range periods {
		options[i] = obj("back", i, "name", p.Name(), "label", p.Label())
	}
	return obj("period", periodJSON(periods[back], today), "periods", options, "rules", domain.LatenessLevels)
}

// lateDay is one late attendance row.
type lateDay struct {
	EmployeeID string
	Row        *Row
	Minutes    int64
}

// lateDays lists the late attendance rows of ids within [start, end] by
// date; times are HH:MM WIB.
func lateDays(ctx context.Context, q database.Querier, ids []string, start, end string) ([]lateDay, error) {
	rows, err := q.Query(ctx, `SELECT a.employee_id::text, a.date::text, s.name,
		to_char(a.scheduled_start AT TIME ZONE 'Asia/Jakarta', 'HH24:MI'),
		to_char(a.clock_in AT TIME ZONE 'Asia/Jakarta', 'HH24:MI'),
		coalesce(a.late_minutes, 0)
		FROM hris.attendance a
		LEFT JOIN hris.shifts s ON s.id = a.shift_id
		WHERE a.employee_id = ANY($1::uuid[]) AND a.is_late AND a.date BETWEEN $2::date AND $3::date
		ORDER BY a.date, a.clock_in`, ids, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []lateDay{}
	for rows.Next() {
		var d lateDay
		var date string
		var shift, scheduled, clockIn *string
		if err := rows.Scan(&d.EmployeeID, &date, &shift, &scheduled, &clockIn, &d.Minutes); err != nil {
			return nil, err
		}
		d.Row = obj("date", date, "shift_name", shift, "scheduled_start", scheduled, "clock_in", clockIn, "late_minutes", d.Minutes)
		out = append(out, d)
	}
	return out, rows.Err()
}

// setLateness sets count, total_minutes, level, next and days of a list of
// late days on out.
func setLateness(out *Row, days []*Row, minutes int64) {
	count := len(days)
	var next any
	if l, remaining, ok := domain.NextLatenessLevel(count); ok {
		next = obj("code", l.Code, "label", l.Label, "at", l.Min, "remaining", remaining)
	}
	out.Set("count", count)
	out.Set("total_minutes", minutes)
	out.Set("level", domain.LatenessLevelFor(count))
	out.Set("next", next)
	out.Set("days", days)
}

// MyLateness is GET /api/hris/me/lateness: the signed-in employee's late
// days in the chosen period (R2–R5). An account without an employee gets
// zero.
func (s *Service) MyLateness(ctx context.Context, a *Actor, back int) (*Row, error) {
	q := s.repo.querier()
	today := domain.TodayWIB(s.now())
	periods, err := latenessPeriods(ctx, q, today)
	if err != nil {
		return nil, err
	}
	p := periods[back]
	days := []*Row{}
	var minutes int64
	if a.EmployeeID != nil {
		late, err := lateDays(ctx, q, []string{*a.EmployeeID}, p.Start, p.End)
		if err != nil {
			return nil, err
		}
		for _, d := range late {
			days = append(days, d.Row)
			minutes += d.Minutes
		}
	}
	out := latenessEnvelope(periods, back, today)
	setLateness(out, days, minutes)
	return out, nil
}

// TeamLateness is GET /api/hris/me/team-lateness: every member of the
// manager's team (EPIC-074 teamAttendees) with their late days in the
// chosen period, most late first, and the member count per level (R6).
func (s *Service) TeamLateness(ctx context.Context, a *Actor, back int) (*Row, error) {
	q := s.repo.querier()
	today := domain.TodayWIB(s.now())
	periods, err := latenessPeriods(ctx, q, today)
	if err != nil {
		return nil, err
	}
	p := periods[back]
	perLevel := map[string]int{}
	summary := func() *Row {
		row := newRow()
		for _, l := range domain.LatenessLevels {
			row.Set(l.Code, perLevel[l.Code])
		}
		return row
	}
	out := latenessEnvelope(periods, back, today)
	out.Set("summary", summary())
	out.Set("employees", []*Row{})
	if a.EmployeeID == nil {
		return out, nil
	}
	members, err := teamAttendees(ctx, q, *a.EmployeeID)
	if err != nil || len(members) == 0 {
		return out, err
	}
	ids := make([]string, len(members))
	for i, m := range members {
		ids[i] = m.ID
	}
	names, err := teamMemberNames(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	late, err := lateDays(ctx, q, ids, p.Start, p.End)
	if err != nil {
		return nil, err
	}
	days := map[string][]*Row{}
	minutes := map[string]int64{}
	for _, d := range late {
		days[d.EmployeeID] = append(days[d.EmployeeID], d.Row)
		minutes[d.EmployeeID] += d.Minutes
	}
	type member struct {
		row   *Row
		count int
		name  string
	}
	list := make([]member, 0, len(members))
	for _, m := range members {
		mine := days[m.ID]
		if mine == nil {
			mine = []*Row{}
		}
		row := obj("employee_id", m.ID, "full_name", names[m.ID].name, "position_title", names[m.ID].position,
			"direct", m.Direct, "reporting_to_name", m.ReportingToName)
		setLateness(row, mine, minutes[m.ID])
		perLevel[domain.LatenessLevelFor(len(mine)).Code]++
		list = append(list, member{row, len(mine), names[m.ID].name})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].name < list[j].name
	})
	rows := make([]*Row, len(list))
	for i, m := range list {
		rows[i] = m.row
	}
	out.Set("summary", summary())
	out.Set("employees", rows)
	return out, nil
}

type memberName struct {
	name     string
	position *string
}

// teamMemberNames is the full name and position title of each id.
func teamMemberNames(ctx context.Context, q database.Querier, ids []string) (map[string]memberName, error) {
	rows, err := q.Query(ctx, `SELECT e.id::text, e.full_name, p.title FROM hris.employees e
		LEFT JOIN hris.positions p ON p.id = e.job_title_id WHERE e.id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]memberName{}
	for rows.Next() {
		var id string
		var m memberName
		if err := rows.Scan(&id, &m.name, &m.position); err != nil {
			return nil, err
		}
		out[id] = m
	}
	return out, rows.Err()
}

// GET /api/hris/me/lateness?back=0..5
func (h handlers) myLateness(w http.ResponseWriter, r *http.Request, a *Actor) error {
	back, err := latenessBack(r)
	if err != nil {
		return err
	}
	out, err := h.svc.MyLateness(r.Context(), a, back)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}

// GET /api/hris/me/team-lateness?back=0..5
func (h handlers) teamLateness(w http.ResponseWriter, r *http.Request, a *Actor) error {
	back, err := latenessBack(r)
	if err != nil {
		return err
	}
	out, err := h.svc.TeamLateness(r.Context(), a, back)
	if err != nil {
		return err
	}
	return ok(w, dataOf(out))
}
