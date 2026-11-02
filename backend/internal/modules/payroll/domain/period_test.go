package domain

import "testing"

// rosterDay is a hris.v_employee_schedule per-date row (EPIC-079 R7): a
// one-day pattern on date with priority 0; shift nil = libur roster.
func rosterDay(date string, shift *string) ShiftRow {
	to := date
	return ShiftRow{DayOfWeek: ISODayOfWeek(date), ShiftID: shift, EffectiveFrom: date, EffectiveTo: &to, Priority: 0}
}

// novemberRoster: roster per tanggal November 2026, 26 shift dan libur roster
// pada Selasa 3, 10, 17, 24.
func novemberRoster() []ShiftRow {
	off := map[string]bool{"2026-11-03": true, "2026-11-10": true, "2026-11-17": true, "2026-11-24": true}
	var rows []ShiftRow
	for _, d := range EachDate("2026-11-01", "2026-11-30") {
		if off[d] {
			rows = append(rows, rosterDay(d, nil))
		} else {
			rows = append(rows, rosterDay(d, ptr("SPG1")))
		}
	}
	return rows
}

// R7: jadwal per tanggal (prioritas 0) mengalahkan pola mingguan (prioritas 1)
// yang berlaku sejak tanggal yang sama.
func TestResolveScheduleRowPrefersPerDate(t *testing.T) {
	weekly := ShiftRow{DayOfWeek: 4, ShiftID: ptr("NS"), EffectiveFrom: "2026-11-05", Priority: 1}
	perDate := rosterDay("2026-11-05", nil)
	for _, rows := range [][]ShiftRow{{weekly, perDate}, {perDate, weekly}} {
		got := ResolveScheduleRow(rows, "2026-11-05")
		if got == nil || got.Priority != 0 || got.ShiftID != nil {
			t.Fatalf("ResolveScheduleRow = %+v, want the per-date day off", got)
		}
	}
	// Kamis berikutnya tanpa roster: pola mingguan berlaku.
	if got := ResolveScheduleRow([]ShiftRow{weekly, perDate}, "2026-11-12"); got == nil || got.ShiftID == nil || *got.ShiftID != "NS" {
		t.Fatalf("ResolveScheduleRow(12/11) = %+v, want the weekly NS row", got)
	}
}

func TestScheduledDaysCountsRoster(t *testing.T) {
	days, has := ScheduledDays(novemberRoster(), "2026-11-01", "2026-11-30")
	if !has || days != 26 {
		t.Fatalf("ScheduledDays = %d (has=%v), want 26", days, has)
	}
}

// Pembagi staf toko = hari ber-shift di roster. Roster yang baru dimulai di
// tengah periode tidak diperpanjang ke awal periode: hanya pola mingguan yang
// diukur seolah berlaku sejak awal periode (perbaikan pembagi 22-10).
func TestBuildInputRosterNotExtendedToPeriodStart(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "permanent", IsActive: true, ScheduleType: "shift"}
	var roster []ShiftRow
	for _, r := range novemberRoster() {
		if r.EffectiveFrom >= "2026-11-16" { // roster baru dibuat mulai 16/11
			roster = append(roster, r)
		}
	}
	want, _ := ScheduledDays(roster, "2026-11-01", "2026-11-30") // 16–30 Nov tanpa 17 & 24 = 13
	if want != 13 {
		t.Fatalf("fixture: %d rostered days, want 13", want)
	}
	in := BuildInputRange(emp, Salary{BaseSalary: "2600000.00"}, WorkforceRecords{Schedule: roster}, nil, nil,
		11, 2026, "2026-11-01", "2026-11-30", false)
	if in.WorkingDays != 13 || in.DivisorDays != 13 {
		t.Fatalf("hari kerja = %v, pembagi = %v, want 13 (hari ber-shift di roster)", in.WorkingDays, in.DivisorDays)
	}

	// Pola mingguan yang dimulai di tengah periode tetap diperpanjang, dan roster
	// per tanggal di tanggal awalnya tetap tidak.
	weekly := scheduleFrom("2026-11-16")
	for i := range weekly {
		weekly[i].Priority = 1
	}
	mixed := append(append([]ShiftRow{}, weekly...), rosterDay("2026-11-16", ptr("SPG1")), rosterDay("2026-11-17", nil))
	in = BuildInputRange(emp, Salary{BaseSalary: "2600000.00"}, WorkforceRecords{Schedule: mixed}, nil, nil,
		11, 2026, "2026-11-01", "2026-11-30", false)
	// Senin–Jumat November = 21; Selasa 17/11 diliburkan roster = 20.
	if in.WorkingDays != 20 {
		t.Fatalf("hari kerja campuran = %v, want 20", in.WorkingDays)
	}
}

