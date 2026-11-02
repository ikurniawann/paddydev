package domain

import (
	"strings"
	"testing"
)

func uctx() ScheduleUploadContext {
	dates, _ := ScheduleDates("2026-11-02", "2026-11-08") // Sen–Min
	all := map[string]bool{}
	for _, d := range dates {
		all[d] = true
	}
	return ScheduleUploadContext{
		BranchID: "b1", BranchName: "PPJA", Dates: dates, Today: "2026-11-02",
		Employees: map[string]UploadEmployee{
			"111": {ID: "e1", NIP: "111", Name: "Ana", ScheduleType: "shift", Active: true, Placed: all},
			"222": {ID: "e2", NIP: "222", Name: "Budi", ScheduleType: "shift", Active: true, Placed: all},
			"333": {ID: "e3", NIP: "333", Name: "Cici", ScheduleType: "non_shift", Active: true, Placed: all},
			"444": {ID: "e4", NIP: "444", Name: "Dodi", ScheduleType: "shift", Active: false, Placed: all},
			"555": {ID: "e5", NIP: "555", Name: "Eka", ScheduleType: "shift", Active: true, Placed: map[string]bool{"2026-11-02": true}},
		},
		Shifts: map[string]UploadShift{
			"SPG1":  {ID: "s1", Code: "SPG1", Start: "08:00:00", End: "16:00:00", Active: true, BreakMinutes: 60},
			"MALAM": {ID: "s2", Code: "MALAM", Start: "22:00:00", End: "06:00:00", Overnight: true, Active: true, BreakMinutes: 60},
			"OLD":   {ID: "s3", Code: "OLD", Start: "08:00:00", End: "16:00:00", Active: false},
		},
		ClosedDates: map[string]bool{}, Holidays: map[string]string{},
		OnLeave: map[string]map[string]bool{}, ClockedIn: map[string]map[string]bool{},
	}
}

func sfile(c ScheduleUploadContext, rows ...ScheduleFileRow) ScheduleFile {
	h := make([]string, len(c.Dates))
	for i, d := range c.Dates {
		h[i] = ScheduleDateHeader(d)
	}
	return ScheduleFile{InfoBranchID: c.BranchID, InfoFrom: c.Dates[0], InfoTo: c.Dates[len(c.Dates)-1], HeaderLine: 3, DateHeaders: h, Rows: rows}
}

func row(line int, nip string, cells ...string) ScheduleFileRow {
	return ScheduleFileRow{Line: line, NIP: nip, Cells: cells}
}

func week(v string) []string { return []string{v, v, v, v, v, "L", "L"} }

