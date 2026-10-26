package domain

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// checkLike stands in for hris.validateLeaveBalance: the same order and the
// same messages for the rules the upload relies on.
func checkLike(c LeaveBalanceCheck) string {
	switch {
	case c.Year != 2026 && c.Year != 2027:
		return "Tahun saldo hanya boleh 2026 (tahun berjalan) atau 2027 (tahun depan)"
	case strings.TrimSpace(c.Reason) == "":
		return "Alasan perubahan wajib diisi"
	case math.Mod(c.Values.Total*2, 1) != 0:
		return "Jatah harus kelipatan 0,5"
	case c.Values.Total+c.Values.Adjustment-c.Used < 0:
		return "Sisa jatah tidak boleh kurang dari 0"
	case c.Values.CarryDays < c.CarryUsed:
		return "Sisa bawaan tidak boleh kurang dari bawaan yang sudah terpakai"
	}
	return ""
}

func leaveCtx() LeaveBalanceUploadContext {
	return LeaveBalanceUploadContext{
		Year: 2026, BranchID: "b1", BranchName: "PPJA", Today: "2026-10-05", Check: checkLike,
		Employees: map[string]LeaveBalanceEmployee{
			"111": {ID: "e1", Name: "Ana", Active: true, InScope: true, Exists: true,
				Current: LeaveBalanceValues{Total: 12}, Used: 5},
			"222": {ID: "e2", Name: "Budi", Active: true, InScope: true, Exists: false,
				Current: LeaveBalanceValues{Total: 9.5}},
			"333": {ID: "e3", Name: "Cici", Active: false, InScope: false, Current: LeaveBalanceValues{Total: 12}},
			"444": {ID: "e4", Name: "Dodi", Active: true, InScope: false, Current: LeaveBalanceValues{Total: 12}},
			"555": {ID: "e5", Name: "Eka", Active: true, InScope: true, Exists: true,
				Current: LeaveBalanceValues{Total: 12, CarryDays: 2, CarryExpires: "2027-03-31"}, Used: 0, CarryUsed: 1.5},
		},
	}
}

func leaveMatrix(rows ...[]string) [][]string {
	m := [][]string{
		{"Saldo Cuti 2026"},
		{"NIP", "Nama", "Cabang", "Terpakai", "Jatah", "Penyesuaian", "Sisa Bawaan", "Kedaluwarsa", "Alasan"},
	}
	for _, r := range rows {
		m = append(m, append([]string{r[0], "", "", ""}, r[1:]...))
	}
	return m
}

var leaveInfo = [][]string{{"Tahun", "2026"}, {"Cabang ID", "b1"}}

func readLeave(t *testing.T, rows ...[]string) LeaveBalanceFile {
	t.Helper()
	f, msg := ReadLeaveBalanceFile(leaveMatrix(rows...), leaveInfo)
	if msg != "" {
		t.Fatal(msg)
	}
	return f
}

func hasLeaveIssue(is []UploadIssue, line int, sub string) bool {
	for _, i := range is {
		if i.Line == line && strings.Contains(i.Column+" "+i.Message, sub) {
			return true
		}
	}
	return false
}

func TestReadLeaveBalanceFile(t *testing.T) {
	f := readLeave(t,
		[]string{"111", "12", "0", "0", "", ""},
		[]string{"", "", "", "", "", ""}, // blank: skipped
		[]string{"222", "10", "1", "2", "31/03/2027", "Koreksi"})
	if f.HeaderLine != 2 || len(f.Rows) != 2 || f.Rows[1].Line != 5 || f.Rows[1].Expires != "31/03/2027" || f.Rows[1].Reason != "Koreksi" {
		t.Fatalf("file = %+v", f)
	}
	if f.InfoYear != "2026" || f.InfoBranchID != "b1" || !f.InfoFound {
		t.Fatalf("info = %+v", f)
	}
	// Columns are found by header (NIP stays first), so reordered or differently cased columns still read.
	m := [][]string{{"nip", "alasan", "JATAH", "penyesuaian", "sisa bawaan *", "kedaluwarsa"}, {"111", "x", "7", "", "", ""}}
	if g, msg := ReadLeaveBalanceFile(m, leaveInfo); msg != "" || g.Rows[0].NIP != "111" || g.Rows[0].Total != "7" || g.Rows[0].Reason != "x" {
		t.Fatalf("by header: %+v %q", g, msg)
	}
	if _, msg := ReadLeaveBalanceFile([][]string{{"Nama"}}, leaveInfo); !strings.Contains(msg, "Judul kolom NIP tidak ditemukan") {
		t.Fatalf("no NIP header: %q", msg)
	}
	if _, msg := ReadLeaveBalanceFile([][]string{{"NIP", "Jatah"}}, leaveInfo); !strings.Contains(msg, "Kolom Penyesuaian, Sisa Bawaan, Kedaluwarsa, Alasan tidak ditemukan") {
		t.Fatalf("missing columns: %q", msg)
	}
}

