package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Bulk upload Jadwal Kerja (Spec 2026-10-19 §2.5–2.6): read the template
// grid, check it, and turn valid cells into per-date assignments.

const (
	ScheduleOff       = "L"
	ScheduleNotPlaced = "-"
	MaxScheduleRows   = 300
	minScheduleDays   = 7
	maxScheduleDays   = 45
	minShiftGapMin    = 8 * 60
	maxStreak         = 6
	// minHoursBeforeDayOff: a day off is given only after this many net
	// working hours since the previous one (EPIC-079 R5).
	minHoursBeforeDayOff = 40
)

type UploadEmployee struct {
	ID, NIP, Name, Position, ScheduleType string
	Active                                bool
	Placed                                map[string]bool
}

type UploadShift struct {
	ID, Code, Name, Start, End string
	Overnight, Active          bool
	BreakMinutes               int
}

// shiftNetMinutes is the shift's working time: end minus start (across
// midnight for an overnight shift) minus the unpaid break.
func shiftNetMinutes(s UploadShift) int {
	start, end := clockMinutes(s.Start), clockMinutes(s.End)
	if s.Overnight || end <= start {
		end += 24 * 60
	}
	return end - start - s.BreakMinutes
}

// shiftSpanMinutes is end minus start, across midnight for an overnight shift.
func shiftSpanMinutes(s UploadShift) int {
	return shiftNetMinutes(s) + s.BreakMinutes
}

// ScheduleTwoShiftSep joins the two shift codes of one day (EPIC-080 R7).
const ScheduleTwoShiftSep = "+"

// ParseScheduleCode splits a schedule code into its first and second shift
// code ("PAGI4+SORE4" → PAGI4, SORE4; "SPG1" → SPG1, ""), trimming each part.
// A second shift needs two shift codes: libur ("L"/"OFF") and "-" cannot be
// combined, and a part cannot be empty.
func ParseScheduleCode(code string) (first, second string, err error) {
	code = strings.TrimSpace(code)
	parts := strings.Split(code, ScheduleTwoShiftSep)
	if len(parts) == 1 {
		return code, "", nil
	}
	if len(parts) > 2 {
		return "", "", fmt.Errorf("Kode '%s': maksimal 2 shift sehari (contoh PAGI4+SORE4)", code)
	}
	first, second = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	for _, p := range []string{first, second} {
		switch u := strings.ToUpper(p); {
		case u == "":
			return "", "", fmt.Errorf("Kode '%s' tidak lengkap: isi dua kode shift, contoh PAGI4+SORE4", code)
		case isOffCode(u) || u == ScheduleNotPlaced:
			return "", "", fmt.Errorf("Kode '%s': L atau - tidak bisa digabung dengan shift", code)
		}
	}
	return first, second, nil
}

// secondShiftOffset is the start of second in minutes from the start of
// first, within the 24 hours after it (a clock time before first's start is
// the next day).
func secondShiftOffset(first, second UploadShift) int {
	return ((clockMinutes(second.Start)-clockMinutes(first.Start))%(24*60) + 24*60) % (24 * 60)
}

// shiftIsOvernight is whether the shift ends the day after it starts.
func shiftIsOvernight(s UploadShift) bool {
	return s.Overnight || clockMinutes(s.End) <= clockMinutes(s.Start)
}

// TwoShiftIssue checks a two-shift day (EPIC-080 R6), the same rule as the
// grid (frontend canFollow): the second shift must start at or after the
// first ends and end within 24 hours of the first's start, and it may not
// start, by clock, before the first shift starts unless the first is
// overnight (so SORE4 17–21 + PAGI4 06–10 is rejected, not read as the next
// day). The second shift itself may run past midnight.
func TwoShiftIssue(first, second UploadShift) string {
	off := secondShiftOffset(first, second)
	startsBefore := clockMinutes(second.Start) < clockMinutes(first.Start)
	if (!startsBefore || shiftIsOvernight(first)) &&
		off >= shiftSpanMinutes(first) && off+shiftSpanMinutes(second) <= 24*60 {
		return ""
	}
	return fmt.Sprintf("Shift kedua harus mulai setelah shift pertama selesai (%s selesai %s, %s mulai %s)",
		first.Code, dotClock(first.End), second.Code, dotClock(second.Start))
}