// Resign: baris yang berakhir tepat pada end_date dianggap berlaku sampai akhir
// periode. Roster per tanggal pada end_date tidak boleh ikut terbuka (menjadi
// pola mingguan tanpa akhir).
func TestBuildInputLeaverRosterOnEndDateStaysOneDay(t *testing.T) {
	end := "2026-11-12" // Kamis
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "resigned", EndDate: &end, ScheduleType: "shift"}
	var roster []ShiftRow
	for _, r := range novemberRoster() {
		if r.EffectiveFrom <= end {
			roster = append(roster, r)
		}
	}
	in := BuildInputRange(emp, Salary{BaseSalary: "2600000.00"}, WorkforceRecords{Schedule: roster}, nil, nil,
		11, 2026, "2026-11-01", "2026-11-30", false)
	// 1–12 Nov tanpa 3 & 10 = 10 hari ber-shift; 13–30 Nov tanpa roster.
	if in.DivisorDays != 10 {
		t.Fatalf("pembagi = %v, want 10 (Kamis 12/11 tidak menjadi pola mingguan)", in.DivisorDays)
	}
}

// R3/R7: staf shift tidak mengenal tanggal merah. Lembur di 25/12 yang
// ber-shift di roster dibayar tarif hari kerja; staf HQ (peta libur R2) di
// bucket libur.
func TestShiftOvertimeOnNationalHolidayIsWeekday(t *testing.T) {
	schedule := []ShiftRow{rosterDay("2026-12-25", ptr("SPG1"))}
	req := []OvertimeRequest{{Date: "2026-12-25", Hours: 2}}
	att := []AttendanceRow{{Date: "2026-12-25", Status: "present", HasClockOut: true}}

	shift := Employee{ScheduleType: "shift"}
	got := SplitOvertimeWith(req, att, shift.DayOffHolidays(map[string]bool{"2026-12-25": true}), schedule, true)
	if got.Regular != 2 || got.Holiday != 0 {
		t.Fatalf("staf shift: %+v, want 2 jam hari kerja", got)
	}
	hq := Employee{ScheduleType: "non_shift"}
	got = SplitOvertimeWith(req, att, hq.DayOffHolidays(map[string]bool{"2026-12-25": true}), schedule, true)
	if got.Regular != 0 || got.Holiday != 2 {
		t.Fatalf("staf HQ: %+v, want 2 jam hari libur", got)
	}
	// Libur roster staf shift tetap rest_day.
	got = SplitOvertimeWith(req, att, nil, []ShiftRow{rosterDay("2026-12-25", nil)}, true)
	if got.Holiday != 2 {
		t.Fatalf("libur roster: %+v, want 2 jam hari libur", got)
	}
}

// attendRoster: hadir di setiap tanggal ber-shift roster.
func attendRoster(rows []ShiftRow) []AttendanceRow {
	var att []AttendanceRow
	for _, r := range rows {
		if r.ShiftID != nil {
			att = append(att, AttendanceRow{Date: r.EffectiveFrom, Status: "present", HasClockOut: true})
		}
	}
	return att
}

func rosterBetween(from, to string) []ShiftRow {
	var rows []ShiftRow
	for _, r := range novemberRoster() {
		if r.EffectiveFrom >= from && r.EffectiveFrom <= to {
			rows = append(rows, r)
		}
	}
	return rows
}

func prorated() Salary {
	return Salary{BaseSalary: "2600000.00", PayBasis: ptr("monthly_prorated")}
}

