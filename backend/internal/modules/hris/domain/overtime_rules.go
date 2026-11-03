package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Ketentuan Lembur TnC 03-11-2026 (EPIC-085).

const (
	OvertimeMaxDailyHours  = 4
	OvertimeMaxWeeklyHours = 18
	OvertimeMaxPeriodHours = 72
)

// OvertimeLeaderRefused is R1: no overtime from Supervisor up.
const OvertimeLeaderRefused = "Lembur tidak berlaku untuk jabatan Supervisor ke atas"

// WeekBounds is the Monday and Sunday of the week holding date.
func WeekBounds(date string) (monday, sunday string, ok bool) {
	t, ok := ParseDate(date)
	if !ok {
		return "", "", false
	}
	back := (int(t.Weekday()) + 6) % 7 // Senin = 0
	mon := t.AddDate(0, 0, -back)
	return mon.Format(DateLayout), mon.AddDate(0, 0, 6).Format(DateLayout), true
}

// OvertimeShiftEnd is the end of the employee's last shift on a date.
type OvertimeShiftEnd struct {
	End       string // HH:MM
	Overnight bool
}

// OvertimeStartError is R2: on a working day with a known, same-day shift,
// overtime starts at or after the shift's end. Rest days, overnight shifts,
// days without a schedule and business-trip days are not checked.
func OvertimeStartError(start, dayType string, shift *OvertimeShiftEnd, onTrip bool) string {
	if dayType != OvertimeWeekday || shift == nil || shift.Overnight || onTrip || len(shift.End) < 5 {
		return ""
	}
	end := shift.End[:5]
	if start < end {
		return "Lembur hari kerja dimulai setelah jam kerja selesai (" + end + ")"
	}
	return ""
}

// jam formats hours with a decimal comma, rounded to 2 decimals: 70,5.
func jam(h float64) string {
	return strings.Replace(strconv.FormatFloat(math.Round(h*100)/100, 'f', -1, 64), ".", ",", 1)
}

// OvertimeCapError is R3, checked day → week → period: the used totals
// (pending + approved, without the row being changed) plus hours must stay
// within 4 / 18 / 72 hours.
func OvertimeCapError(hours, dayUsed, weekUsed, periodUsed float64, weekStart, weekEnd, periodLabel string) string {
	switch {
	case dayUsed+hours > OvertimeMaxDailyHours:
		return fmt.Sprintf("Lembur maksimal %d jam per hari", OvertimeMaxDailyHours)
	case weekUsed+hours > OvertimeMaxWeeklyHours:
		return fmt.Sprintf("Lembur minggu %s s.d. %s sudah %s jam; maksimal %d jam per minggu (sisa %s jam)",
			dayMonth(weekStart), dayMonth(weekEnd), jam(weekUsed), OvertimeMaxWeeklyHours, jam(max(0, OvertimeMaxWeeklyHours-weekUsed)))
	case periodUsed+hours > OvertimeMaxPeriodHours:
		return fmt.Sprintf("Lembur periode %s sudah %s jam; maksimal %d jam per periode (sisa %s jam)",
			periodLabel, jam(periodUsed), OvertimeMaxPeriodHours, jam(max(0, OvertimeMaxPeriodHours-periodUsed)))
	}
	return ""
}

// dayMonth is "02-11" of a YYYY-MM-DD date.
func dayMonth(date string) string {
	if t, ok := ParseDate(date); ok {
		return t.Format("02-01")
	}
	return date
}
