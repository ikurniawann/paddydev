package domain

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Holiday is an active hris.public_holidays row.
type Holiday struct {
	Date         string
	Name         string
	Type         string
	DeductsLeave bool
	// IsDayOff: cuti bersama yang ditetapkan libur oleh perusahaan (EPIC-079 R2).
	IsDayOff bool
}

// HolidayDayOff reports whether h is a day off for an employee of the given
// schedule type (EPIC-079 R1-R3). Shift staff ("shift") ignore every red date:
// their roster decides. Non-Shift staff rest on nasional and perusahaan days,
// and on cuti bersama only when the company set it as a day off.
func HolidayDayOff(h Holiday, scheduleType string) bool {
	if scheduleType == "shift" {
		return false
	}
	if h.Type == "cuti_bersama" {
		return h.IsDayOff
	}
	return true
}

// DayOffDates is the set of dates that are a day off for scheduleType.
func DayOffDates(rows []Holiday, scheduleType string) map[string]bool {
	out := map[string]bool{}
	for _, h := range rows {
		if HolidayDayOff(h, scheduleType) {
			out[h.Date] = true
		}
	}
	return out
}

// HolidayIndex groups holidays by date.
type HolidayIndex map[string][]Holiday

// IndexHolidays is indexHolidays.
func IndexHolidays(rows []Holiday) HolidayIndex {
	idx := HolidayIndex{}
	for _, h := range rows {
		idx[h.Date] = append(idx[h.Date], h)
	}
	return idx
}

// IsWeekend: Saturday or Sunday.
func IsWeekend(dateISO string) bool { return IsoDayOfWeek(dateISO) >= 6 }

// ExcludedHoliday names a holiday that kept a date from consuming leave.
type ExcludedHoliday struct {
	Date string `json:"date"`
	Name string `json:"name"`
}

// DescribeLeaveDays counts the days in [start, end] that consume annual
// leave: weekends and holidays with deducts_leave = false are skipped (a
// national holiday beats cuti bersama on the same date); cuti bersama still
// counts. It also lists the holidays that were skipped.
func DescribeLeaveDays(startISO, endISO string, idx HolidayIndex) (int, []ExcludedHoliday) {
	excluded := []ExcludedHoliday{}
	total := 0
	for _, d := range EachDateISO(startISO, endISO) {
		var exempt *Holiday
		for i := range idx[d] {
			if !idx[d][i].DeductsLeave {
				exempt = &idx[d][i]
				break
			}
		}
		if exempt != nil {
			excluded = append(excluded, ExcludedHoliday{Date: d, Name: exempt.Name})
			continue
		}
		if IsWeekend(d) {
			continue
		}
		total++
	}
	return total, excluded
}

// NoWorkingDayMessage explains a leave range with zero working days.
func NoWorkingDayMessage(excluded []ExcludedHoliday) string {
	alasan := "jatuh pada akhir pekan"
	if len(excluded) > 0 {
		names := make([]string, len(excluded))
		for i, h := range excluded {
			names[i] = h.Name
		}
		alasan = "sudah hari libur (" + strings.Join(names, ", ") + ")"
	}
	return "Rentang tanggal ini " + alasan + " — tidak perlu mengajukan cuti"
}

/* ── Input rules (lib/hris/holidays-input) ───────────────────────────── */

// HolidayTypes are the hris.public_holidays.type values.
var HolidayTypes = []string{"nasional", "cuti_bersama", "perusahaan"}

var yearRe = regexp.MustCompile(`^\d{4}$`)

// HolidayInput is the optional-field body of the holiday routes.
type HolidayInput struct {
	HolidayDate  *string
	Name         *string
	Type         *string
	DeductsLeave *bool
	IsDayOff     *bool
	Status       *string
	Note         *string
	NoteSent     bool
	SourceRef    *string
}

func isHolidayType(t string) bool {
	for _, v := range HolidayTypes {
		if v == t {
			return true
		}
	}
	return false
}

// ErrDayOffType is the 400 for is_day_off on a type other than cuti bersama.
const ErrDayOffType = "Hanya cuti bersama yang bisa ditetapkan libur"