func factorOf(t *testing.T, in Input) float64 {
	t.Helper()
	if in.ProrateFactor == nil {
		t.Fatal("ProrateFactor nil")
	}
	return *in.ProrateFactor
}

// Karyawan baru 16/11 dengan roster hanya sejak 16/11: tanggal sebelumnya
// tanpa jadwal sama sekali, jadi faktor cakupan memakai hari kalender
// (15/30), sama seperti sebelum payroll membaca roster. Pembagi tetap hari
// ber-shift di roster.
func TestJoinerRosterOnlyProratesOnCalendar(t *testing.T) {
	join := "2026-11-16"
	emp := Employee{ID: "e", JoinDate: join, EmploymentStatus: "contract", IsActive: true, ScheduleType: "shift"}
	roster := rosterBetween(join, "2026-11-30")
	rec := WorkforceRecords{Schedule: roster, Attendance: attendRoster(roster),
		Contracts: []Contract{{ContractType: ptr("pkwt"), StartDate: &join}}}
	in := BuildInputRange(emp, prorated(), rec, nil, nil, 11, 2026, "2026-11-01", "2026-11-30", false)
	if f := factorOf(t, in); f != 0.5 {
		t.Fatalf("factor = %v, want 0.5 (15/30 hari kalender)", f)
	}
	if in.DivisorDays != 13 || in.WorkingDays != 13 {
		t.Fatalf("pembagi = %v, hari kerja = %v, want 13 (roster)", in.DivisorDays, in.WorkingDays)
	}
	eq(t, "pokok 2,6 jt × 15/30", Calculate(in, DefaultConfig()).BaseSalary, 1_300_000)
}

// Resign 12/11: roster sesudah end_date dihapus alur Resign, jadi 13–30 Nov
// tanpa jadwal dan faktor memakai hari kalender (12/30).
func TestLeaverRosterStopsAtEndDateProratesOnCalendar(t *testing.T) {
	end := "2026-11-12"
	emp := Employee{ID: "e", JoinDate: "2020-01-01", EmploymentStatus: "resigned", EndDate: &end, ScheduleType: "shift"}
	roster := rosterBetween("2026-11-01", end)
	rec := WorkforceRecords{Schedule: roster, Attendance: attendRoster(roster)}
	in := BuildInputRange(emp, prorated(), rec, nil, nil, 11, 2026, "2026-11-01", "2026-11-30", false)
	if f := factorOf(t, in); Round(f*1000) != Round(12.0/30*1000) {
		t.Fatalf("factor = %v, want 12/30", f)
	}
	eq(t, "pokok 2,6 jt × 12/30", Calculate(in, DefaultConfig()).BaseSalary, 1_040_000)
}

// Roster sebulan penuh dengan kontrak mulai 16/11: tidak ada tanggal tanpa
// jadwal, jadi faktor tetap hari ber-shift tercakup / hari ber-shift periode.
func TestFullRosterPartialContractProratesOnSchedule(t *testing.T) {
	join := "2026-11-16"
	emp := Employee{ID: "e", JoinDate: join, EmploymentStatus: "contract", IsActive: true, ScheduleType: "shift"}
	roster := novemberRoster()
	rec := WorkforceRecords{Schedule: roster, Attendance: attendRoster(rosterBetween(join, "2026-11-30")),
		Contracts: []Contract{{ContractType: ptr("pkwt"), StartDate: &join}}}
	in := BuildInputRange(emp, prorated(), rec, nil, nil, 11, 2026, "2026-11-01", "2026-11-30", false)
	if f := factorOf(t, in); Round(f*1000) != Round(13.0/26*1000) {
		t.Fatalf("factor = %v, want 13/26 (hari ber-shift)", f)
	}
	if in.DivisorDays != 26 || in.WorkingDays != 13 {
		t.Fatalf("pembagi = %v, hari kerja = %v, want 26 and 13", in.DivisorDays, in.WorkingDays)
	}
	eq(t, "pokok 2,6 jt × 13/26", Calculate(in, DefaultConfig()).BaseSalary, 1_300_000)
}
