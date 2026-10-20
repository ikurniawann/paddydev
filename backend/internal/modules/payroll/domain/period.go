package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const isoDate = "2006-01-02"

// MonthNamesID are the Indonesian month names (MONTH_NAMES_ID).
var MonthNamesID = [12]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

// PeriodLabelID is periodLabelId: "Januari 2026"; out-of-range months use Januari.
func PeriodLabelID(month *int, year int) string {
	m := 1
	if month != nil {
		m = *month
	}
	name := MonthNamesID[0]
	if m >= 1 && m <= 12 {
		name = MonthNamesID[m-1]
	}
	return fmt.Sprintf("%s %d", name, year)
}

// PeriodBounds is periodBounds: first and last ISO date of the month.
func PeriodBounds(month, year int) (start, end string) {
	last := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC)
	return fmt.Sprintf("%d-%02d-01", year, month), fmt.Sprintf("%d-%02d-%02d", year, month, last.Day())
}

func parseISO(s string) time.Time {
	t, _ := time.Parse(isoDate, s)
	return t
}

// AddDaysISO is addDaysIso.
func AddDaysISO(date string, days int) string {
	return parseISO(date).AddDate(0, 0, days).Format(isoDate)
}

// EachDate is eachDateOfPeriod: inclusive ISO dates.
func EachDate(start, end string) []string {
	var out []string
	last := parseISO(end)
	for d := parseISO(start); !d.After(last); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format(isoDate))
	}
	return out
}

// ShiftRow is EmployeeShiftRow: day_of_week 1=Monday … 7=Sunday, ShiftID nil = day off.
type ShiftRow struct {
	DayOfWeek     int
	ShiftID       *string
	EffectiveFrom string
	EffectiveTo   *string
}

// ISODayOfWeek is isoDayOfWeek.
func ISODayOfWeek(date string) int {
	d := int(parseISO(date).Weekday())
	if d == 0 {
		return 7
	}
	return d
}

// ResolveScheduleRow is resolveScheduleRowForDate: the matching row with the
// latest effective_from, day-off rows included.
func ResolveScheduleRow(rows []ShiftRow, date string) *ShiftRow {
	dow := ISODayOfWeek(date)
	var best *ShiftRow
	for i := range rows {
		r := &rows[i]
		if r.DayOfWeek != dow || r.EffectiveFrom > date || (r.EffectiveTo != nil && *r.EffectiveTo < date) {
			continue
		}
		if best == nil || r.EffectiveFrom > best.EffectiveFrom {
			best = r
		}
	}
	return best
}

// ScheduledDays is countScheduledDays.
func ScheduledDays(rows []ShiftRow, start, end string) (days int, hasSchedule bool) {
	if len(rows) == 0 {
		return 0, false
	}
	for _, d := range EachDate(start, end) {
		if r := ResolveScheduleRow(rows, d); r != nil && r.ShiftID != nil {
			days++
		}
	}
	return days, true
}

// IsWorkDate: a date with a shift in the schedule in force that day; without
// a schedule in force, Monday–Friday that is not a national holiday (the
// hris DescribeLeaveDays rule). Payroll counts leave on work dates only.
func IsWorkDate(date string, schedule []ShiftRow, holidays map[string]bool) bool {
	for _, r := range schedule {
		if r.EffectiveFrom <= date && (r.EffectiveTo == nil || *r.EffectiveTo >= date) {
			row := ResolveScheduleRow(schedule, date)
			return row != nil && row.ShiftID != nil
		}
	}
	dow := ISODayOfWeek(date)
	return dow >= 1 && dow <= 5 && !holidays[date]
}

// LeaveRange is a leave's inclusive date span.
type LeaveRange struct{ Start, End string }

// ClampedLeaveDays is clampedLeaveDays: days of the leave inside the period.
func ClampedLeaveDays(l LeaveRange, start, end string) int {
	from, to := max(l.Start, start), min(l.End, end)
	if from > to {
		return 0
	}
	return int(math.Round(parseISO(to).Sub(parseISO(from)).Hours()/24)) + 1
}

// AttendanceRow is AttendancePeriodRow.
type AttendanceRow struct {
	Date          string
	Status        string
	HasClockOut   bool
	IsLate        *bool
	LateMinutes   any // numeric text or nil
	OvertimeHours any
	// BranchID is the store the day was worked at (nil when not recorded)
	// and WorkHours its work hours (nil without a clock-out); the store
	// allocation of the slip reads them.
	BranchID  *string
	WorkHours *float64
}

// OvertimeRequest is an approved overtime request.
type OvertimeRequest struct {
	Date    string
	Hours   float64
	DayType string // regular | rest_day (hari istirahat mingguan)
}

