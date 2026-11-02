package domain

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// ScheduleRow is one hris.employee_shifts pattern row: day 1 = Monday …
// 7 = Sunday, ShiftID nil = day off, dates "YYYY-MM-DD".
type ScheduleRow struct {
	DayOfWeek     int
	ShiftID       *string
	EffectiveFrom string
	EffectiveTo   *string
	Shift2ID      *string // second shift of a two-shift day (EPIC-080), nil when none
}

// ResolveScheduleRow is resolveScheduleRowForDate: the index of the row that
// applies on dateISO (matching weekday, inside its effective range, latest
// effective_from wins), day-off rows included; -1 when no pattern applies.
func ResolveScheduleRow(rows []ScheduleRow, dateISO string) int {
	dow := IsoDayOfWeek(dateISO)
	var candidates []int
	for i, r := range rows {
		if r.DayOfWeek == dow && r.EffectiveFrom <= dateISO && (r.EffectiveTo == nil || *r.EffectiveTo >= dateISO) {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return -1
	}
	// Array.prototype.sort with a comparator that never returns 0 for equal
	// keys still keeps a stable order on ties in V8 (TimSort); mirror it.
	sort.SliceStable(candidates, func(a, b int) bool {
		return rows[candidates[a]].EffectiveFrom > rows[candidates[b]].EffectiveFrom
	})
	return candidates[0]
}

// ShiftTimes are the hris.shifts columns the schedule math needs.
type ShiftTimes struct {
	StartTime            string // "HH:MM:SS"
	EndTime              string
	IsOvernight          bool
	LateToleranceMinutes int
}

func wibDateTime(dateISO, clock string, addDays int) time.Time {
	base, _ := time.Parse(DateLayout, dateISO)
	parts := strings.Split(clock, ":")
	num := func(i int) int {
		if i >= len(parts) {
			return 0
		}
		n, _ := strconv.Atoi(parts[i])
		return n
	}
	return base.AddDate(0, 0, addDays).Add(time.Duration(num(0))*time.Hour - WIBOffset +
		time.Duration(num(1))*time.Minute + time.Duration(num(2))*time.Second)
}

// ScheduledWindow is the shift's start and end (UTC) for a shift starting on
// dateISO; an overnight shift ends the next day.
func ScheduledWindow(dateISO string, s ShiftTimes) (start, end time.Time) {
	add := 0
	if s.IsOvernight {
		add = 1
	}
	return wibDateTime(dateISO, s.StartTime, 0), wibDateTime(dateISO, s.EndTime, add)
}

// TwoShiftWindow is the scheduled window of a two-shift day (EPIC-080 R8):
// start of the first shift, end of the second, and the idle gap in minutes
// between the first shift's end and the second's start. A second shift whose
// clock time starts before the first's runs on the next calendar day (only a
// first overnight shift passes TwoShiftIssue that way; kept for rows stored
// before that rule). A
// second shift that does not start after the first ends has no gap (0).
func TwoShiftWindow(dateISO string, first, second ShiftTimes) (start, end time.Time, gapMinutes int) {
	start, firstEnd := ScheduledWindow(dateISO, first)
	secondStart, secondEnd := ScheduledWindow(dateISO, second)
	if secondStart.Before(start) {
		secondStart, secondEnd = secondStart.AddDate(0, 0, 1), secondEnd.AddDate(0, 0, 1)
	}
	if gap := secondStart.Sub(firstEnd); gap > 0 {
		gapMinutes = int(gap / time.Minute)
	}
	return start, secondEnd, gapMinutes
}

// Roster statuses, in summary order.
const (
	RosterHadir         = "hadir"
	RosterTerlambat     = "terlambat"
	RosterBelumAbsen    = "belum_absen"
	RosterAbsen         = "absen"
	RosterCuti          = "cuti"
	RosterLiburNasional = "libur_nasional"
	RosterLibur         = "libur"
	RosterTanpaJadwal   = "tanpa_jadwal"
)

// RosterStatusInput is RosterStatusInput in lib/hris/daily-roster.
type RosterStatusInput struct {
	HasAttendance   bool
	IsLate          bool
	OnApprovedLeave bool
	HasSchedule     bool
	ShiftID         *string
	IsPastDate      bool
	IsPublicHoliday bool
}

// DeriveRosterStatus: attendance beats leave, leave beats a public holiday,
// a holiday beats the shift pattern; "absen" only for past dates.
func DeriveRosterStatus(in RosterStatusInput) string {
	switch {
	case in.HasAttendance && in.IsLate:
		return RosterTerlambat
	case in.HasAttendance:
		return RosterHadir
	case in.OnApprovedLeave:
		return RosterCuti
	case in.IsPublicHoliday:
		return RosterLiburNasional
	case in.HasSchedule && in.ShiftID != nil && *in.ShiftID != "":
		if in.IsPastDate {
			return RosterAbsen
		}
		return RosterBelumAbsen
	case in.HasSchedule:
		return RosterLibur
	}
	return RosterTanpaJadwal
}

// IsOverdue: a "belum_absen" employee past shift start + tolerance.
func IsOverdue(now time.Time, dateISO string, shift *ShiftTimes, status string) bool {
	if status != RosterBelumAbsen || shift == nil {
		return false
	}
	start, _ := ScheduledWindow(dateISO, *shift)
	return now.After(start.Add(time.Duration(shift.LateToleranceMinutes) * time.Minute))
}
