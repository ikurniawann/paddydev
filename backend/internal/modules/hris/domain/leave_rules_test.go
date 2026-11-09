package domain

import "testing"

func leaveType(t *testing.T, code string) LeaveType {
	t.Helper()
	lt, ok := LookupLeaveType(code)
	if !ok {
		t.Fatalf("%s tidak ada di katalog", code)
	}
	return lt
}

func TestLeaveTimingError(t *testing.T) {
	const today = "2026-11-03"
	sick := "Sakit diajukan pada hari sakit (tanggal mulai = hari ini)"
	cases := []struct{ code, start, want string }{
		{"annual", "2026-11-06", ""},
		{"annual", "2026-11-05", "Cuti Tahunan diajukan paling lambat H-3 (mulai 06-11-2026 atau sesudahnya)"},
		{"pilgrimage", "2026-12-03", ""},
		{"pilgrimage", "2026-12-02", "Cuti Ibadah Haji/Umrah diajukan paling lambat H-30 (mulai 03-12-2026 atau sesudahnya)"},
		{"sick", "2026-11-03", ""},
		{"sick", "2026-11-02", sick},
		{"sick", "2026-11-04", sick},
		{"bereavement", "2026-10-30", ""},
	}
	for _, c := range cases {
		if got := LeaveTimingError(leaveType(t, c.code), c.start, today); got != c.want {
			t.Errorf("%s %s: got %q, want %q", c.code, c.start, got, c.want)
		}
	}
}

func TestLeaveSizeError(t *testing.T) {
	cases := []struct {
		code, start, end string
		days             int
		want             string
	}{
		{"marriage", "2026-11-09", "2026-11-12", 4, "Cuti Menikah maksimal 3 hari kerja"},
		{"marriage", "2026-11-09", "2026-11-11", 3, ""},
		{"maternity", "2026-11-01", "2027-01-29", 62, ""},
		{"maternity", "2026-11-01", "2027-01-30", 63, "Cuti Melahirkan maksimal 90 hari kalender"},
		{"pilgrimage", "2026-11-01", "2026-12-10", 30, ""},
		{"pilgrimage", "2026-11-01", "2026-12-11", 30, "Cuti Ibadah Haji/Umrah maksimal 40 hari kalender"},
	}
	for _, c := range cases {
		if got := LeaveSizeError(leaveType(t, c.code), c.start, c.end, c.days); got != c.want {
			t.Errorf("%s %s..%s: got %q, want %q", c.code, c.start, c.end, got, c.want)
		}
	}
}

func TestLeaveGenderError(t *testing.T) {
	if got := LeaveGenderError(leaveType(t, "maternity"), "male"); got != "Cuti Melahirkan hanya untuk karyawan perempuan" {
		t.Errorf("maternity male = %q", got)
	}
	if got := LeaveGenderError(leaveType(t, "maternity"), ""); got != "" {
		t.Errorf("maternity tanpa gender = %q", got)
	}
	if got := LeaveGenderError(leaveType(t, "paternity"), "female"); got != "Cuti Istri Melahirkan hanya untuk karyawan laki-laki" {
		t.Errorf("paternity female = %q", got)
	}
	if got := LeaveGenderError(leaveType(t, "annual"), "female"); got != "" {
		t.Errorf("annual = %q", got)
	}
}

func TestBirthdayInYear(t *testing.T) {
	cases := []struct {
		birth string
		year  int
		want  string
		ok    bool
	}{
		{"1992-02-29", 2027, "2027-02-28", true},
		{"1992-02-29", 2028, "2028-02-29", true},
		{"1995-11-20", 2026, "2026-11-20", true},
		{"", 2026, "", false},
		{"bukan", 2026, "", false},
	}
	for _, c := range cases {
		got, ok := BirthdayInYear(c.birth, c.year)
		if got != c.want || ok != c.ok {
			t.Errorf("BirthdayInYear(%q, %d) = %q %v", c.birth, c.year, got, ok)
		}
	}
}

func TestInitialContractPassed(t *testing.T) {
	cases := []struct {
		name   string
		first  *FirstContract
		status string
		on     string
		want   bool
	}{
		{"pkwt selesai sebelum", &FirstContract{EndDate: "2026-10-31"}, "contract", "2026-11-10", true},
		{"pkwt selesai hari itu", &FirstContract{EndDate: "2026-10-31"}, "contract", "2026-10-31", false},
		{"pkwtt tanpa probation", &FirstContract{}, "permanent", "2026-11-10", true},
		{"pkwtt probation belum lewat", &FirstContract{ProbationEnd: "2026-12-01"}, "probation", "2026-11-10", false},
		{"pkwtt probation lewat", &FirstContract{ProbationEnd: "2026-11-01"}, "permanent", "2026-11-10", true},
		{"tanpa kontrak tetap", nil, "permanent", "2026-11-10", true},
		{"tanpa kontrak probation", nil, "probation", "2026-11-10", false},
	}
	for _, c := range cases {
		if got := InitialContractPassed(c.first, c.status, c.on); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

// EPIC-094 R14: SLA pengajuan dari katalog, bukan dari tipe buatan tes —
// Cuti Tahunan H-3, Cuti Ibadah Haji/Umrah H-30, Sakit hari H.
func TestLeaveSLACatalog(t *testing.T) {
	const today = "2026-11-06"
	get := func(code string) LeaveType {
		lt, ok := LookupLeaveType(code)
		if !ok {
			t.Fatalf("leave type %s missing", code)
		}
		return lt
	}
	cases := []struct {
		code, start string
		refused     bool
	}{
		{"annual", "2026-11-08", true}, {"annual", "2026-11-09", false},
		{"pilgrimage", "2026-12-05", true}, {"pilgrimage", "2026-12-06", false},
		{"sick", "2026-11-06", false}, {"sick", "2026-11-07", true}, {"sick", "2026-11-05", true},
	}
	for _, c := range cases {
		if got := LeaveTimingError(get(c.code), c.start, today); (got != "") != c.refused {
			t.Errorf("%s mulai %s: %q, refused want %v", c.code, c.start, got, c.refused)
		}
	}
}