func typeOrStatusError(in HolidayInput) string {
	if in.Type != nil && !isHolidayType(*in.Type) {
		return "Tipe libur tidak valid"
	}
	if in.IsDayOff != nil && *in.IsDayOff && in.Type != nil && *in.Type != "cuti_bersama" {
		return ErrDayOffType
	}
	if in.Status != nil && *in.Status != "draft" && *in.Status != "aktif" {
		return "Status tidak valid"
	}
	return ""
}

// jsDateValid is !isNaN(new Date(`${d}T00:00:00Z`)) for a YYYY-MM-DD string:
// V8 accepts any day 1..31 regardless of the month.
func jsDateValid(d string) bool {
	m, _ := strconv.Atoi(d[5:7])
	day, _ := strconv.Atoi(d[8:10])
	return m >= 1 && m <= 12 && day >= 1 && day <= 31
}

func blank(s *string) bool { return s == nil || JSTrim(*s) == "" }

// ValidateHolidayBody is validateHolidayBody (POST); "" means valid.
func ValidateHolidayBody(in HolidayInput) string {
	if in.HolidayDate == nil || !DateRe.MatchString(*in.HolidayDate) {
		return "Tanggal wajib diisi (YYYY-MM-DD)"
	}
	if !jsDateValid(*in.HolidayDate) {
		return "Tanggal tidak valid"
	}
	if blank(in.Name) {
		return "Nama libur wajib diisi"
	}
	return typeOrStatusErrorDefaulted(in)
}

// typeOrStatusErrorDefaulted is typeOrStatusError for a body that creates a
// row, where a missing type means "nasional".
func typeOrStatusErrorDefaulted(in HolidayInput) string {
	if in.Type == nil {
		nasional := "nasional"
		in.Type = &nasional
	}
	return typeOrStatusError(in)
}

// ValidateHolidayPatch checks only the fields that were sent.
func ValidateHolidayPatch(in HolidayInput) string {
	if in.HolidayDate != nil && !DateRe.MatchString(*in.HolidayDate) {
		return "Tanggal tidak valid (YYYY-MM-DD)"
	}
	if msg := typeOrStatusError(in); msg != "" {
		return msg
	}
	if in.Name != nil && JSTrim(*in.Name) == "" {
		return "Nama libur wajib diisi"
	}
	return ""
}

// ValidateImportItem checks one ticked import row.
func ValidateImportItem(in HolidayInput) string {
	if in.HolidayDate == nil || !DateRe.MatchString(*in.HolidayDate) {
		return "Tanggal tidak valid"
	}
	if blank(in.Name) {
		return "Nama libur wajib diisi"
	}
	return typeOrStatusErrorDefaulted(in)
}

// HolidayTypeOrDefault: unknown or missing types are "nasional".
func HolidayTypeOrDefault(t *string) string {
	if t != nil && isHolidayType(*t) {
		return *t
	}
	return "nasional"
}

// DefaultDeductsLeave: only cuti bersama consumes annual leave (SKB).
func DefaultDeductsLeave(t string) bool { return t == "cuti_bersama" }

// ResolveHolidayRange: start_date+end_date (both required when either is
// given) or one calendar year (default this year). ok=false is a bad range.
func ResolveHolidayRange(start, end *string, year string, now time.Time) (string, string, bool) {
	if (start != nil && *start != "") || (end != nil && *end != "") {
		if start == nil || !DateRe.MatchString(*start) || end == nil || !DateRe.MatchString(*end) {
			return "", "", false
		}
		return *start, *end, true
	}
	y := strconv.Itoa(now.Year())
	if yearRe.MatchString(year) {
		y = year
	}
	return y + "-01-01", y + "-12-31", true
}

// ResolveImportYear: a four digit year, else this year.
func ResolveImportYear(year string, now time.Time) int {
	if yearRe.MatchString(year) {
		n, _ := strconv.Atoi(year)
		return n
	}
	return now.Year()
}

/* ── ICS import (lib/hris/holiday-ics) ───────────────────────────────── */

// IcsEvent is one VEVENT.
type IcsEvent struct {
	UID  string
	Date string
	Name string
}

