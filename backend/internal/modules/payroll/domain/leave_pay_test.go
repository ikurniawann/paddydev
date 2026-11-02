package domain

import "testing"

// EPIC-084 R8: hari cuti → PIP ("personal"), PIS ("sick") atau full paid ("").
func TestLeaveDayKind(t *testing.T) {
	cases := []struct{ name, kind, start, day, join, want string }{
		{"melahirkan genap 1 tahun", "maternity", "2026-11-02", "2026-11-03", "2025-11-02", ""},
		{"melahirkan kurang 1 hari", "maternity", "2026-11-02", "2026-11-03", "2025-11-03", "personal"},
		{"keguguran < 1 tahun", "miscarriage", "2026-11-02", "2026-11-03", "2025-11-03", "personal"},
		{"keguguran ≥ 1 tahun", "miscarriage", "2026-11-02", "2026-11-03", "2024-01-01", ""},
		{"haji hari ke-14", "pilgrimage", "2026-10-25", "2026-11-07", "2026-06-01", ""},
		{"haji hari ke-15 < 1 tahun", "pilgrimage", "2026-10-25", "2026-11-08", "2026-06-01", "personal"},
		{"haji hari ke-15 ≥ 1 tahun", "pilgrimage", "2026-10-25", "2026-11-08", "2025-01-01", ""},
		{"izin leader", "leader", "2026-11-02", "2026-11-02", "2026-06-01", ""},
		{"birthday", "birthday", "2026-11-02", "2026-11-02", "2026-06-01", ""},
		{"sakit", "sick", "2026-11-02", "2026-11-02", "2020-01-01", "sick"},
		{"izin pribadi", "personal", "2026-11-02", "2026-11-02", "2020-01-01", "personal"},
		{"join tak terbaca dianggap ≥ 1 tahun", "maternity", "2026-11-02", "2026-11-03", "", ""},
	}
	for _, c := range cases {
		if got := LeaveDayKind(c.kind, c.start, c.day, c.join); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// ES-16: Cuti Melahirkan tidak lagi memotong transport + makan.
func TestMaternityNoLongerInPIS(t *testing.T) {
	c := DefaultConfig()
	c.SickLeaveDeduction = true
	r := Calculate(base4(func(in *Input) { in.MaternityLeaveDays = 5 }), c)
	eq(t, "melahirkan full paid", r.SickLeaveDeduction, 0)
}

// Review Focus 4: hari ke-15 haji dihitung dari tanggal mulai cuti, bukan awal
// periode; karyawan < 1 tahun dipotong PIP untuk hari kerja sesudahnya.
func TestBuildInputPilgrimageAndMaternityByService(t *testing.T) {
	sal := Salary{BaseSalary: "2200000.00"}
	rec := WorkforceRecords{Schedule: weekdaySchedule(), Leaves: []Leave{
		{ptr("2026-08-25"), ptr("2026-09-18"), "pilgrimage"}, // hari ke-15 = 08-09-2026
		{ptr("2026-09-28"), ptr("2026-09-29"), "maternity"},
	}}
	fresh := Employee{ID: "e", JoinDate: "2026-03-01", EmploymentStatus: "contract"}
	in := BuildInputRange(fresh, sal, rec, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	// Haji dipotong 08–11 & 14–18 Sep (9 hari kerja; 01–07 masih 14 hari pertama),
	// melahirkan 28–29 Sep (2 hari).
	if in.PersonalLeaveDays != 11 || in.MaternityLeaveDays != 2 {
		t.Fatalf("< 1 tahun: personal=%v maternity=%v, want 11 dan 2", in.PersonalLeaveDays, in.MaternityLeaveDays)
	}
	senior := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "permanent"}
	in = BuildInputRange(senior, sal, rec, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if in.PersonalLeaveDays != 0 {
		t.Fatalf("≥ 1 tahun: personal=%v, want 0", in.PersonalLeaveDays)
	}
}