func TestParseLeaveDays(t *testing.T) {
	for in, want := range map[string]float64{"9.5": 9.5, "9,5": 9.5, " 12 ": 12, "-2": -2} {
		if v, ok := ParseLeaveDays(in); !ok || v != want {
			t.Errorf("ParseLeaveDays(%q) = %v %v", in, v, ok)
		}
	}
	for _, in := range []string{"", "abc", "NaN", "Inf", "1e999"} {
		if _, ok := ParseLeaveDays(in); ok {
			t.Errorf("ParseLeaveDays(%q) must fail", in)
		}
	}
}

func TestValidateLeaveBalanceUploadChanges(t *testing.T) {
	f := readLeave(t,
		[]string{"111", "12", "0", "0", "", "tidak dibaca"}, // same as stored: ignored, reason or not
		[]string{"222", "9.5", "0", "0", "", ""},            // equals the default of a missing row: same
		[]string{"555", "14", "1", "2", "2027-03-31T00:00:00.000Z", "Koreksi"})
	res := ValidateLeaveBalanceUpload(f, leaveCtx())
	if len(res.Errors) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("issues %v %v", res.Errors, res.Warnings)
	}
	if res.Same != 2 || len(res.Changes) != 1 || res.Rows != 3 || len(res.EmployeeIDs) != 3 {
		t.Fatalf("result = %+v", res)
	}
	c := res.Changes[0]
	if c.NIP != "555" || c.Name != "Eka" || c.New || c.Reason != "Koreksi" || c.Line != 5 ||
		c.After != (LeaveBalanceValues{Total: 14, Adjustment: 1, CarryDays: 2, CarryExpires: "2027-03-31"}) {
		t.Fatalf("change = %+v", c)
	}
}

func TestValidateLeaveBalanceUploadCreatesMissingRow(t *testing.T) {
	res := ValidateLeaveBalanceUpload(readLeave(t, []string{"222", "10", "0", "0", "", "Jatah khusus"}), leaveCtx())
	if len(res.Errors) != 0 || len(res.Changes) != 1 || !res.Changes[0].New || res.Changes[0].Before.Total != 9.5 {
		t.Fatalf("result = %+v", res)
	}
}

func TestValidateLeaveBalanceUploadErrors(t *testing.T) {
	cases := []struct {
		name string
		row  []string
		want string
	}{
		{"NIP tidak dikenal", []string{"999", "12", "0", "0", "", "x"}, "NIP 999 tidak dikenal atau karyawan sudah nonaktif"},
		{"nonaktif", []string{"333", "12", "0", "0", "", "x"}, "tidak dikenal atau karyawan sudah nonaktif"},
		{"luar cabang", []string{"444", "13", "0", "0", "", "x"}, "Karyawan tidak termasuk cabang PPJA"},
		{"bukan kelipatan 0,5", []string{"111", "12.25", "0", "0", "", "x"}, "Jatah harus kelipatan 0,5"},
		{"sisa < 0", []string{"111", "4", "0", "0", "", "x"}, "Sisa jatah tidak boleh kurang dari 0"},
		{"bawaan < terpakai", []string{"555", "12", "0", "1", "2027-03-31", "x"}, "Sisa bawaan tidak boleh kurang dari bawaan yang sudah terpakai"},
		{"alasan kosong", []string{"111", "13", "0", "0", "", ""}, "Alasan perubahan wajib diisi"},
		{"jatah kosong", []string{"111", "", "0", "0", "", "x"}, "wajib diisi"},
		{"jatah bukan angka", []string{"111", "dua belas", "0", "0", "", "x"}, "harus berupa angka"},
		{"tanggal rusak", []string{"111", "12", "0", "1", "besok", "x"}, "bukan tanggal yang valid"},
		{"NIP kosong", []string{"", "12", "0", "0", "", "x"}, "NIP wajib diisi"},
		{"NIP ilmiah", []string{"2.02E+08", "12", "0", "0", "", "x"}, "NIP berubah jadi format angka Excel"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := ValidateLeaveBalanceUpload(readLeave(t, c.row), leaveCtx())
			if len(res.Errors) != 1 || !hasLeaveIssue(res.Errors, 3, c.want) || len(res.Changes) != 0 {
				t.Fatalf("errors %v changes %v, want %q on line 3", res.Errors, res.Changes, c.want)
			}
		})
	}
}