// HolidayCandidate is a preview row of the ICS import.
type HolidayCandidate struct {
	SourceRef    string  `json:"source_ref"`
	HolidayDate  string  `json:"holiday_date"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	DeductsLeave bool    `json:"deducts_leave"`
	Status       string  `json:"status"`
	Suggested    bool    `json:"suggested"`
	Reason       *string `json:"reason,omitempty"`
}

var notRedDays = []struct {
	pattern *regexp.Regexp
	reason  string
}{
	{regexp.MustCompile(`^\d+ ramadan`), "Awal Ramadan bukan hari libur nasional"},
	{regexp.MustCompile(`paskah`), "Hari Paskah jatuh Minggu dan bukan tanggal merah"},
	{regexp.MustCompile(`hari kedua muharram`), "Artefak batas hari hijriah, bukan libur resmi"},
	{regexp.MustCompile(`malam tahun baru`), "31 Desember bukan tanggal merah"},
	{regexp.MustCompile(`^hari (ayah|ibu|guru|pahlawan|kartini|sumpah pemuda)`), "Hari peringatan, bukan hari libur"},
}

var (
	tentative  = regexp.MustCompile(`(?i)\(belum pasti\)`)
	icsDate    = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})`)
	lineBreaks = regexp.MustCompile(`\r\n|\n|\r`)
	escNewline = regexp.MustCompile(`(?i)\\n`)
	escChar    = regexp.MustCompile(`\\([,;\\])`)
)

func unfold(text string) []string {
	var lines []string
	for _, raw := range lineBreaks.Split(text, -1) {
		if (strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")) && len(lines) > 0 {
			lines[len(lines)-1] += raw[1:]
		} else {
			lines = append(lines, raw)
		}
	}
	return lines
}

// ParseICS is parseIcs: VEVENTs with a date and a summary.
func ParseICS(text string) []IcsEvent {
	events := []IcsEvent{}
	var cur *IcsEvent
	for _, line := range unfold(text) {
		if strings.HasPrefix(line, "BEGIN:VEVENT") {
			cur = &IcsEvent{}
			continue
		}
		if strings.HasPrefix(line, "END:VEVENT") {
			if cur != nil && cur.Date != "" && cur.Name != "" {
				if cur.UID == "" {
					cur.UID = cur.Date + "_" + cur.Name
				}
				events = append(events, *cur)
			}
			cur = nil
			continue
		}
		if cur == nil {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		name := strings.ToUpper(strings.Split(line[:colon], ";")[0])
		value := line[colon+1:]
		switch name {
		case "UID":
			cur.UID = JSTrim(value)
		case "SUMMARY":
			cur.Name = JSTrim(escChar.ReplaceAllString(escNewline.ReplaceAllString(value, "\n"), "$1"))
		case "DTSTART":
			cur.Date = ""
			if m := icsDate.FindStringSubmatch(JSTrim(value)); m != nil {
				cur.Date = m[1] + "-" + m[2] + "-" + m[3]
			}
		}
	}
	return events
}

// ToHolidayCandidates curates the events of one year (see holiday-ics.ts).
func ToHolidayCandidates(events []IcsEvent, year int) []HolidayCandidate {
	prefix := strconv.Itoa(year) + "-"
	out := []HolidayCandidate{}
	for _, e := range events {
		if !strings.HasPrefix(e.Date, prefix) {
			continue
		}
		normalized := strings.ToLower(e.Name)
		cutiBersama := strings.HasPrefix(normalized, "cuti bersama")
		c := HolidayCandidate{
			SourceRef: e.UID, HolidayDate: e.Date, Name: e.Name,
			Type: "nasional", DeductsLeave: cutiBersama, Status: "aktif", Suggested: true,
		}
		if cutiBersama {
			c.Type = "cuti_bersama"
		}
		isTentative := tentative.MatchString(e.Name)
		if isTentative {
			c.Status = "draft"
		}
		for _, rule := range notRedDays {
			if rule.pattern.MatchString(normalized) {
				reason := rule.reason
				c.Suggested, c.Reason = false, &reason
				break
			}
		}
		if c.Reason == nil && isTentative {
			reason := "Sumbernya menandai tanggal ini belum pasti — masuk sebagai draft"
			c.Reason = &reason
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].HolidayDate == out[j].HolidayDate {
			return out[i].Name < out[j].Name
		}
		return out[i].HolidayDate < out[j].HolidayDate
	})
	return out
}
