package domain

import (
	"strconv"
	"strings"
	"time"
)

// Input lembur setelah selesai (EPIC-066): pengajuan milik sendiri dibuat
// setelah jam selesai lewat; jam dibayar = durasi − istirahat.

// OvertimeCorrectManagerOnly is the refusal of a time correction by anyone
// but the direct manager (HR only for an employee without one).
const OvertimeCorrectManagerOnly = "Koreksi jam lembur hanya oleh atasan langsung"

var wibZone = time.FixedZone("WIB", 7*3600)

// overtimeClock parses "HH:MM" to minutes after midnight. Named apart from the
// schedule helper clockMinutes, which is lenient and returns no ok flag.
func overtimeClock(s string) (int, bool) {
	p := strings.Split(s, ":")
	if len(p) < 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(p[0])
	m, err2 := strconv.Atoi(p[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// OvertimeGrossMinutes is end − start in minutes, past midnight when the end
// is not after the start (0 on unparsable input).
func OvertimeGrossMinutes(start, end string) int {
	s, ok1 := overtimeClock(start)
	e, ok2 := overtimeClock(end)
	if !ok1 || !ok2 {
		return 0
	}
	if e <= s {
		e += 24 * 60
	}
	return e - s
}

// OvertimeEndsAt is the WIB wall-clock end of the overtime; the next day when
// end is not after start. ok is false on unparsable input.
func OvertimeEndsAt(date, start, end string) (time.Time, bool) {
	day, err := time.ParseInLocation(DateLayout, date, wibZone)
	if err != nil {
		return time.Time{}, false
	}
	s, ok1 := overtimeClock(start)
	if !ok1 {
		return time.Time{}, false
	}
	gross := OvertimeGrossMinutes(start, end)
	if gross == 0 {
		return time.Time{}, false
	}
	return day.Add(time.Duration(s+gross) * time.Minute), true
}

// OvertimeNotFinished: the overtime has not ended yet at now (WIB); unparsable
// input is left to the input validation (false).
func OvertimeNotFinished(date, start, end string, now time.Time) bool {
	ends, ok := OvertimeEndsAt(date, start, end)
	return ok && now.Before(ends)
}

// PaidOvertimeHours is (gross minutes − break) / 60, rounded to two decimals,
// never below zero.
func PaidOvertimeHours(start, end string, breakMinutes int) float64 {
	minutes := OvertimeGrossMinutes(start, end) - breakMinutes
	if minutes < 0 {
		minutes = 0
	}
	return Round2(float64(minutes) / 60)
}

// CanCorrectOvertimeTimes: the direct manager corrects an employee's own
// overtime; HRD only when the employee has no valid manager (nil, pointing
// at themselves, or an inactive employee — EPIC-063 R9). Nobody corrects
// their own overtime.
func CanCorrectOvertimeTimes(actor *string, isHRD bool, ownerID string, reportingTo *string, managerActive bool) (bool, string) {
	if samePerson(actor, &ownerID) {
		return false, "Tidak bisa mengoreksi lembur sendiri"
	}
	hasManager := managerActive && reportingTo != nil && *reportingTo != "" && *reportingTo != ownerID
	if hasManager && samePerson(actor, reportingTo) {
		return true, ""
	}
	if !hasManager && isHRD {
		return true, ""
	}
	return false, OvertimeCorrectManagerOnly
}