func hasIssue(list []UploadIssue, substr string) bool {
	for _, i := range list {
		if strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}

func TestScheduleDatesAndHeader(t *testing.T) {
	if _, msg := ScheduleDates("2026-11-02", "2026-11-07"); msg != "Rentang tanggal 7–45 hari" {
		t.Fatalf("6 days: %q", msg)
	}
	if _, msg := ScheduleDates("2026-11-02", "2026-12-17"); msg != "Rentang tanggal 7–45 hari" {
		t.Fatalf("46 days: %q", msg)
	}
	if d, msg := ScheduleDates("2026-11-02", "2026-12-16"); msg != "" || len(d) != 45 {
		t.Fatalf("45 days: %d %q", len(d), msg)
	}
	if ScheduleDateHeader("2026-11-03") != "Sel 03/11" {
		t.Fatalf("header %q", ScheduleDateHeader("2026-11-03"))
	}
}

func TestValidateScheduleUploadOK(t *testing.T) {
	c := uctx()
	res := ValidateScheduleUpload(c, sfile(c, row(4, "111", week("spg1")...), row(5, "222", week("SPG1")...)))
	if len(res.Errors) != 0 {
		t.Fatalf("errors %v", res.Errors)
	}
	// Both employees are off on Sat–Sun, so only "Tidak ada yang masuk" may appear.
	for _, w := range res.Warnings {
		if !strings.HasPrefix(w.Message, "Tidak ada yang masuk") {
			t.Fatalf("unexpected warning %+v", w)
		}
	}
	if len(res.Assignments) != 14 || res.Assignments[5].ShiftID != nil {
		t.Fatalf("assignments = %+v", res.Assignments)
	}
}

func TestValidateScheduleUploadErrors(t *testing.T) {
	cases := []struct {
		name string
		edit func(*ScheduleUploadContext, *ScheduleFile)
		want string
	}{
		{"template lain", func(c *ScheduleUploadContext, f *ScheduleFile) { f.InfoBranchID = "b2" }, "File ini bukan template PPJA 02/11/2026–08/11/2026"},
		{"tanpa info", func(c *ScheduleUploadContext, f *ScheduleFile) { f.InfoBranchID, f.InfoFrom, f.InfoTo = "", "", "" }, "Sheet Info tidak ditemukan"},
		{"kolom tanggal bergeser", func(c *ScheduleUploadContext, f *ScheduleFile) { f.DateHeaders[2] = "Kam 05/11" }, "Kolom tanggal tidak sesuai template"},
		{"tanggal lewat", func(c *ScheduleUploadContext, f *ScheduleFile) { c.Today = "2026-11-03" }, "Tanggal 02/11/2026 sudah lewat"},
		{"periode closing", func(c *ScheduleUploadContext, f *ScheduleFile) { c.ClosedDates["2026-11-04"] = true }, "Periode absensi 04/11/2026 sudah closing"},
		{"NIP tidak dikenal", func(c *ScheduleUploadContext, f *ScheduleFile) {
			f.Rows = append(f.Rows, row(6, "999", week("SPG1")...))
		}, "NIP 999 tidak dikenal atau sudah nonaktif"},
		{"NIP nonaktif", func(c *ScheduleUploadContext, f *ScheduleFile) {
			f.Rows = append(f.Rows, row(6, "444", week("SPG1")...))
		}, "NIP 444 tidak dikenal atau sudah nonaktif"},
		{"Non-Shift", func(c *ScheduleUploadContext, f *ScheduleFile) {
			f.Rows = append(f.Rows, row(6, "333", week("SPG1")...))
		}, "Cici Non-Shift, pakai jadwal tetap"},
		{"NIP ganda", func(c *ScheduleUploadContext, f *ScheduleFile) {
			f.Rows = append(f.Rows, row(9, "111", week("SPG1")...))
		}, "NIP 111 ganda (baris 4 & 9)"},
		{"kode salah ketik", func(c *ScheduleUploadContext, f *ScheduleFile) { f.Rows[0].Cells[0] = "SPG 1" }, "Kode 'SPG 1' tidak ada; maksud Anda SPG1?"},
		{"kode tidak ada", func(c *ScheduleUploadContext, f *ScheduleFile) { f.Rows[0].Cells[0] = "XX" }, "Kode 'XX' tidak ada di sheet Kode Shift"},
		{"shift nonaktif", func(c *ScheduleUploadContext, f *ScheduleFile) { f.Rows[0].Cells[0] = "old" }, "Shift OLD sudah nonaktif"},
		{"sel kosong", func(c *ScheduleUploadContext, f *ScheduleFile) { f.Rows[0].Cells[1] = " " }, "Isi kode shift atau L"},
		{"tidak ditempatkan", func(c *ScheduleUploadContext, f *ScheduleFile) {
			f.Rows = append(f.Rows, row(6, "555", week("SPG1")...))
		}, "Eka tidak ditempatkan di PPJA pada 03/11/2026"},
		{"terlalu banyak baris", func(c *ScheduleUploadContext, f *ScheduleFile) {
			for i := 0; i < MaxScheduleRows; i++ {
				f.Rows = append(f.Rows, row(10+i, "111", week("SPG1")...))
			}
		}, "Maksimal 300 karyawan per file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := uctx()
			f := sfile(c, row(4, "111", week("SPG1")...))
			tc.edit(&c, &f)
			res := ValidateScheduleUpload(c, f)
			if !hasIssue(res.Errors, tc.want) {
				t.Fatalf("want error %q, got %+v", tc.want, res.Errors)
			}
		})
	}
}

func TestValidateScheduleUploadNotPlacedDash(t *testing.T) {
	c := uctx()
	res := ValidateScheduleUpload(c, sfile(c, row(4, "555", "SPG1", "-", "-", "", "-", "-", "-")))
	if len(res.Errors) != 0 || len(res.Assignments) != 1 {
		t.Fatalf("errors %v, assignments %d (dash/empty outside placement are skipped)", res.Errors, len(res.Assignments))
	}
}

