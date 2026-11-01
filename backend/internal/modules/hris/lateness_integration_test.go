package hris

import (
	"testing"
)

const (
	myLatenessURL   = "/api/hris/me/lateness"
	teamLatenessURL = "/api/hris/me/team-lateness"
)

// late inserts an attendance row of employee on date (clock-in 08:00 WIB +
// minutes), late or not.
func (h *harness) late(employee, date string, minutes int, isLate bool) {
	h.t.Helper()
	h.exec(`INSERT INTO hris.attendance (employee_id, date, clock_in, scheduled_start, status, is_late, late_minutes)
		VALUES ($1, $2::date, ($2 || ' 08:00+07')::timestamptz + make_interval(mins => $3), ($2 || ' 08:00+07')::timestamptz,
		'present', $4, $5)`, employee, date, minutes, isLate, map[bool]int{true: minutes, false: 0}[isLate])
}

func latenessDays(t *testing.T, m map[string]any) []string {
	t.Helper()
	list, _ := m["days"].([]any)
	out := make([]string, len(list))
	for i, d := range list {
		out[i] = d.(map[string]any)["date"].(string)
	}
	return out
}

func levelCode(m map[string]any) string {
	l, _ := m["level"].(map[string]any)
	s, _ := l["code"].(string)
	return s
}

// EPIC-077 R2–R4: only the employee's own is_late rows inside the period
// count; the level and the next level follow the count.
func TestMyLatenessCountsPeriod(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	h.setNow("2026-10-20 10:00")
	me, id := h.staff("pos", nil, true)
	other := h.employee(emp{})
	h.late(id, "2026-10-09", 45, true)
	h.late(id, "2026-10-02", 12, true)
	h.late(id, "2026-10-05", 20, true)
	h.late(id, "2026-10-06", 3, false)
	h.late(id, "2026-09-30", 30, true)
	h.late(other, "2026-10-07", 15, true)

	c := h.do(&me, "GET", myLatenessURL, nil)
	expect(t, c, 200, "")
	d := c.data()
	if d["count"] != float64(3) || d["total_minutes"] != float64(77) || levelCode(d) != "sp1" {
		t.Fatalf("lateness = %s", c.raw)
	}
	if got := latenessDays(t, d); len(got) != 3 || got[0] != "2026-10-02" || got[1] != "2026-10-05" || got[2] != "2026-10-09" {
		t.Errorf("days = %v", got)
	}
	day := d["days"].([]any)[0].(map[string]any)
	if day["clock_in"] != "08:12" || day["scheduled_start"] != "08:00" || day["late_minutes"] != float64(12) {
		t.Errorf("day = %v", day)
	}
	next, _ := d["next"].(map[string]any)
	if next["code"] != "sp2" || next["at"] != float64(5) || next["remaining"] != float64(2) {
		t.Errorf("next = %v", d["next"])
	}
	periods, _ := d["periods"].([]any)
	if len(periods) != 6 || periods[5].(map[string]any)["back"] != float64(5) {
		t.Errorf("periods = %v", d["periods"])
	}
	if p, _ := d["period"].(map[string]any); p["start_date"] != "2026-10-01" || p["end_date"] != "2026-10-31" {
		t.Errorf("period = %v", d["period"])
	}
	if rules, _ := d["rules"].([]any); len(rules) != 5 {
		t.Errorf("rules = %v", d["rules"])
	}
}

// R3: back steps to earlier periods; anything outside 0..5 is refused.
func TestMyLatenessBack(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	h.setNow("2026-10-20 10:00")
	me, id := h.staff("pos", nil, true)
	h.late(id, "2026-09-30", 30, true)

	c := h.do(&me, "GET", myLatenessURL+"?back=1", nil)
	expect(t, c, 200, "")
	if p, _ := c.data()["period"].(map[string]any); p["start_date"] != "2026-09-01" || c.data()["count"] != float64(1) {
		t.Fatalf("back=1 = %s", c.raw)
	}
	for _, bad := range []string{"6", "-1", "x"} {
		expect(t, h.do(&me, "GET", myLatenessURL+"?back="+bad, nil), 400, "Periode tidak tersedia")
		expect(t, h.do(&me, "GET", teamLatenessURL+"?back="+bad, nil), 400, "Periode tidak tersedia")
	}
}

