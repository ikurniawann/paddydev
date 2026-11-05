package domain

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Ketentuan Lembur TnC 03-11-2026 (EPIC-085).

// Batas keras 18 jam/minggu dan 72 jam/periode; batas harian 4 jam dan 12 jam
// kotor dihapus (EPIC-089 R1): lebih dari 4 jam cukup diberi Keterangan.
const (
	OvertimeMaxWeeklyHours = 18
	OvertimeMaxPeriodHours = 72

	OvertimeReasonThresholdHours = 4
	OvertimeLongReasonMin        = 10
)

// OvertimeLongReasonMsg is EPIC-089 R2.
var OvertimeLongReasonMsg = fmt.Sprintf("Lembur lebih dari %d jam: isi Keterangan alasannya (minimal %d karakter)",
	OvertimeReasonThresholdHours, OvertimeLongReasonMin)

// OvertimeReasonError is R2 for the employee's own request: more than 4
// paid hours need a Keterangan of at least 10 characters (trimmed).
func OvertimeReasonError(hours float64, reason string) string {
	if hours <= OvertimeReasonThresholdHours || len([]rune(strings.TrimSpace(reason))) >= OvertimeLongReasonMin {
		return ""
	}
	return OvertimeLongReasonMsg
}

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

// OvertimeCapError is R3 as revised by EPIC-089, checked week → period: the
// used totals (pending + approved, without the row being changed) plus hours
// must stay within 18 / 72 hours.
func OvertimeCapError(hours, weekUsed, periodUsed float64, weekStart, weekEnd, periodLabel string) string {
	switch {
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

// OvertimeSpan is an active overtime entry's clock times (HH:MM) on a date.
type OvertimeSpan struct{ Start, End string }

// OvertimeOverlapError guards a cross-midnight entry now that the daily and
// 12-hour caps are gone (EPIC-089 review): an entry on date may not run into
// the previous day's entry, the next day's entry, or past the next day's
// first shift start (nextShiftStart "" = none). "" when it fits.
func OvertimeOverlapError(date, start, end string, prev, next *OvertimeSpan, nextShiftStart string) string {
	s, ok := overtimeClock(start)
	if !ok {
		return ""
	}
	e := s + OvertimeGrossMinutes(start, end)
	const day = 24 * 60
	if prev != nil {
		if ps, ok := overtimeClock(prev.Start); ok && ps+OvertimeGrossMinutes(prev.Start, prev.End)-day > s {
			return fmt.Sprintf("Lembur bertabrakan dengan lembur %s (%s–%s)",
				ShortDateID(AddDaysISO(date, -1)), clockHM(prev.Start), clockHM(prev.End))
		}
	}
	if next != nil {
		if ns, ok := overtimeClock(next.Start); ok && e > ns+day {
			return fmt.Sprintf("Lembur bertabrakan dengan lembur %s (%s–%s)",
				ShortDateID(AddDaysISO(date, 1)), clockHM(next.Start), clockHM(next.End))
		}
	}
	if shift, ok := overtimeClock(nextShiftStart); ok && e > shift+day {
		return fmt.Sprintf("Lembur melewati jam masuk kerja %s (%s)", ShortDateID(AddDaysISO(date, 1)), clockHM(nextShiftStart))
	}
	return ""
}