func TestValidateScheduleUploadWarnings(t *testing.T) {
	cases := []struct {
		name string
		edit func(*ScheduleUploadContext, *ScheduleFile)
		want string
	}{
		{"7 hari berturut", func(c *ScheduleUploadContext, f *ScheduleFile) {
			f.Rows[0].Cells = []string{"SPG1", "SPG1", "SPG1", "SPG1", "SPG1", "SPG1", "SPG1"}
		}, "Ana masuk 7 hari berturut-turut (02/11/2026–08/11/2026)"},
		{"overnight gap", func(c *ScheduleUploadContext, f *ScheduleFile) {
			f.Rows[0].Cells = []string{"MALAM", "SPG1", "L", "SPG1", "SPG1", "L", "L"}
		}, "Ana: jeda 2 jam antara 02/11/2026 (MALAM) dan 03/11/2026 (SPG1)"},
		{"cuti", func(c *ScheduleUploadContext, f *ScheduleFile) { c.OnLeave["e1"] = map[string]bool{"2026-11-03": true} }, "Ana cuti 03/11/2026 tapi diberi SPG1"},
		{"tidak ada yang masuk", func(c *ScheduleUploadContext, f *ScheduleFile) {}, "Tidak ada yang masuk di PPJA 07/11/2026"},
		{"sudah clock-in", func(c *ScheduleUploadContext, f *ScheduleFile) {
			c.ClockedIn["e1"] = map[string]bool{"2026-11-02": true}
		}, "1 karyawan sudah clock-in 02/11/2026; telatnya tidak dihitung ulang"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := uctx()
			f := sfile(c, row(4, "111", week("SPG1")...))
			tc.edit(&c, &f)
			res := ValidateScheduleUpload(c, f)
			if len(res.Errors) != 0 {
				t.Fatalf("unexpected errors %+v", res.Errors)
			}
			if !hasIssue(res.Warnings, tc.want) {
				t.Fatalf("want warning %q, got %+v", tc.want, res.Warnings)
			}
		})
	}
}

// EPIC-079 R3: the grid only holds shift staff, who ignore red dates; a shift
// on a national holiday is no longer warned about.
func TestValidateScheduleUploadNoHolidayWarning(t *testing.T) {
	c := uctx()
	c.Holidays["2026-11-04"] = "Libur Uji"
	res := ValidateScheduleUpload(c, sfile(c, row(4, "111", week("SPG1")...)))
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors %+v", res.Errors)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w.Message, "libur nasional") {
			t.Fatalf("holiday warning returned: %+v", w)
		}
	}
}

func TestReadScheduleFile(t *testing.T) {
	matrix := [][]string{
		{"Jadwal Kerja — PPJA — 02/11/2026 s/d 08/11/2026"},
		{"Isi dengan kode shift"},
		{"NIP", "Nama Karyawan", "Jabatan", "Sen 02/11", "Sel 03/11", "Hari Masuk", "Hari Libur"},
		{"111", "Ana", "Kasir", "SPG1", "L", "", ""},
		{"", "", "", "", "", "", ""},
	}
	info := [][]string{{"Cabang ID", "b1"}, {"Dari", "2026-11-02"}, {"Sampai", "2026-11-03"}}
	f, msg := ReadScheduleFile(matrix, info)
	if msg != "" || f.HeaderLine != 3 || len(f.DateHeaders) != 2 || len(f.Rows) != 1 || f.Rows[0].Line != 4 || f.InfoBranchID != "b1" {
		t.Fatalf("file = %+v msg %q", f, msg)
	}
	if _, msg := ReadScheduleFile([][]string{{"Nama"}}, info); msg != "Judul kolom NIP tidak ditemukan — pakai template dari halaman Jadwal Kerja" {
		t.Fatalf("msg %q", msg)
	}
}

func TestFortyHourDayOffWarning(t *testing.T) {
	const msg = "sebelum 40 jam kerja"
	// Sen..Min: the window is 7 days, so a previous day off must be in it.
	cases := []struct {
		name  string
		cells []string
		want  string // "" = no 40-hour warning
	}{
		{"4 hari sebelum libur", []string{"L", "SPG1", "SPG1", "SPG1", "SPG1", "L", "SPG1"}, "Ana libur 07/11/2026 sebelum 40 jam kerja (baru 28 jam sejak libur 02/11/2026)"},
		{"6 hari cukup", []string{"L", "SPG1", "SPG1", "SPG1", "SPG1", "SPG1", "SPG1"}, ""},
		{"6 shift malam 7 jam", []string{"L", "MALAM", "MALAM", "MALAM", "MALAM", "MALAM", "MALAM"}, ""},
		{"tanpa libur sebelumnya", []string{"SPG1", "SPG1", "SPG1", "SPG1", "L", "SPG1", "SPG1"}, ""},
		{"libur berurutan", []string{"SPG1", "L", "L", "SPG1", "SPG1", "SPG1", "SPG1"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := uctx()
			res := ValidateScheduleUpload(c, sfile(c, row(4, "111", tc.cells...)))
			if len(res.Errors) != 0 {
				t.Fatalf("errors %v", res.Errors)
			}
			n := 0
			for _, w := range res.Warnings {
				if strings.Contains(w.Message, msg) {
					n++
					if w.Message != tc.want || w.Column != "Sab 07/11" {
						t.Fatalf("warning %+v, want %q at Sab 07/11", w, tc.want)
					}
				}
			}
			if (tc.want == "") != (n == 0) || n > 1 {
				t.Fatalf("want %q, got %d matching warnings: %+v", tc.want, n, res.Warnings)
			}
		})
	}
}