func TestValidateLeaveBalanceUploadDuplicateNIP(t *testing.T) {
	res := ValidateLeaveBalanceUpload(readLeave(t,
		[]string{"111", "13", "0", "0", "", "x"},
		[]string{"111", "14", "0", "0", "", "x"}), leaveCtx())
	if !hasLeaveIssue(res.Errors, 4, "NIP 111 ganda (sudah ada di baris 3)") || len(res.Errors) != 1 || len(res.Changes) != 1 {
		t.Fatalf("result = %+v", res)
	}
}

func TestValidateLeaveBalanceUploadWarnings(t *testing.T) {
	res := ValidateLeaveBalanceUpload(readLeave(t,
		[]string{"111", "25", "0", "0", "", "Besar"},
		[]string{"222", "10", "0", "2", "", "Bawaan"},
		[]string{"555", "12", "0", "2", "2026-09-30", "Lewat"}), leaveCtx())
	if len(res.Errors) != 0 || len(res.Changes) != 3 {
		t.Fatalf("result = %+v", res)
	}
	for _, w := range []struct {
		line int
		sub  string
	}{{3, "jatah 25 hari lebih dari 24"}, {4, "sisa bawaan tanpa tanggal kedaluwarsa dianggap tidak berlaku"}, {5, "tanggal kedaluwarsa sudah lewat"}} {
		if !hasLeaveIssue(res.Warnings, w.line, w.sub) {
			t.Errorf("missing warning %q on line %d: %v", w.sub, w.line, res.Warnings)
		}
	}
	// Unchanged rows never warn: re-uploading a stored 25-day row is silent.
	ctx := leaveCtx()
	ctx.Employees["111"] = LeaveBalanceEmployee{ID: "e1", Name: "Ana", Active: true, InScope: true, Exists: true, Current: LeaveBalanceValues{Total: 25}}
	if again := ValidateLeaveBalanceUpload(readLeave(t, []string{"111", "25", "0", "0", "", "Besar"}), ctx); len(again.Warnings) != 0 || again.Same != 1 {
		t.Fatalf("again = %+v", again)
	}
}

func TestValidateLeaveBalanceUploadFileLevelErrors(t *testing.T) {
	row := []string{"111", "13", "0", "0", "", "x"}
	cases := map[string]func(*LeaveBalanceFile, *LeaveBalanceUploadContext) string{
		"sheet Info hilang": func(f *LeaveBalanceFile, c *LeaveBalanceUploadContext) string {
			f.InfoFound = false
			return "Sheet Info tidak ditemukan"
		},
		"tahun tidak cocok": func(f *LeaveBalanceFile, c *LeaveBalanceUploadContext) string {
			f.InfoYear = "2027"
			return "File ini bukan template saldo cuti 2026 untuk cabang PPJA"
		},
		"cabang tidak cocok": func(f *LeaveBalanceFile, c *LeaveBalanceUploadContext) string {
			f.InfoBranchID = ""
			return "File ini bukan template saldo cuti 2026 untuk cabang PPJA"
		},
		"semua cabang vs satu": func(f *LeaveBalanceFile, c *LeaveBalanceUploadContext) string {
			c.BranchID = ""
			return "untuk semua cabang"
		},
		"tahun tidak boleh": func(f *LeaveBalanceFile, c *LeaveBalanceUploadContext) string {
			f.InfoYear, c.Year = "2025", 2025
			return "Tahun saldo hanya boleh 2026"
		},
		"terlalu banyak baris": func(f *LeaveBalanceFile, c *LeaveBalanceUploadContext) string {
			for len(f.Rows) <= MaxLeaveBalanceRows {
				f.Rows = append(f.Rows, f.Rows[0])
			}
			return fmt.Sprintf("maksimal %d baris", MaxLeaveBalanceRows)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f, c := readLeave(t, row), leaveCtx()
			want := mutate(&f, &c)
			res := ValidateLeaveBalanceUpload(f, c)
			if len(res.Errors) != 1 || !hasLeaveIssue(res.Errors, 0, want) || len(res.Changes) != 0 {
				t.Fatalf("errors %v, want %q at line 0", res.Errors, want)
			}
		})
	}
}
