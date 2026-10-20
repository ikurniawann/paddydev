package domain

import "testing"

// weekdaySchedule (Senin–Jumat, shift, berlaku sejak 2026-01-01) sudah ada di kpi_collect_test.go.

func TestIsWorkDate(t *testing.T) {
	holidays := map[string]bool{"2026-09-16": true}
	sched := weekdaySchedule()
	for date, want := range map[string]bool{
		"2026-09-14": true,  // Senin
		"2026-09-19": false, // Sabtu tanpa shift
		"2026-09-16": true,  // libur nasional, tapi jadwal punya shift: jadwal yang menentukan
	} {
		if got := IsWorkDate(date, sched, holidays); got != want {
			t.Errorf("IsWorkDate(%s, jadwal) = %v, want %v", date, got, want)
		}
	}
	// Tanpa jadwal: Senin–Jumat dan bukan libur nasional.
	for date, want := range map[string]bool{"2026-09-14": true, "2026-09-16": false, "2026-09-20": false} {
		if got := IsWorkDate(date, nil, holidays); got != want {
			t.Errorf("IsWorkDate(%s, tanpa jadwal) = %v, want %v", date, got, want)
		}
	}
}

// Periode 1–30 Sep 2026, Senin–Jumat = 22 hari kerja. Hadir 18 hari; cuti tahunan
// 2 hari kerja; sakit Jumat 11 – Senin 14 (2 hari kerja); satu hari cuti tahunan juga
// punya absensi efektif (tidak dihitung dua kali); melahirkan 1 hari; unpaid 1 hari.
func TestBuildInputPaidDays(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "permanent"}
	sal := Salary{BaseSalary: "2200000.00"}
	var att []AttendanceRow
	for _, d := range []string{"01", "02", "03", "04", "07", "08", "09", "10", "15", "16", "17", "18", "21", "22", "23", "24", "25", "28"} {
		att = append(att, AttendanceRow{Date: "2026-09-" + d, Status: "present", HasClockOut: true})
	}
	rec := WorkforceRecords{Schedule: weekdaySchedule(), Attendance: att, Leaves: []Leave{
		{ptr("2026-09-28"), ptr("2026-09-29"), "annual"}, // 28 sudah hadir → hanya 29 yang dihitung
		{ptr("2026-09-11"), ptr("2026-09-14"), "sick"},   // Jumat + Senin = 2 hari kerja
		{ptr("2026-09-30"), ptr("2026-09-30"), "maternity"},
		{ptr("2026-09-29"), ptr("2026-09-29"), "unpaid"}, // tidak masuk hari dibayar
	}}
	in := BuildInputRange(emp, sal, rec, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if in.AttendanceDays != 18 {
		t.Fatalf("AttendanceDays = %v, want 18", in.AttendanceDays)
	}
	if in.SickLeaveDays != 2 || in.MaternityLeaveDays != 1 || in.UnpaidLeaveDays != 1 {
		t.Fatalf("sick=%v maternity=%v unpaid=%v", in.SickLeaveDays, in.MaternityLeaveDays, in.UnpaidLeaveDays)
	}
	// 18 hadir + sick 11 & 14 + maternity 30 = 21. Tanggal 28 hadir (cuti tidak dihitung
	// dua kali); tanggal 29 tercakup unpaid sehingga tidak dibayar meski ada annual.
	if in.PaidDays != 21 {
		t.Fatalf("PaidDays = %v, want 21", in.PaidDays)
	}
}

