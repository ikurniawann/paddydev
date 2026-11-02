package app

import (
	"testing"

	hrisdomain "nuhabit/backend/internal/modules/hris/domain"
	paydomain "nuhabit/backend/internal/modules/payroll/domain"
)

// EPIC-079 R4: for shift staff the days a leave takes from the quota (hris
// DescribeRosterLeaveDays) are exactly the dates payroll treats as work dates
// (payroll IsWorkDate with the empty holiday map of shift staff), so quota
// days equal payroll-deducted days.
func TestShiftLeaveQuotaDaysEqualPayrollWorkDates(t *testing.T) {
	ns := "NS"
	day := func(date string, shift *string) (hrisdomain.ScheduleRow, paydomain.ShiftRow) {
		dow := hrisdomain.IsoDayOfWeek(date)
		return hrisdomain.ScheduleRow{DayOfWeek: dow, ShiftID: shift, EffectiveFrom: date, EffectiveTo: &date},
			paydomain.ShiftRow{DayOfWeek: dow, ShiftID: shift, EffectiveFrom: date, EffectiveTo: &date}
	}
	var rosterH []hrisdomain.ScheduleRow
	var rosterP []paydomain.ShiftRow
	for _, d := range []struct {
		date  string
		shift *string
	}{{"2026-12-26", &ns}, {"2026-12-27", nil}, {"2026-12-28", &ns}} {
		h, p := day(d.date, d.shift)
		rosterH, rosterP = append(rosterH, h), append(rosterP, p)
	}
	weeklyFrom := "2026-12-28"
	cases := []struct {
		name       string
		start, end string
		h          []hrisdomain.ScheduleRow
		p          []paydomain.ShiftRow
		want       int
	}{
		{"no roster, Fri-Mon", "2026-12-25", "2026-12-28", nil, nil, 2},
		{"no roster, weekend only", "2026-12-26", "2026-12-27", nil, nil, 0},
		{"no roster, red date Fri-Fri", "2026-12-21", "2026-12-25", nil, nil, 5},
		{"per-date cells, work/off/work", "2026-12-26", "2026-12-28", rosterH, rosterP, 2},
		{"weekly Monday starts later", "2026-12-25", "2026-12-28",
			[]hrisdomain.ScheduleRow{{DayOfWeek: 1, ShiftID: &ns, EffectiveFrom: weeklyFrom}},
			[]paydomain.ShiftRow{{DayOfWeek: 1, ShiftID: &ns, EffectiveFrom: weeklyFrom, Priority: 1}}, 2},
		{"weekly Monday in force, other weekdays off", "2026-12-28", "2027-01-03",
			[]hrisdomain.ScheduleRow{{DayOfWeek: 1, ShiftID: &ns, EffectiveFrom: "2026-01-01"}},
			[]paydomain.ShiftRow{{DayOfWeek: 1, ShiftID: &ns, EffectiveFrom: "2026-01-01", Priority: 1}}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hrisdomain.DescribeRosterLeaveDays(c.start, c.end, c.h)
			payroll := 0
			for _, d := range paydomain.EachDate(c.start, c.end) {
				if paydomain.IsWorkDate(d, c.p, nil) {
					payroll++
				}
			}
			if got != c.want || payroll != c.want {
				t.Fatalf("hris quota days = %d, payroll work dates = %d, want %d", got, payroll, c.want)
			}
		})
	}
}
