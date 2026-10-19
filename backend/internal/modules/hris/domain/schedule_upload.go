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
		if d < c.Today {
			fail(f.HeaderLine, ScheduleDateHeader(d), "", "Tanggal "+FormatDateID(d)+" sudah lewat")
		} else if c.ClosedDates[d] {
			fail(f.HeaderLine, ScheduleDateHeader(d), "", "Periode absensi "+FormatDateID(d)+" sudah closing")
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
		if e.ScheduleType != "shift" {
			fail(r.Line, "NIP", e.Name, e.Name+" Non-Shift, pakai jadwal tetap")
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
				fail(r.Line, col, e.Name, fmt.Sprintf("%s tidak ditempatkan di %s pada %s", e.Name, c.BranchName, FormatDateID(d)))
				rowOK = false
				continue
			}
			switch {
			case code == "":
				fail(r.Line, col, e.Name, "Isi kode shift atau L")
				rowOK = false
			case code == ScheduleOff || code == "OFF":
				p.set[i] = true
			default:
				s, known := c.Shifts[code]
				if !known {
					if alt, ok := c.Shifts[strings.ReplaceAll(code, " ", "")]; ok {
						fail(r.Line, col, e.Name, fmt.Sprintf("Kode '%s' tidak ada; maksud Anda %s?", raw, alt.Code))
					} else {
						fail(r.Line, col, e.Name, fmt.Sprintf("Kode '%s' tidak ada di sheet Kode Shift", raw))
					}
					rowOK = false
					continue
				}
				if !s.Active {
					fail(r.Line, col, e.Name, "Shift "+s.Code+" sudah nonaktif")
					rowOK = false
					continue
				}
				sc := s
				p.cells[i], p.set[i] = &sc, true
				working[d]++
			}
		}
		if rowOK {
			valid = append(valid, p)
		}
	}
	for _, p := range valid {
		res.EmployeeIDs = append(res.EmployeeIDs, p.emp.ID)
		run := 0 // working days in the current streak
		for i, d := range c.Dates {
			if !p.set[i] {
				run = 0
				continue
			}
			s := p.cells[i]
			var shiftID *string
			if s != nil {
				id := s.ID
				shiftID = &id
				run++
				if c.OnLeave[p.emp.ID][d] {
					warn(p.line, ScheduleDateHeader(d), p.emp.Name, fmt.Sprintf("%s cuti %s tapi diberi %s", p.emp.Name, FormatDateID(d), s.Code))
				}
				if i+1 < len(c.Dates) && p.cells[i+1] != nil {
					next := p.cells[i+1]
					end := clockMinutes(s.End)
					if s.Overnight || end <= clockMinutes(s.Start) {
						end += 24 * 60
					}
					if gap := 24*60 + clockMinutes(next.Start) - end; gap < minShiftGapMin {
						warn(p.line, ScheduleDateHeader(c.Dates[i+1]), p.emp.Name, fmt.Sprintf("%s: jeda %d jam antara %s (%s) dan %s (%s)",
							p.emp.Name, gap/60, FormatDateID(d), s.Code, FormatDateID(c.Dates[i+1]), next.Code))
					}
				}
			} else {
				run = 0
			}
			// One warning per streak longer than 6, naming the whole streak.
			lastOfRun := s != nil && (i+1 == len(c.Dates) || p.cells[i+1] == nil)
			if lastOfRun && run > maxStreak {
				warn(p.line, "", p.emp.Name, fmt.Sprintf("%s masuk %d hari berturut-turut (%s–%s)", p.emp.Name, run,
					FormatDateID(c.Dates[i-run+1]), FormatDateID(d)))
			}
			res.Assignments = append(res.Assignments, ScheduleAssignment{EmployeeID: p.emp.ID, Date: d, ShiftID: shiftID})
		}
	}
	for _, d := range c.Dates {
		if name, ok := c.Holidays[d]; ok && working[d] > 0 {
			warn(f.HeaderLine, ScheduleDateHeader(d), "", fmt.Sprintf("%s libur nasional (%s): %d karyawan diberi shift", FormatDateID(d), name, working[d]))
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
				warn(f.HeaderLine, ScheduleDateHeader(d), "", fmt.Sprintf("%d karyawan sudah clock-in %s; telatnya tidak dihitung ulang", n, FormatDateID(d)))
			}
		}
	}
	sort.SliceStable(res.Errors, func(i, j int) bool { return res.Errors[i].Line < res.Errors[j].Line })
	sort.SliceStable(res.Warnings, func(i, j int) bool { return res.Warnings[i].Line < res.Warnings[j].Line })
	return res
}
