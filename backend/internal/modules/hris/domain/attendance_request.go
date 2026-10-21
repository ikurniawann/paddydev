package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Paket Absensi lanjutan (EPIC-065, MOM AB-04/AB-05/AB-06, K-9): pengajuan
// absen manual / koreksi absen yang diputus HR, absen terlewat di periode
// absensi terbuka, dan jam kerja minimum per cabang.

// Request kinds: manual = no attendance row on the date, correction = a row
// exists. Gap kinds: missing = a scheduled working day without a row,
// no_clock_out = a row with a clock-in and no clock-out.
const (
	AttendanceRequestManual     = "manual"
	AttendanceRequestCorrection = "correction"
	GapMissing                  = "missing"
	GapNoClockOut               = "no_clock_out"
)

// AttendanceRequestInput is a request as the employee entered it, with the
// attendance row recorded on its date (HasRow false: none).
type AttendanceRequestInput struct {
	Date              string
	ClockIn, ClockOut *string // "HH:MM" WIB; nil = not requested
	Reason            string
	HasRow            bool
	RecordedIn        *time.Time
	RecordedOut       *time.Time
}

// CheckAttendanceRequest returns the request kind, or the 400 message when
// the request is invalid (EPIC-065 R4/R5). now is the server clock.
func CheckAttendanceRequest(in AttendanceRequestInput, now time.Time) (kind, msg string) {
	if _, ok := ParseDate(in.Date); !ok {
		return "", "Format tanggal tidak valid"
	}
	if in.Date > TodayWIB(now) {
		return "", "Tanggal pengajuan tidak boleh di masa depan"
	}
	if len([]rune(JSTrim(in.Reason))) < 5 {
		return "", "Keterangan wajib diisi (minimal 5 karakter)"
	}
	for _, c := range []*string{in.ClockIn, in.ClockOut} {
		if c != nil && !ValidClock(*c) {
			return "", "Format jam harus HH:MM"
		}
	}
	if in.ClockIn == nil && in.ClockOut == nil {
		return "", "Isi jam masuk dan/atau jam pulang yang diajukan"
	}
	clockIn := in.RecordedIn
	if in.ClockIn != nil {
		t, _ := ClockAt(in.Date, *in.ClockIn)
		clockIn = &t
	}
	if clockIn == nil {
		return "", "Jam masuk wajib diisi"
	}
	clockOut := in.RecordedOut
	if in.ClockOut != nil {
		t, _ := ClockOutAt(in.Date, *in.ClockOut, clockIn)
		clockOut = &t
	}
	if clockIn.After(now) || (clockOut != nil && clockOut.After(now)) {
		return "", "Jam yang diajukan belum terjadi"
	}
	if !in.HasRow {
		return AttendanceRequestManual, ""
	}
	if sameMinute(clockIn, in.RecordedIn) && sameMinute(clockOut, in.RecordedOut) {
		return "", "Jam yang diajukan sama dengan absensi tercatat"
	}
	return AttendanceRequestCorrection, ""
}

// sameMinute compares two optional instants to the minute (a request
// carries "HH:MM", a clock-in carries seconds).
func sameMinute(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Truncate(time.Minute).Equal(b.Truncate(time.Minute))
}

// RecordedDay is the attendance row of a date in the gap scan.
type RecordedDay struct {
	ClockIn, ClockOut *time.Time
	// Absent: HR marked the day "Tidak hadir"; not a gap.
	Absent bool
}

// GapInput is what AttendanceGaps scans, [Start, End] inclusive.
type GapInput struct {
	Start, End string
	Schedule   []ScheduleRow
	Holidays   map[string]bool
	OnLeave    map[string]bool
	Recorded   map[string]RecordedDay
	Pending    map[string]bool
}

// AttendanceGap is a day the employee is invited to request a correction for.
type AttendanceGap struct {
	Date string `json:"date"`
	Kind string `json:"kind"`
	// ClockIn is the recorded clock-in, "HH:MM" WIB (no_clock_out only).
	ClockIn *string `json:"clock_in"`
}

// AttendanceGaps lists, oldest first, the days of the window without a
// complete attendance (MOM AB-06, EPIC-065 R1): a row with no clock-out, or
// a working day of the schedule in force without a row — unless the date is
// an active public holiday, an approved leave day, a day HR marked absent,
// or already has a pending request. Without a schedule only missing
// clock-outs are known.
func AttendanceGaps(in GapInput) []AttendanceGap {
	out := []AttendanceGap{}
	for _, date := range EachDateISO(in.Start, in.End) {
		if in.Pending[date] {
			continue
		}
		if rec, ok := in.Recorded[date]; ok {
			if !rec.Absent && rec.ClockIn != nil && rec.ClockOut == nil {
				clock := ClockWIBText(*rec.ClockIn)
				out = append(out, AttendanceGap{Date: date, Kind: GapNoClockOut, ClockIn: &clock})
			}
			continue
		}
		if in.Holidays[date] || in.OnLeave[date] {
			continue
		}
		if i := ResolveScheduleRow(in.Schedule, date); i >= 0 && in.Schedule[i].ShiftID != nil && *in.Schedule[i].ShiftID != "" {
			out = append(out, AttendanceGap{Date: date, Kind: GapMissing})
		}
	}
	return out
}

// ValidateMinWorkHours: nil (no rule) or 1–24 hours in quarter hours
// (EPIC-065 R7); "" when valid.
func ValidateMinWorkHours(hours *float64) string {
	if hours == nil {
		return ""
	}
	if h := *hours; h < 1 || h > 24 || math.Mod(h*4, 1) != 0 {
		return "Jam kerja minimum 1–24 jam (kelipatan 0,25)"
	}
	return ""
}

// AttendanceRequestNotice is HR's in-app notification of a new request (K-9).
func AttendanceRequestNotice(employeeName, kind, date string, clockIn, clockOut *string) (title, message string) {
	what := "absen manual"
	if kind == AttendanceRequestCorrection {
		what = "koreksi absen"
	}
	var clocks []string
	if clockIn != nil {
		clocks = append(clocks, "masuk "+*clockIn)
	}
	if clockOut != nil {
		clocks = append(clocks, "pulang "+*clockOut)
	}
	return "Pengajuan Absensi: " + employeeName, fmt.Sprintf(
		"%s mengajukan %s %s (%s). Tinjau di HRIS → Absensi → Pengajuan Absensi.",
		employeeName, what, ShortDateID(date), strings.Join(clocks, ", "))
}

// AttendanceDecisionNotice is the employee's in-app notification of HR's
// decision.
func AttendanceDecisionNotice(status, date string, rejection *string) (title, message string) {
	if status == "approved" {
		return "Pengajuan absensi disetujui", "Pengajuan absensi " + ShortDateID(date) + " disetujui HR. Absensi sudah diperbarui."
	}
	message = "Pengajuan absensi " + ShortDateID(date) + " ditolak HR."
	if rejection != nil && *rejection != "" {
		message += " Alasan: " + *rejection
	}
	return "Pengajuan absensi ditolak", message
}