func TestFortyHourDayOffWarningNeedsAnEdit(t *testing.T) {
	// Per-cell save: an existing short streak (10-12/11 between days off) is
	// not warned about while untouched, but is once one of its days is edited.
	c := uctx()
	c.EmployeesByID = map[string]UploadEmployee{"e1": c.Employees["111"]}
	c.Current = map[string]map[string]string{"e1": {"2026-11-09": "L", "2026-11-10": "SPG1", "2026-11-11": "SPG1", "2026-11-12": "SPG1", "2026-11-13": "L"}}
	c.Employees["111"].Placed["2026-11-07"] = true
	c.Employees["111"].Placed["2026-11-11"] = true
	res := ValidateScheduleCells(c, []ScheduleCellInput{{EmployeeID: "e1", Date: "2026-11-07", Code: "L"}})
	if hasIssue(res.Warnings, "sebelum 40 jam kerja") {
		t.Fatalf("unedited streak warned: %+v", res.Warnings)
	}
	res = ValidateScheduleCells(c, []ScheduleCellInput{{EmployeeID: "e1", Date: "2026-11-11", Code: "SPG1"}})
	if !hasIssue(res.Warnings, "libur 13/11/2026 sebelum 40 jam kerja (baru 21 jam sejak libur 09/11/2026)") {
		t.Fatalf("edited streak should warn: %+v", res.Warnings)
	}
}

func TestFortyHourDayOffWarningIgnoresUnknownDays(t *testing.T) {
	// Fresh roster: a lone edited shift between empty or "-" neighbours is no
	// "libur diberikan", so no 40-hour warning. Explicit L on both sides still warns.
	for _, tc := range []struct{ name, around string }{{"kosong", ""}, {"tidak ditempatkan", "-"}} {
		t.Run(tc.name, func(t *testing.T) {
			c := uctx()
			c.EmployeesByID = map[string]UploadEmployee{"e1": c.Employees["111"]}
			c.Current = map[string]map[string]string{"e1": {}}
			for _, d := range ScheduleCellWindow([]string{"2026-11-04"}) {
				c.Current["e1"][d] = tc.around
			}
			res := ValidateScheduleCells(c, []ScheduleCellInput{{EmployeeID: "e1", Date: "2026-11-04", Code: "SPG1"}})
			if len(res.Errors) != 0 || len(res.Warnings) != 0 {
				t.Fatalf("errors %+v warnings %+v", res.Errors, res.Warnings)
			}
		})
	}
	// A cleared edit next to an explicit L is unknown too.
	c := uctx()
	c.EmployeesByID = map[string]UploadEmployee{"e1": c.Employees["111"]}
	c.Current = map[string]map[string]string{"e1": {"2026-11-03": "L", "2026-11-04": "SPG1", "2026-11-05": "L"}}
	res := ValidateScheduleCells(c, []ScheduleCellInput{{EmployeeID: "e1", Date: "2026-11-05", Code: ""}})
	if hasIssue(res.Warnings, "sebelum 40 jam kerja") {
		t.Fatalf("cleared cell treated as day off: %+v", res.Warnings)
	}
	// Explicit L on both sides still warns.
	c.Current["e1"]["2026-11-05"] = "L"
	res = ValidateScheduleCells(c, []ScheduleCellInput{{EmployeeID: "e1", Date: "2026-11-04", Code: "SPG1"}})
	if !hasIssue(res.Warnings, "libur 05/11/2026 sebelum 40 jam kerja (baru 7 jam sejak libur 03/11/2026)") {
		t.Fatalf("L, SPG1, L should warn: %+v", res.Warnings)
	}
}
