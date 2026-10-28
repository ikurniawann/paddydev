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
)

type UploadEmployee struct {
	ID, NIP, Name, Position, ScheduleType string
	Active                                bool
	Placed                                map[string]bool
}

type UploadShift struct {
	ID, Code, Name, Start, End string
	Overnight, Active          bool
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
	Clear            bool // remove the per-date row only (back to the weekly pattern)
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
		emp   UploadEmployee
		line  int
		cells []*UploadShift // nil = libur / tidak dijadwalkan
		set   []bool         // sel menghasilkan penugasan
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
			fail(r.Line, "NIP", "", "NIP "+nip+" tidak dikenal atau sudah nonaktif")
			continue
		}
		if msg := nonShiftIssue(e); msg != "" {
			fail(r.Line, "NIP", e.Name, msg)
			continue
		}
		p := parsed{emp: e, line: r.Line, cells: make([]*UploadShift, len(c.Dates)), set: make([]bool, len(c.Dates))}
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
				s, msg := shiftByCode(c, raw)
				if msg != "" {
					fail(r.Line, col, e.Name, msg)
					rowOK = false
					continue
				}
				p.cells[i], p.set[i] = s, true
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
			days[i] = plannedDay{Date: d, Shift: p.cells[i], Edited: p.set[i]}
			if p.set[i] {
				res.Assignments = append(res.Assignments, ScheduleAssignment{EmployeeID: p.emp.ID, Date: d, ShiftID: shiftIDOf(p.cells[i])})
			}
		}
		sequenceWarnings(c, p.emp, p.line, days, warn)
	}
	for _, d := range c.Dates {
		if msg := holidayIssue(c, d, working[d]); msg != "" {
			warn(f.HeaderLine, ScheduleDateHeader(d), "", msg)
		}
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

func holidayIssue(c ScheduleUploadContext, d string, working int) string {
	if name, ok := c.Holidays[d]; ok && working > 0 {
		return fmt.Sprintf("%s libur nasional (%s): %d karyawan diberi shift", FormatDateID(d), name, working)
	}
	return ""
}

func clockedInIssue(n int, d string) string {
	return fmt.Sprintf("%d karyawan sudah clock-in %s; telatnya tidak dihitung ulang", n, FormatDateID(d))
}

// plannedDay is one date of an employee's resulting schedule; Edited marks
// the dates this upload/save writes.
type plannedDay struct {
	Date   string
	Shift  *UploadShift // nil = libur / tidak dijadwalkan
	Edited bool
}

// sequenceWarnings checks consecutive days: leave on an edited working day,
// a gap under minShiftGapMin next to an edited day, and one warning per
// streak longer than maxStreak that contains an edited day.
func sequenceWarnings(c ScheduleUploadContext, e UploadEmployee, line int, days []plannedDay, warn func(line int, col, name, msg string)) {
	run, edited := 0, false // working days in the current streak
	for i, d := range days {
		s := d.Shift
		if s == nil {
			run, edited = 0, false
			continue
		}
		run++
		edited = edited || d.Edited
		if d.Edited && c.OnLeave[e.ID][d.Date] {
			warn(line, ScheduleDateHeader(d.Date), e.Name, fmt.Sprintf("%s cuti %s tapi diberi %s", e.Name, FormatDateID(d.Date), s.Code))
		}
		last := i+1 == len(days) || days[i+1].Shift == nil
		if !last && (d.Edited || days[i+1].Edited) {
			next := days[i+1]
			end := clockMinutes(s.End)
			if s.Overnight || end <= clockMinutes(s.Start) {
				end += 24 * 60
			}
			if gap := 24*60 + clockMinutes(next.Shift.Start) - end; gap < minShiftGapMin {
				warn(line, ScheduleDateHeader(next.Date), e.Name, fmt.Sprintf("%s: jeda %d jam antara %s (%s) dan %s (%s)",
					e.Name, gap/60, FormatDateID(d.Date), s.Code, FormatDateID(next.Date), next.Shift.Code))
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
// Leave, gap, streak and holiday warnings read the resulting schedule: the
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
		shift *UploadShift
		clear bool
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
			s, msg := shiftByCode(c, raw)
			if msg != "" {
				fail(col, e.Name, msg)
				continue
			}
			ed.shift = s
		}
		if edits[e.ID] == nil {
			edits[e.ID] = map[string]edit{}
			order = append(order, e.ID)
		}
		edits[e.ID][in.Date] = ed
	}

	working := map[string]int{} // edited cells given a shift, per date
	var editedDates []string
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
			res.Assignments = append(res.Assignments, ScheduleAssignment{EmployeeID: id, Date: d, ShiftID: shiftIDOf(ed.shift), Clear: ed.clear})
			if ed.shift != nil {
				if working[d] == 0 {
					editedDates = append(editedDates, d)
				}
				working[d]++
			}
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
				days = append(days, plannedDay{Date: d, Shift: ed.shift, Edited: true})
				continue
			}
			var s *UploadShift
			if sh, ok := c.Shifts[strings.ToUpper(c.Current[id][d])]; ok {
				s = &sh
			}
			days = append(days, plannedDay{Date: d, Shift: s})
		}
		sequenceWarnings(c, e, 0, days, warn)
	}
	sort.Strings(editedDates)
	for _, d := range editedDates {
		if msg := holidayIssue(c, d, working[d]); msg != "" {
			warn(0, ScheduleDateHeader(d), "", msg)
		}
	}
	if clocked > 0 {
		warn(0, ScheduleDateHeader(c.Today), "", clockedInIssue(clocked, c.Today))
	}
	return res
}
