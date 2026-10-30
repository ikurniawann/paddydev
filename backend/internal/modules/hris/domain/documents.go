package domain

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

// Pure parts of the HRIS file routes: clock-in lateness, the attendance
// recap labels and ordering (lib/hris/attendance-report.ts), the id-ID date
// formats of lib/format.ts, and the download names of contracts.

// Lateness is computeLateness: past shift start + tolerance is late, and
// the late minutes count from the shift start (rounded up).
func Lateness(clockIn time.Time, dateISO string, s ShiftTimes) (bool, int) {
	start, _ := ScheduledWindow(dateISO, s)
	if !clockIn.After(start.Add(time.Duration(s.LateToleranceMinutes) * time.Minute)) {
		return false, 0
	}
	return true, int(math.Ceil(float64(clockIn.Sub(start).Milliseconds()) / 60000))
}

// monthNamesLong are the Indonesian month names.
var monthNamesLong = []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember"}

var calendarDateRe = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)

// parseCalendarDate reads the parts of "YYYY-MM-DD" without a Date (month
// 0-based), rejecting months outside 1–12 and days outside 1–31.
func parseCalendarDate(s string) (y, m, d int, ok bool) {
	match := calendarDateRe.FindStringSubmatch(s)
	if match == nil {
		return 0, 0, 0, false
	}
	y, _ = strconv.Atoi(match[1])
	m, _ = strconv.Atoi(match[2])
	d, _ = strconv.Atoi(match[3])
	m--
	return y, m, d, m >= 0 && m < 12 && d >= 1 && d <= 31
}

// PeriodLabel is buildPeriodLabel: "1–31 Agustus 2026",
// "25 Agustus – 5 September 2026", or "a s.d. b" for unreadable dates.
func PeriodLabel(start, end string) string {
	sy, smi, sd, ok1 := parseCalendarDate(start)
	ey, emi, ed, ok2 := parseCalendarDate(end)
	if !ok1 || !ok2 {
		return start + " s.d. " + end
	}
	sm, em := monthNamesLong[smi], monthNamesLong[emi]
	switch {
	case sy == ey && sm == em:
		return fmt.Sprintf("%d–%d %s %d", sd, ed, sm, sy)
	case sy == ey:
		return fmt.Sprintf("%d %s – %d %s %d", sd, sm, ed, em, sy)
	}
	return fmt.Sprintf("%d %s %d – %d %s %d", sd, sm, sy, ed, em, ey)
}

var statusLabels = map[string]string{
	"present": "Hadir", "late": "Terlambat", "absent": "Tidak Hadir",
	"leave": "Cuti/Izin", "sick": "Sakit", "holiday": "Libur",
}

// StatusLabel is statusLabel: the Indonesian attendance status, "—" for none.
func StatusLabel(status *string) string {
	if status == nil || *status == "" {
		return "—"
	}
	if l, ok := statusLabels[*status]; ok {
		return l
	}
	return *status
}

// TanggalShort is formatTanggalId: "Kam, 27 Agu 2026" (the input when it is
// not a date).
func TanggalShort(iso string) string { return tanggalID(iso) }

// ClockWIB is formatTime(t, fallback): "08.58" in WIB.
func ClockWIB(t *time.Time, fallback string) string {
	if t == nil {
		return fallback
	}
	return NowWIB(*t).Format("15.04")
}

// DateTimeWIB is formatDateTime and toLocaleString("id-ID", {dateStyle:
// "medium", timeStyle: "short"}) in WIB: "27 Agu 2026, 08.58".
func DateTimeWIB(t *time.Time, fallback string) string {
	if t == nil {
		return fallback
	}
	w := NowWIB(*t)
	return fmt.Sprintf("%d %s %d, %s", w.Day(), idMonths[w.Month()-1], w.Year(), w.Format("15.04"))
}

// LongDateID is toLocaleDateString("id-ID", {day: "numeric", month: "long",
// year: "numeric"}) of a calendar date: "1 Agustus 2026"; ok is false when
// the date is unreadable.
func LongDateID(dateISO string) (string, bool) {
	t, ok := ParseDate(dateISO)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%d %s %d", t.Day(), monthNamesLong[t.Month()-1], t.Year()), true
}

// ReportRow is AttendanceReportRow: one line of the Excel and PDF recap.
type ReportRow struct {
	Date                      string
	EmployeeName              string
	NIP, Department, Position *string
	Branch                    *string // EPIC-075 R2: store of the attendance, nil = none
	ClockIn, ClockOut         *time.Time
	WorkHours                 *float64
	Status                    *string
	IsLate                    bool
	LateMinutes               int
	IsShortHours              bool     // EPIC-067 R2: hris.attendance.is_short_hours
	MinWorkHours              *float64 // EPIC-067 R2: hris.attendance.min_work_hours, nil = no rule
	Notes                     *string
	ClockInPhoto              *string
	ClockOutPhoto             *string
}

var nameCollator = collate.New(language.Und)

// SortReportRows is toReportRows' order: by employee name (localeCompare),
// then date; stable like Array#sort.
func SortReportRows(rows []ReportRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.EmployeeName == b.EmployeeName {
			return a.Date < b.Date
		}
		return nameCollator.CompareString(a.EmployeeName, b.EmployeeName) < 0
	})
}

var combiningMarks = regexp.MustCompile(`[\x{0300}-\x{036f}]`)
var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// ContractFileName is contractFileName: "kontrak-0001-pkwt-vii-2026-budi-santoso.pdf".
func ContractFileName(contractNumber, employeeName string) string {
	slug := norm.NFKD.String(strings.ToLower(contractNumber + " " + employeeName))
	slug = nonSlug.ReplaceAllString(combiningMarks.ReplaceAllString(slug, ""), "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 80 {
		slug = slug[:80]
	}
	if slug == "" {
		slug = "karyawan"
	}
	return "kontrak-" + slug + ".pdf"
}

// SignedDocumentName is the inline name of a signed scan: "ttd-" plus the
// contract number with every UTF-16 unit outside [a-zA-Z0-9.-] as "_".
func SignedDocumentName(contractNumber string) string {
	var b strings.Builder
	b.WriteString("ttd-")
	for _, r := range contractNumber {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		case r > 0xffff:
			b.WriteString("__")
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