// dotClock is "16:00:00" as "16.00".
func dotClock(hms string) string {
	if len(hms) >= 5 {
		return strings.Replace(hms[:5], ":", ".", 1)
	}
	return hms
}

type ScheduleUploadContext struct {
	BranchID, BranchName string
	Dates                []string
	Today                string
	Employees            map[string]UploadEmployee
	Shifts               map[string]UploadShift
	ClosedDates          map[string]bool
	Holidays             map[string]string
	OnLeave              map[string]map[string]bool
	ClockedIn            map[string]map[string]bool
	// Per-cell save (EPIC-070): the employees keyed by ID, and the code that
	// applies today per employee and date around the edited cells (shift
	// code, "L", "-" or "").
	EmployeesByID map[string]UploadEmployee
	Current       map[string]map[string]string
	// RosterOnly (EPIC-071 store delegates): Employees holds only the people
	// placed at the branch in the window. Anyone else, and any inactive
	// employee, is reported as not found at the branch, revealing nothing
	// about them (no name, schedule type or active state).
	RosterOnly bool
}

type ScheduleFile struct {
	InfoBranchID, InfoFrom, InfoTo string
	HeaderLine                     int
	DateHeaders                    []string
	Rows                           []ScheduleFileRow
}

type ScheduleFileRow struct {
	Line  int
	NIP   string
	Cells []string
}

