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