// OvertimeSplit is splitOvertimeHours' result.
type OvertimeSplit struct{ Regular, Holiday, Total float64 }

func round2(v float64) float64 { return Round(v*100) / 100 }

// SplitOvertime is splitOvertimeHours: realized approved hours (a clock_out
// that day; capped at the system-computed overtime when there is one),
// bucketed into regular days and holidays / rest days.
func SplitOvertime(requests []OvertimeRequest, attendance []AttendanceRow, holidays map[string]bool) OvertimeSplit {
	return SplitOvertimeWith(requests, attendance, holidays, nil, true)
}

// SplitOvertimeWith is SplitOvertime with the EPIC-055 rules. A request falls
// in the holiday bucket on a public holiday, when marked rest_day, or on a day
// without a shift in the employee's schedule (when there is one). With
// requireClockout false the requested hours are paid without an attendance.
func SplitOvertimeWith(requests []OvertimeRequest, attendance []AttendanceRow, holidays map[string]bool,
	schedule []ShiftRow, requireClockout bool) OvertimeSplit {
	byDate := map[string]AttendanceRow{}
	for _, a := range attendance {
		byDate[a.Date] = a
	}
	// A rest day is a weekday without a shift in a schedule that is in force
	// that date; before the schedule starts (or after it ends) nothing is known.
	restDay := func(date string) bool {
		inForce := false
		for _, r := range schedule {
			if r.EffectiveFrom <= date && (r.EffectiveTo == nil || *r.EffectiveTo >= date) {
				inForce = true
				break
			}
		}
		if !inForce {
			return false
		}
		row := ResolveScheduleRow(schedule, date)
		return row == nil || row.ShiftID == nil
	}
	var regular, holiday float64
	for _, req := range requests {
		att, ok := byDate[req.Date]
		if requireClockout && (!ok || !att.HasClockOut) {
			continue
		}
		hours := req.Hours
		if ok {
			if actual := OrZero(att.OvertimeHours); actual > 0 {
				hours = math.Min(req.Hours, actual)
			}
		}
		if holidays[req.Date] || req.DayType == "rest_day" || restDay(req.Date) {
			holiday += hours
		} else {
			regular += hours
		}
	}
	return OvertimeSplit{round2(regular), round2(holiday), round2(regular + holiday)}
}

// HealthEligible is the Odoo INSURANCE condition: service length from the
// join date to the period end, both days counted, of at least minDays.
func HealthEligible(joinDate, periodEnd string, minDays int) bool {
	if minDays <= 0 {
		return true
	}
	join, err1 := time.Parse("2006-01-02", joinDate)
	end, err2 := time.Parse("2006-01-02", periodEnd)
	if err1 != nil || err2 != nil {
		return false
	}
	return int(end.Sub(join).Hours()/24)+1 >= minDays
}

// LateStats is computeLateStats.
func LateStats(rows []AttendanceRow) (days, minutes float64) {
	for _, r := range rows {
		if (r.IsLate != nil && *r.IsLate) || r.Status == "late" {
			days++
			minutes += OrZero(r.LateMinutes)
		}
	}
	return days, minutes
}

// DateRange is an inclusive ISO date span.
type DateRange struct{ Start, End string }

// Coverage is periodCoverage: the overlap of [start, end|open) with the period.
func Coverage(start string, end *string, periodStart, periodEnd string) *DateRange {
	s := max(start, periodStart)
	e := periodEnd
	if end != nil && *end < periodEnd {
		e = *end
	}
	if s > e {
		return nil
	}
	return &DateRange{s, e}
}

// MergeRanges is mergeDateRanges: overlapping or adjacent ranges merge.
func MergeRanges(ranges []DateRange) []DateRange {
	if len(ranges) == 0 {
		return nil
	}
	sorted := append([]DateRange(nil), ranges...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	merged := []DateRange{sorted[0]}
	for _, next := range sorted[1:] {
		last := &merged[len(merged)-1]
		if next.Start <= AddDaysISO(last.End, 1) {
			if next.End > last.End {
				last.End = next.End
			}
			continue
		}
		merged = append(merged, next)
	}
	return merged
}

// DateText normalizes a date column value to "YYYY-MM-DD" (dateColToIso on
// the text form): the first 10 characters when they look like a date.
func DateText(v *string) *string {
	if v == nil || len(*v) < 10 {
		return nil
	}
	s := (*v)[:10]
	if _, err := time.Parse(isoDate, s); err != nil || strings.Count(s, "-") != 2 {
		return nil
	}
	return &s
}