type UploadIssue struct {
	Line    int    `json:"line"`
	Column  string `json:"column"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

type ScheduleAssignment struct {
	EmployeeID, Date string
	ShiftID          *string
	Shift2ID         *string // second shift of the day (EPIC-080), nil when none
	Clear            bool    // remove the per-date row only (back to the weekly pattern)
}

type ScheduleUploadResult struct {
	Assignments []ScheduleAssignment
	Errors      []UploadIssue
	Warnings    []UploadIssue
	EmployeeIDs []string
}

var dayShort = [...]string{"Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"}

func ScheduleDates(from, to string) ([]string, string) {
	a, err1 := time.Parse(DateLayout, from)
	b, err2 := time.Parse(DateLayout, to)
	if err1 != nil || err2 != nil || b.Before(a) {
		return nil, "Tanggal mulai dan selesai wajib diisi dengan benar"
	}
	n := int(b.Sub(a).Hours()/24) + 1
	if n < minScheduleDays || n > maxScheduleDays {
		return nil, "Rentang tanggal 7–45 hari"
	}
	out := make([]string, n)
	for i := range out {
		out[i] = a.AddDate(0, 0, i).Format(DateLayout)
	}
	return out, ""
}

func ScheduleDateHeader(date string) string {
	t, err := time.Parse(DateLayout, date)
	if err != nil {
		return date
	}
	return dayShort[t.Weekday()] + " " + t.Format("02/01")
}

// ReadScheduleFile finds the header row (first cell "NIP"), the date
// columns up to "Hari Masuk", and the non-blank rows below it.
func ReadScheduleFile(matrix, info [][]string) (ScheduleFile, string) {
	var f ScheduleFile
	for _, kv := range info {
		if len(kv) < 2 {
			continue
		}
		switch NormalizeHeader(kv[0]) {
		case "cabang id":
			f.InfoBranchID = strings.TrimSpace(kv[1])
		case "dari":
			f.InfoFrom, _ = ParseDateCell(kv[1])
		case "sampai":
			f.InfoTo, _ = ParseDateCell(kv[1])
		}
	}
	header := -1
	for i, r := range matrix {
		if len(r) > 0 && NormalizeHeader(r[0]) == "nip" {
			header = i
			break
		}
	}
	if header < 0 {
		return f, "Judul kolom NIP tidak ditemukan — pakai template dari halaman Jadwal Kerja"
	}
	f.HeaderLine = header + 1
	cols := matrix[header]
	if len(cols) < 4 {
		return f, "Kolom tanggal tidak ditemukan — pakai template dari halaman Jadwal Kerja"
	}
	for _, h := range cols[3:] {
		n := NormalizeHeader(h)
		if n == "" || n == "hari masuk" || n == "hari libur" {
			break
		}
		f.DateHeaders = append(f.DateHeaders, strings.TrimSpace(h))
	}
	for i := header + 1; i < len(matrix); i++ {
		r := matrix[i]
		nip := ""
		if len(r) > 0 {
			nip = r[0]
		}
		cells := make([]string, len(f.DateHeaders))
		blank := strings.TrimSpace(nip) == ""
		for j := range cells {
			if 3+j < len(r) {
				cells[j] = strings.TrimSpace(r[3+j])
				blank = blank && cells[j] == ""
			}
		}
		if blank {
			continue
		}
		f.Rows = append(f.Rows, ScheduleFileRow{Line: i + 1, NIP: nip, Cells: cells})
	}
	return f, ""
}

func clockMinutes(hms string) int {
	var h, m int
	fmt.Sscanf(hms, "%d:%d", &h, &m)
	return h*60 + m
}

func ValidateScheduleUpload(c ScheduleUploadContext, f ScheduleFile) ScheduleUploadResult {
	var res ScheduleUploadResult
	fail := func(line int, col, name, msg string) {
		res.Errors = append(res.Errors, UploadIssue{line, col, name, msg})
	}
	warn := func(line int, col, name, msg string) {
		res.Warnings = append(res.Warnings, UploadIssue{line, col, name, msg})
	}
	from, to := c.Dates[0], c.Dates[len(c.Dates)-1]
	switch {
	case f.InfoBranchID == "" && f.InfoFrom == "" && f.InfoTo == "":
		fail(0, "", "", "Sheet Info tidak ditemukan — pakai template xlsx yang diunduh dari halaman Jadwal Kerja")
		return res
	case f.InfoBranchID != c.BranchID || f.InfoFrom != from || f.InfoTo != to:
		fail(0, "", "", fmt.Sprintf("File ini bukan template %s %s–%s. Unduh template baru dari halaman Jadwal Kerja.",
			c.BranchName, FormatDateID(from), FormatDateID(to)))
		return res
	}
	if len(f.DateHeaders) != len(c.Dates) {
		fail(f.HeaderLine, "", "", fmt.Sprintf("Kolom tanggal tidak sesuai template (%d kolom, seharusnya %d)", len(f.DateHeaders), len(c.Dates)))
		return res
	}
	for i, d := range c.Dates {
		if want := ScheduleDateHeader(d); f.DateHeaders[i] != want {
			fail(f.HeaderLine, f.DateHeaders[i], "", fmt.Sprintf("Kolom tanggal tidak sesuai template ('%s', seharusnya '%s')", f.DateHeaders[i], want))
			return res
		}
	}
	for _, d := range c.Dates {
		if msg := dateIssue(c, d); msg != "" {
			fail(f.HeaderLine, ScheduleDateHeader(d), "", msg)
		}
	}
	if len(f.Rows) > MaxScheduleRows {
		fail(0, "", "", fmt.Sprintf("Maksimal %d karyawan per file", MaxScheduleRows))
		return res
	}

	type parsed struct {
		emp    UploadEmployee
		line   int
		cells  []*UploadShift // nil = libur / tidak dijadwalkan
		cells2 []*UploadShift // shift kedua, nil bila tidak ada
		set    []bool         // sel menghasilkan penugasan
	}
	seen := map[string]int{}
	var valid []parsed
	working := map[string]int{}
	for _, r := range f.Rows {
		nip, msg := CleanNIP(r.NIP)
		if msg != "" {
			fail(r.Line, "NIP", "", msg)
			continue
		}
		if first, dup := seen[nip]; dup {
			fail(r.Line, "NIP", "", fmt.Sprintf("NIP %s ganda (baris %d & %d)", nip, first, r.Line))
			continue
		}
		seen[nip] = r.Line
		e, ok := c.Employees[nip]
		if !ok || !e.Active {
			if c.RosterOnly {
				fail(r.Line, "NIP", "", "NIP "+nip+" tidak ditemukan di "+c.BranchName)
			} else {
				fail(r.Line, "NIP", "", "NIP "+nip+" tidak dikenal atau sudah nonaktif")
			}
			continue
		}
		if msg := nonShiftIssue(e); msg != "" {
			fail(r.Line, "NIP", e.Name, msg)
			continue
		}
		p := parsed{emp: e, line: r.Line, cells: make([]*UploadShift, len(c.Dates)),
			cells2: make([]*UploadShift, len(c.Dates)), set: make([]bool, len(c.Dates))}
		rowOK := true
		for i, d := range c.Dates {
			col := ScheduleDateHeader(d)
			raw := strings.TrimSpace(r.Cells[i])
			code := strings.ToUpper(raw)
			if !e.Placed[d] {
				if code == "" || code == ScheduleNotPlaced {
					continue
				}
				fail(r.Line, col, e.Name, notPlacedIssue(c, e, d))
				rowOK = false
				continue
			}
			switch {
			case code == "":
				fail(r.Line, col, e.Name, "Isi kode shift atau L")
				rowOK = false
			case isOffCode(code):
				p.set[i] = true
			default:
				s, s2, msg := shiftsByCode(c, raw)
				if msg != "" {
					fail(r.Line, col, e.Name, msg)
					rowOK = false
					continue
				}
				p.cells[i], p.cells2[i], p.set[i] = s, s2, true
				working[d]++
			}
		}
		if rowOK {
			valid = append(valid, p)
		}
	}
	for _, p := range valid {
		res.EmployeeIDs = append(res.EmployeeIDs, p.emp.ID)
		days := make([]plannedDay, len(c.Dates))
		for i, d := range c.Dates {
			days[i] = plannedDay{Date: d, Shift: p.cells[i], Shift2: p.cells2[i], Edited: p.set[i], Off: p.set[i] && p.cells[i] == nil}
			if p.set[i] {
				res.Assignments = append(res.Assignments, ScheduleAssignment{EmployeeID: p.emp.ID, Date: d,
					ShiftID: shiftIDOf(p.cells[i]), Shift2ID: shiftIDOf(p.cells2[i])})
			}
		}
		sequenceWarnings(c, p.emp, p.line, days, warn)
	}
	for _, d := range c.Dates {
		if len(valid) > 0 && working[d] == 0 {
			warn(f.HeaderLine, ScheduleDateHeader(d), "", fmt.Sprintf("Tidak ada yang masuk di %s %s", c.BranchName, FormatDateID(d)))
		}
		if d == c.Today {
			n := 0
			for _, p := range valid {
				if c.ClockedIn[p.emp.ID][d] {
					n++
				}
			}
			if n > 0 {
				warn(f.HeaderLine, ScheduleDateHeader(d), "", clockedInIssue(n, d))
			}
		}
	}
	sort.SliceStable(res.Errors, func(i, j int) bool { return res.Errors[i].Line < res.Errors[j].Line })
	sort.SliceStable(res.Warnings, func(i, j int) bool { return res.Warnings[i].Line < res.Warnings[j].Line })
	return res
}

/* ── Rules shared by the file upload and the per-cell save ───────────── */

func isOffCode(code string) bool { return code == ScheduleOff || code == "OFF" }

func shiftIDOf(s *UploadShift) *string {
	if s == nil {
		return nil
	}
	id := s.ID
	return &id
}

// dateIssue: a past date or a date in a closed attendance period.
func dateIssue(c ScheduleUploadContext, d string) string {
	switch {
	case d < c.Today:
		return "Tanggal " + FormatDateID(d) + " sudah lewat"
	case c.ClosedDates[d]:
		return "Periode absensi " + FormatDateID(d) + " sudah closing"
	}
	return ""
}

func nonShiftIssue(e UploadEmployee) string {
	if e.ScheduleType != "shift" {
		return e.Name + " Non-Shift, pakai jadwal tetap"
	}
	return ""
}

func notPlacedIssue(c ScheduleUploadContext, e UploadEmployee, d string) string {
	return fmt.Sprintf("%s tidak ditempatkan di %s pada %s", e.Name, c.BranchName, FormatDateID(d))
}

// shiftByCode resolves a shift code (not empty, not libur); msg explains a
// typo, an unknown code or an inactive shift.
func shiftByCode(c ScheduleUploadContext, raw string) (*UploadShift, string) {
	code := strings.ToUpper(raw)
	s, known := c.Shifts[code]
	if !known {
		if alt, ok := c.Shifts[strings.ReplaceAll(code, " ", "")]; ok {
			return nil, fmt.Sprintf("Kode '%s' tidak ada; maksud Anda %s?", raw, alt.Code)
		}
		return nil, fmt.Sprintf("Kode '%s' tidak ada di sheet Kode Shift", raw)
	}
	if !s.Active {
		return nil, "Shift " + s.Code + " sudah nonaktif"
	}
	return &s, ""
}

// shiftsByCode resolves a working-day code: one shift code or KODE1+KODE2
// (EPIC-080), the second starting after the first ends. second is nil for a
// one-shift day.
func shiftsByCode(c ScheduleUploadContext, raw string) (first, second *UploadShift, msg string) {
	a, b, err := ParseScheduleCode(raw)
	if err != nil {
		return nil, nil, err.Error()
	}
	if first, msg = shiftByCode(c, a); msg != "" || b == "" {
		return first, nil, msg
	}
	if second, msg = shiftByCode(c, b); msg != "" {
		return nil, nil, msg
	}
	if msg = TwoShiftIssue(*first, *second); msg != "" {
		return nil, nil, msg
	}
	return first, second, ""
}

// currentShifts reads a resolved current code (a shift code or KODE1+KODE2)
// as shifts; nil when it is not a known working-day code.
func currentShifts(c ScheduleUploadContext, code string) (first, second *UploadShift) {
	a, b, err := ParseScheduleCode(code)
	if err != nil {
		return nil, nil
	}
	s1, ok := c.Shifts[strings.ToUpper(a)]
	if !ok {
		return nil, nil
	}
	if b == "" {
		return &s1, nil
	}
	s2, ok := c.Shifts[strings.ToUpper(b)]
	if !ok {
		return &s1, nil
	}
	return &s1, &s2
}

func clockedInIssue(n int, d string) string {
	return fmt.Sprintf("%d karyawan sudah clock-in %s; telatnya tidak dihitung ulang", n, FormatDateID(d))
}

// plannedDay is one date of an employee's resulting schedule; Edited marks
// the dates this upload/save writes.
type plannedDay struct {
	Date   string
	Shift  *UploadShift // nil = libur / tidak dijadwalkan
	Shift2 *UploadShift // second shift of a two-shift day (EPIC-080), nil when none
	Edited bool
	// Off marks an explicit libur ("L"). A nil Shift without Off is unknown
	// (empty, not placed, cleared): it is not a day off for the 40-hour rule.
	Off bool
}

// code is the day's schedule code: the shift code, or KODE1+KODE2.
func (d plannedDay) code() string {
	if d.Shift2 == nil {
		return d.Shift.Code
	}
	return d.Shift.Code + ScheduleTwoShiftSep + d.Shift2.Code
}

// netMinutes is the working time of both shifts (EPIC-080 R9).
func (d plannedDay) netMinutes() int {
	n := shiftNetMinutes(*d.Shift)
	if d.Shift2 != nil {
		n += shiftNetMinutes(*d.Shift2)
	}
	return n
}

// endMinutes is when the day's last shift ends, in minutes from the start of
// its date (past 24*60 when it ends the next day).
func (d plannedDay) endMinutes() int {
	start := clockMinutes(d.Shift.Start)
	if d.Shift2 == nil {
		return start + shiftSpanMinutes(*d.Shift)
	}
	return start + secondShiftOffset(*d.Shift, *d.Shift2) + shiftSpanMinutes(*d.Shift2)
}

// sequenceWarnings checks consecutive days: leave on an edited working day,
// a gap under minShiftGapMin next to an edited day, and one warning per
// streak longer than maxStreak that contains an edited day, and a day off
// that follows another day off inside days after fewer than
// minHoursBeforeDayOff net working hours when the streak or that day off is
// edited.
func sequenceWarnings(c ScheduleUploadContext, e UploadEmployee, line int, days []plannedDay, warn func(line int, col, name, msg string)) {
	run, edited := 0, false // working days in the current streak
	streakMin := 0          // net working minutes of the streak
	prevOff := ""           // previous day off inside days
	for i, d := range days {
		s := d.Shift
		if s == nil {
			if !d.Off {
				prevOff = ""
				run, edited, streakMin = 0, false, 0
				continue
			}
			if prevOff != "" && run > 0 && streakMin < minHoursBeforeDayOff*60 && (edited || d.Edited) {
				warn(line, ScheduleDateHeader(d.Date), e.Name, fmt.Sprintf("%s libur %s sebelum %d jam kerja (baru %d jam sejak libur %s)",
					e.Name, FormatDateID(d.Date), minHoursBeforeDayOff, streakMin/60, FormatDateID(prevOff)))
			}
			prevOff = d.Date
			run, edited, streakMin = 0, false, 0
			continue
		}
		run++
		streakMin += d.netMinutes()
		edited = edited || d.Edited
		if d.Edited && c.OnLeave[e.ID][d.Date] {
			warn(line, ScheduleDateHeader(d.Date), e.Name, fmt.Sprintf("%s cuti %s tapi diberi %s", e.Name, FormatDateID(d.Date), d.code()))
		}
		last := i+1 == len(days) || days[i+1].Shift == nil
		if !last && (d.Edited || days[i+1].Edited) {
			next := days[i+1]
			if gap := 24*60 + clockMinutes(next.Shift.Start) - d.endMinutes(); gap < minShiftGapMin {
				warn(line, ScheduleDateHeader(next.Date), e.Name, fmt.Sprintf("%s: jeda %d jam antara %s (%s) dan %s (%s)",
					e.Name, gap/60, FormatDateID(d.Date), d.code(), FormatDateID(next.Date), next.code()))
			}
		}
		if last && run > maxStreak && edited {
			warn(line, "", e.Name, fmt.Sprintf("%s masuk %d hari berturut-turut (%s–%s)", e.Name, run,
				FormatDateID(days[i-run+1].Date), FormatDateID(d.Date)))
		}
	}
}

/* ── Per-cell save (EPIC-070) ────────────────────────────────────────── */

// ScheduleCellInput is one grid cell HR edited. Code is a shift code, "L"
// or "OFF" (libur), or "" (remove the per-date row: back to the weekly
// pattern or nothing).
type ScheduleCellInput struct {
	EmployeeID, Date, Code string
}

// ScheduleCellWindow is the sorted edited dates widened by maxStreak days
// on each side, day by day: the dates whose current schedule the streak
// and gap warnings read (c.Current must cover them).
func ScheduleCellWindow(dates []string) []string {
	if len(dates) == 0 {
		return nil
	}
	lo, hi := dates[0], dates[0]
	for _, d := range dates {
		lo, hi = min(lo, d), max(hi, d)
	}
	var out []string
	for d, end := AddDaysISO(lo, -maxStreak), AddDaysISO(hi, maxStreak); d <= end; d = AddDaysISO(d, 1) {
		out = append(out, d)
	}
	return out
}

// ValidateScheduleCells applies the upload rules to edited grid cells.
// Issues have Line 0, the date header as Column and the employee's name.
// Leave, gap and streak warnings read the resulting schedule: the
// edited cells over c.Current for the neighbouring dates.
func ValidateScheduleCells(c ScheduleUploadContext, cells []ScheduleCellInput) ScheduleUploadResult {
	var res ScheduleUploadResult
	fail := func(col, name, msg string) {
		res.Errors = append(res.Errors, UploadIssue{0, col, name, msg})
	}
	warn := func(line int, col, name, msg string) {
		res.Warnings = append(res.Warnings, UploadIssue{line, col, name, msg})
	}
	type edit struct {
		shift, shift2 *UploadShift
		clear         bool
	}
	edits := map[string]map[string]edit{}
	seen := map[[2]string]bool{}
	badEmp, badDate := map[string]bool{}, map[string]bool{}
	var order []string
	for _, in := range cells {
		col := ScheduleDateHeader(in.Date)
		e, ok := c.EmployeesByID[in.EmployeeID]
		msg, name := "", e.Name
		switch {
		case c.RosterOnly && (!ok || !e.Active):
			msg, name = "Karyawan tidak ditemukan di "+c.BranchName, ""
		case !ok:
			msg = "Karyawan tidak dikenal atau sudah nonaktif"
		case !e.Active:
			msg = "NIP " + e.NIP + " tidak dikenal atau sudah nonaktif"
		default:
			msg = nonShiftIssue(e)
		}
		if msg != "" {
			if !badEmp[in.EmployeeID] {
				badEmp[in.EmployeeID] = true
				fail(col, name, msg)
			}
			continue
		}
		key := [2]string{e.ID, in.Date}
		if seen[key] {
			fail(col, e.Name, fmt.Sprintf("%s %s dikirim lebih dari sekali", e.Name, FormatDateID(in.Date)))
			continue
		}
		seen[key] = true
		if msg := dateIssue(c, in.Date); msg != "" {
			if !badDate[in.Date] {
				badDate[in.Date] = true
				fail(col, "", msg)
			}
			continue
		}
		if !e.Placed[in.Date] {
			fail(col, e.Name, notPlacedIssue(c, e, in.Date))
			continue
		}
		raw := strings.TrimSpace(in.Code)
		var ed edit
		switch code := strings.ToUpper(raw); {
		case code == "":
			ed.clear = true
		case isOffCode(code):
		default:
			s, s2, msg := shiftsByCode(c, raw)
			if msg != "" {
				fail(col, e.Name, msg)
				continue
			}
			ed.shift, ed.shift2 = s, s2
		}
		if edits[e.ID] == nil {
			edits[e.ID] = map[string]edit{}
			order = append(order, e.ID)
		}
		edits[e.ID][in.Date] = ed
	}

	clocked := 0
	for _, id := range order {
		e, mine := c.EmployeesByID[id], edits[id]
		res.EmployeeIDs = append(res.EmployeeIDs, id)
		dates := make([]string, 0, len(mine))
		for d := range mine {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		for _, d := range dates {
			ed := mine[d]
			res.Assignments = append(res.Assignments, ScheduleAssignment{EmployeeID: id, Date: d,
				ShiftID: shiftIDOf(ed.shift), Shift2ID: shiftIDOf(ed.shift2), Clear: ed.clear})
			if d == c.Today && c.ClockedIn[id][d] {
				clocked++
			}
		}
		// The resulting schedule around the edits. A cleared cell falls back
		// to the weekly pattern, which Current does not carry: it counts as
		// not working here.
		var days []plannedDay
		for _, d := range ScheduleCellWindow(dates) {
			if ed, ok := mine[d]; ok {
				days = append(days, plannedDay{Date: d, Shift: ed.shift, Shift2: ed.shift2, Edited: true, Off: ed.shift == nil && !ed.clear})
				continue
			}
			cur := strings.ToUpper(c.Current[id][d])
			s, s2 := currentShifts(c, cur)
			days = append(days, plannedDay{Date: d, Shift: s, Shift2: s2, Off: s == nil && isOffCode(cur)})
		}
		sequenceWarnings(c, e, 0, days, warn)
	}
	if clocked > 0 {
		warn(0, ScheduleDateHeader(c.Today), "", clockedInIssue(clocked, c.Today))
	}
	return res
}