// R3: with cutoff 20 the 21st belongs to the next period.
func TestMyLatenessCutoff20(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 20, closing_offset_days = 3`)
	h.exec(`DELETE FROM hris.attendance_periods WHERE end_date >= '2026-08-01'`)
	h.setNow("2026-11-02 10:00")
	me, id := h.staff("pos", nil, true)
	h.late(id, "2026-10-21", 10, true)
	h.late(id, "2026-10-20", 10, true)

	cur := h.do(&me, "GET", myLatenessURL, nil)
	expect(t, cur, 200, "")
	if got := latenessDays(t, cur.data()); len(got) != 1 || got[0] != "2026-10-21" {
		t.Fatalf("current = %s", cur.raw)
	}
	prev := h.do(&me, "GET", myLatenessURL+"?back=1", nil)
	expect(t, prev, 200, "")
	if got := latenessDays(t, prev.data()); len(got) != 1 || got[0] != "2026-10-20" {
		t.Fatalf("previous = %s", prev.raw)
	}
}

// R5: an account without an employee gets zero, not an error.
func TestMyLatenessUnlinked(t *testing.T) {
	h := newHarness(t)
	me, _ := h.staff("pos", nil, false)
	c := h.do(&me, "GET", myLatenessURL, nil)
	expect(t, c, 200, "")
	if c.data()["count"] != float64(0) || len(latenessDays(t, c.data())) != 0 || levelCode(c.data()) != "none" {
		t.Fatalf("unlinked = %s", c.raw)
	}
	team := h.do(&me, "GET", teamLatenessURL, nil)
	expect(t, team, 200, "")
	if list, _ := team.data()["employees"].([]any); list == nil || len(list) != 0 {
		t.Fatalf("unlinked team = %s", team.raw)
	}
}

// R6: the manager sees the whole team (direct and indirect), most late
// first, never outsiders; the summary counts members per level.
func TestTeamLateness(t *testing.T) {
	h := newHarness(t)
	h.exec(`UPDATE hris.attendance_settings SET cutoff_day = 0, closing_offset_days = 3`)
	h.setNow("2026-10-20 10:00")
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Tim"})
	coord := h.employee(emp{name: "Koordinator K", boss: &bossID})
	staff := h.employee(emp{name: "Staf S", boss: &coord})
	quiet := h.employee(emp{name: "Anggota Tepat", boss: &bossID})
	outsider := h.employee(emp{name: "Orang Luar"})
	for _, d := range []string{"01", "02", "05", "06", "07", "08", "09"} {
		h.late(staff, "2026-10-"+d, 11, true)
	}
	h.late(coord, "2026-10-03", 25, true)
	h.late(outsider, "2026-10-03", 25, true)
	h.late(bossID, "2026-10-03", 25, true)

	c := h.do(&boss, "GET", teamLatenessURL, nil)
	expect(t, c, 200, "")
	list, _ := c.data()["employees"].([]any)
	if len(list) != 3 {
		t.Fatalf("team = %s", c.raw)
	}
	first, second, third := list[0].(map[string]any), list[1].(map[string]any), list[2].(map[string]any)
	if first["employee_id"] != staff || levelCode(first) != "sp3" || first["count"] != float64(7) ||
		first["direct"] != false || first["reporting_to_name"] != "Koordinator K" || first["full_name"] != "Staf S" {
		t.Errorf("first = %v", first)
	}
	if second["employee_id"] != coord || levelCode(second) != "teguran" || second["direct"] != true {
		t.Errorf("second = %v", second)
	}
	if third["employee_id"] != quiet || third["count"] != float64(0) || len(latenessDays(t, third)) != 0 {
		t.Errorf("third = %v", third)
	}
	sum, _ := c.data()["summary"].(map[string]any)
	if sum["sp3"] != float64(1) || sum["teguran"] != float64(1) || sum["none"] != float64(1) || sum["sp1"] != float64(0) {
		t.Errorf("summary = %v", sum)
	}

	lonely, _ := h.staff("pos", nil, true)
	solo := h.do(&lonely, "GET", teamLatenessURL, nil)
	expect(t, solo, 200, "")
	if l, _ := solo.data()["employees"].([]any); len(l) != 0 {
		t.Errorf("no team = %s", solo.raw)
	}
}