// K-12: pembagi = hari kerja terjadwal periode; pembagi struktur hanya tanpa jadwal.
func TestDivisorFollowsSchedule(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "permanent"}
	sal := Salary{BaseSalary: "2200000.00", WorkingDaysDivisor: "26.00"}
	withSchedule := BuildInputRange(emp, sal, WorkforceRecords{Schedule: weekdaySchedule()}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if withSchedule.DivisorDays != 22 {
		t.Fatalf("pembagi dengan jadwal = %v, want 22 (hari kerja terjadwal)", withSchedule.DivisorDays)
	}
	noSchedule := BuildInputRange(emp, sal, WorkforceRecords{}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if noSchedule.DivisorDays != 26 {
		t.Fatalf("pembagi tanpa jadwal = %v, want 26 (struktur)", noSchedule.DivisorDays)
	}
	daily := sal
	daily.PayBasis = ptr("daily")
	daily.WorkingDaysDivisor = nil
	if d := BuildInputRange(emp, daily, WorkforceRecords{Schedule: weekdaySchedule()}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false).DivisorDays; d != 0 {
		t.Fatalf("daily tanpa pembagi struktur = %v, want 0 (tidak berubah)", d)
	}
}

func TestOvertimeUnderOneHourNotPaid(t *testing.T) {
	att := []AttendanceRow{
		{Date: "2026-09-07", Status: "present", HasClockOut: true, OvertimeHours: "0.50"},
		{Date: "2026-09-08", Status: "present", HasClockOut: true, OvertimeHours: "2.00"},
	}
	req := []OvertimeRequest{{Date: "2026-09-07", Hours: 0.5}, {Date: "2026-09-08", Hours: 2}}
	got := SplitOvertimeWith(req, att, nil, nil, true)
	if got.Regular != 2 {
		t.Fatalf("Regular = %v, want 2 (0,5 jam tidak dibayar)", got.Regular)
	}
}

func TestServiceOverOneYear(t *testing.T) {
	for _, c := range []struct {
		join, end string
		want      bool
	}{
		{"2025-10-20", "2026-10-20", false}, // tepat 1 tahun: belum lebih
		{"2025-10-19", "2026-10-20", true},
		{"2026-01-01", "2026-10-20", false},
		{"bukan-tanggal", "2026-10-20", false},
	} {
		if got := ServiceOverOneYear(c.join, c.end); got != c.want {
			t.Errorf("ServiceOverOneYear(%s, %s) = %v, want %v", c.join, c.end, got, c.want)
		}
	}
}

func TestHealthSubsidyOverridesStructure(t *testing.T) {
	sal := Salary{BaseSalary: "2200000.00", HealthAllowance: "50000.00"}
	opts := DefaultBuildOptions()
	opts.HealthSubsidyAmount = 100_000
	senior := BuildInputRange(Employee{ID: "a", JoinDate: "2024-01-01"}, sal, WorkforceRecords{}, nil, nil, 10, 2026, "2026-09-21", "2026-10-20", false, opts)
	if senior.HealthAllowance != 100_000 || !senior.HealthEligible {
		t.Fatalf("senior: allowance=%v eligible=%v", senior.HealthAllowance, senior.HealthEligible)
	}
	junior := BuildInputRange(Employee{ID: "b", JoinDate: "2026-01-01"}, sal, WorkforceRecords{}, nil, nil, 10, 2026, "2026-09-21", "2026-10-20", false, opts)
	if junior.HealthEligible {
		t.Fatal("junior (< 1 tahun) tidak berhak subsidi")
	}
	// Setting 0: perilaku lama (nominal struktur).
	old := BuildInputRange(Employee{ID: "a", JoinDate: "2024-01-01"}, sal, WorkforceRecords{}, nil, nil, 10, 2026, "2026-09-21", "2026-10-20", false)
	if old.HealthAllowance != 50_000 {
		t.Fatalf("tanpa setting: allowance=%v, want 50.000", old.HealthAllowance)
	}
}

// Resign 9 Sep (Rabu) dalam periode 1–30 Sep: hari terjadwal 1–9 Sep = 7 dari 22.
func TestLeaverProratedToEndDate(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "resigned", EndDate: ptr("2026-09-09")}
	sal := Salary{BaseSalary: "2200000.00"}
	att := []AttendanceRow{
		{Date: "2026-09-01", Status: "present", HasClockOut: true},
		{Date: "2026-09-15", Status: "present", HasClockOut: true}, // setelah end_date: diabaikan
	}
	in := BuildInputRange(emp, sal, WorkforceRecords{Schedule: weekdaySchedule(), Attendance: att}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if in.ProrateFactor == nil || Round(*in.ProrateFactor*1000) != Round(7.0/22*1000) {
		t.Fatalf("factor = %v, want 7/22", in.ProrateFactor)
	}
	if in.AttendanceDays != 1 {
		t.Fatalf("AttendanceDays = %v, want 1 (absen setelah end_date diabaikan)", in.AttendanceDays)
	}
	r := Calculate(in, DefaultConfig())
	eq(t, "pokok monthly × 7/22", r.BaseSalary, 700_000)
}

// Alur Resign menulis employee_shifts.effective_to = end_date. Ukuran periode penuh
// (pembagi dan penyebut faktor cakupan) tetap dari seluruh periode: 7 dari 22.
func TestLeaverResignFlowScheduleTruncated(t *testing.T) {
	ed := "2026-09-09"
	var sched []ShiftRow
	for _, r := range weekdaySchedule() {
		r.EffectiveTo = &ed
		sched = append(sched, r)
	}
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "resigned", EndDate: &ed}
	var att []AttendanceRow
	for _, d := range []string{"01", "02", "03", "04", "07", "08", "09"} {
		att = append(att, AttendanceRow{Date: "2026-09-" + d, Status: "present", HasClockOut: true})
	}
	rec := WorkforceRecords{Schedule: sched, Attendance: att}

	monthly := Salary{BaseSalary: "2200000.00"}
	in := BuildInputRange(emp, monthly, rec, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if in.ProrateFactor == nil || Round(*in.ProrateFactor*1000) != Round(7.0/22*1000) {
		t.Fatalf("factor = %v, want 7/22", *in.ProrateFactor)
	}
	if in.DivisorDays != 22 {
		t.Fatalf("pembagi = %v, want 22 (hari terjadwal seluruh periode)", in.DivisorDays)
	}
	eq(t, "pokok monthly × 7/22", Calculate(in, DefaultConfig()).BaseSalary, 700_000)

	prorated := monthly
	prorated.PayBasis = ptr("monthly_prorated")
	in = BuildInputRange(emp, prorated, rec, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if in.DivisorDays != 22 || in.PaidDays > 7 {
		t.Fatalf("monthly_prorated: pembagi = %v (want 22), hari dibayar = %v (want ≤ 7)", in.DivisorDays, in.PaidDays)
	}
	if b := Calculate(in, DefaultConfig()).BaseSalary; b > 700_000 {
		t.Fatalf("pokok monthly_prorated = %v, want ≤ 700.000", b)
	}
}

// R11 tentang karyawan keluar: karyawan AKTIF dengan end_date di masa depan di dalam
// periode (mis. kontrak akan berakhir) tidak dipotong — dibayar sebulan penuh.
func TestActiveEmployeeEndDateNotClipped(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "contract", EndDate: ptr("2026-09-09"), IsActive: true}
	sal := Salary{BaseSalary: "2200000.00"}
	att := []AttendanceRow{
		{Date: "2026-09-01", Status: "present", HasClockOut: true},
		{Date: "2026-09-15", Status: "present", HasClockOut: true},
	}
	in := BuildInputRange(emp, sal, WorkforceRecords{Schedule: weekdaySchedule(), Attendance: att}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if in.ProrateFactor == nil || *in.ProrateFactor != 1 {
		t.Fatalf("factor = %v, want 1 (karyawan aktif tidak dipotong)", *in.ProrateFactor)
	}
	if in.AttendanceDays != 2 {
		t.Fatalf("AttendanceDays = %v, want 2", in.AttendanceDays)
	}
	eq(t, "pokok monthly penuh", Calculate(in, DefaultConfig()).BaseSalary, 2_200_000)
}

// Tanpa jadwal: hari dibayar tidak melebihi hari kerja periode, dan pokok
// monthly_prorated / jabatan per_day tidak melebihi 100% (× faktor cakupan).
// Sep 2026 = 22 hari kerja Senin–Jumat.
func TestPaidDaysCappedWithoutSchedule(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "permanent"}
	weekdays := func(skip ...string) []AttendanceRow {
		var att []AttendanceRow
		for _, d := range EachDate("2026-09-01", "2026-09-30") {
			skipped := false
			for _, s := range skip {
				skipped = skipped || s == d
			}
			if ISODayOfWeek(d) <= 5 && !skipped {
				att = append(att, AttendanceRow{Date: d, Status: "present", HasClockOut: true})
			}
		}
		return att
	}
	sick := []Leave{{ptr("2026-09-29"), ptr("2026-09-30"), "sick"}}
	twentyPlusSick := WorkforceRecords{Attendance: weekdays("2026-09-29", "2026-09-30"), Leaves: sick}
	sal := Salary{BaseSalary: "2000000.00", PayBasis: ptr("monthly_prorated"), PositionAllowance: "400000.00", PositionMode: ptr("per_day")}

	// (a) pembagi struktur kosong: pembagi = 20 hari hadir, hari dibayar 22.
	in := BuildInputRange(emp, sal, twentyPlusSick, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	r := Calculate(in, DefaultConfig())
	if r.BaseSalary > 2_000_000 || r.PositionAllowance > 400_000 {
		t.Fatalf("(a) tanpa pembagi: pokok = %v (≤ 2.000.000), jabatan = %v (≤ 400.000)", r.BaseSalary, r.PositionAllowance)
	}
	// (b) pembagi struktur 20 pada bulan 22 hari kerja.
	sal20 := sal
	sal20.WorkingDaysDivisor = "20"
	in = BuildInputRange(emp, sal20, twentyPlusSick, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	r = Calculate(in, DefaultConfig())
	if r.BaseSalary > 2_000_000 || r.PositionAllowance > 400_000 {
		t.Fatalf("(b) pembagi 20: pokok = %v (≤ 2.000.000), jabatan = %v (≤ 400.000)", r.BaseSalary, r.PositionAllowance)
	}
	// Hadir penuh dengan pembagi 20: tepat 100%.
	in = BuildInputRange(emp, sal20, WorkforceRecords{Attendance: weekdays()}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	r = Calculate(in, DefaultConfig())
	eq(t, "pokok hadir penuh pembagi 20", r.BaseSalary, 2_000_000)
	eq(t, "jabatan hadir penuh pembagi 20", r.PositionAllowance, 400_000)

	// Hadir 22 hari kerja + 2 Sabtu: hari dibayar dibatasi 22 hari kerja periode.
	extra := append(weekdays(), AttendanceRow{Date: "2026-09-05", Status: "present", HasClockOut: true},
		AttendanceRow{Date: "2026-09-12", Status: "present", HasClockOut: true})
	in = BuildInputRange(emp, sal20, WorkforceRecords{Attendance: extra}, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false)
	if in.PaidDays != 22 {
		t.Fatalf("PaidDays = %v, want 22 (hari kerja periode)", in.PaidDays)
	}
}
