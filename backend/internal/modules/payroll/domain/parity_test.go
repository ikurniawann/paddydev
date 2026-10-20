package domain

import "testing"

// Rule penggajian Odoo (EPIC-055). Angka harapan dihitung dari rumus Odoo
// hr_salary_rule; default config harus tetap memberi hasil lama.

func parityInput(mod func(*Input)) Input {
	in := Input{
		EmployeeID: "emp-1", PeriodMonth: 9, PeriodYear: 2026,
		BaseSalary: 3_000_000, WorkingDays: 26, JoinDate: "2024-01-15",
		EmploymentStatus: "contract", PTKPStatus: "TK/0", IsTaxable: true,
		BpjsTkEnrolled: true, BpjsKesEnrolled: true,
		PayBasis: "monthly", OvertimeScheme: "standard",
	}
	if mod != nil {
		mod(&in)
	}
	return in
}

func TestPayBasis(t *testing.T) {
	c := DefaultConfig()
	// G1 daily: upah per hari × hari hadir.
	r := Calculate(parityInput(func(in *Input) {
		in.PayBasis, in.BaseSalary, in.AttendanceDays, in.DivisorDays = "daily", 100_000, 20, 26
	}), c)
	eq(t, "daily base", r.BaseSalary, 2_000_000)
	// G8 monthly_prorated: pokok / pembagi × hari hadir.
	r = Calculate(parityInput(func(in *Input) {
		in.PayBasis, in.BaseSalary, in.AttendanceDays, in.DivisorDays = "monthly_prorated", 2_600_000, 13, 26
	}), c)
	eq(t, "prorated base", r.BaseSalary, 1_300_000)
	// monthly: penuh, tidak tergantung hari hadir.
	r = Calculate(parityInput(func(in *Input) { in.AttendanceDays, in.DivisorDays = 3, 26 }), c)
	eq(t, "monthly base", r.BaseSalary, 3_000_000)
	// Pembagi 0: tidak membagi dengan nol.
	r = Calculate(parityInput(func(in *Input) { in.PayBasis, in.AttendanceDays, in.DivisorDays = "monthly_prorated", 10, 0 }), c)
	eq(t, "prorated zero divisor", r.BaseSalary, 0)
}

func TestAllowanceModes(t *testing.T) {
	c := DefaultConfig()
	// G2: per_day = tunjangan / pembagi × hari tunjangan; flat = penuh.
	r := Calculate(parityInput(func(in *Input) {
		in.TransportAllowance, in.MealAllowance, in.PositionAllowance = 500_000, 520_000, 260_000
		in.TransportPerDay, in.PositionPerDay = true, true
		in.AllowanceDays, in.DivisorDays = 20, 26
	}), c)
	eq(t, "transport per day", r.TransportAllowance, 384_615)
	eq(t, "meal flat", r.MealAllowance, 520_000)
	eq(t, "position per day", r.PositionAllowance, 200_000)
	// Tunjangan jabatan & kesehatan kena pajak (masuk bruto), bukan dasar BPJS.
	eq(t, "gross", r.GrossSalary, 3_000_000+384_615+520_000+200_000)
	eq(t, "jht on base only", r.BpjsTkJhtDeduction, 60_000)

	// G7: tunjangan kesehatan hanya bila memenuhi syarat masa kerja.
	r = Calculate(parityInput(func(in *Input) { in.HealthAllowance, in.HealthEligible = 100_000, false }), c)
	eq(t, "health not eligible", r.HealthAllowance, 0)
	r = Calculate(parityInput(func(in *Input) { in.HealthAllowance, in.HealthEligible = 100_000, true }), c)
	eq(t, "health eligible", r.HealthAllowance, 100_000)
}

func TestHealthEligibility(t *testing.T) {
	eligible := func(join string, minDays int) bool {
		return HealthEligible(join, "2026-09-30", minDays)
	}
	if !eligible("2025-10-01", 365) { // 365 hari termasuk tanggal masuk
		t.Error("365 hari harus memenuhi")
	}
	if eligible("2025-10-02", 365) {
		t.Error("364 hari belum memenuhi")
	}
	if !eligible("2026-09-30", 0) || eligible("bukan-tanggal", 365) {
		t.Error("syarat 0 selalu memenuhi; tanggal rusak tidak memenuhi")
	}
}

func TestOvertimeSchemes(t *testing.T) {
	c := DefaultConfig()
	// G4 standard: 1,5× hari biasa, 2× hari istirahat/libur, pokok/173.
	r := Calculate(parityInput(func(in *Input) {
		in.BaseSalary, in.OvertimeHours, in.OvertimeHolidayHrs = 3_460_000, 0, 2
	}), c)
	eq(t, "standard rest day", r.OvertimePay, 80_000)
	// production: tarif × (biasa + 2 × istirahat).
	r = Calculate(parityInput(func(in *Input) {
		in.OvertimeScheme, in.OvertimeHours, in.OvertimeHolidayHrs = "production", 3, 2
	}), c)
	eq(t, "production", r.OvertimePay, 18_000*(3+2*2))
	// freelance: upah harian / 8 × jam istirahat saja.
	r = Calculate(parityInput(func(in *Input) {
		in.OvertimeScheme, in.PayBasis, in.BaseSalary, in.OvertimeHours, in.OvertimeHolidayHrs = "freelance", "daily", 120_000, 5, 4
	}), c)
	eq(t, "freelance", r.OvertimePay, 60_000)
	// daily_worker: upah harian / 5 × total jam; upah harian bulanan = pokok / pembagi.
	r = Calculate(parityInput(func(in *Input) {
		in.OvertimeScheme, in.BaseSalary, in.DivisorDays, in.OvertimeHours, in.OvertimeHolidayHrs = "daily_worker", 2_500_000, 25, 3, 2
	}), c)
	eq(t, "daily worker", r.OvertimePay, 100_000)
}

func TestSplitOvertimeRestDays(t *testing.T) {
	att := func(date string, out bool) AttendanceRow {
		return AttendanceRow{Date: date, Status: "present", HasClockOut: out}
	}
	reqs := []OvertimeRequest{
		{Date: "2026-09-07", Hours: 2},                      // Senin, hari kerja
		{Date: "2026-09-12", Hours: 3, DayType: "rest_day"}, // ditandai hari istirahat
		{Date: "2026-09-13", Hours: 1},                      // Minggu tanpa shift di jadwal
		{Date: "2026-09-08", Hours: 4},                      // tanpa clock-out
	}
	attendance := []AttendanceRow{att("2026-09-07", true), att("2026-09-12", true), att("2026-09-13", true), att("2026-09-08", false)}
	var schedule []ShiftRow
	for dow := 1; dow <= 6; dow++ {
		schedule = append(schedule, ShiftRow{DayOfWeek: dow, ShiftID: ptr("s"), EffectiveFrom: "2026-01-01"})
	}
	s := SplitOvertimeWith(reqs, attendance, nil, schedule, true)
	eq(t, "regular", s.Regular, 2)
	eq(t, "rest", s.Holiday, 4)
	// G9: setting lembur dibayar tanpa clock-out.
	s = SplitOvertimeWith(reqs, attendance, nil, schedule, false)
	eq(t, "regular without clock-out", s.Regular, 6)
	// Tanpa absensi sama sekali tetap dibayar sesuai jam pengajuan bila clock-out tidak wajib.
	s = SplitOvertimeWith([]OvertimeRequest{{Date: "2026-09-09", Hours: 2.5}}, nil, nil, nil, false)
	eq(t, "no attendance", s.Regular, 2.5)
	// Perilaku lama tidak berubah.
	if old := SplitOvertime(reqs, attendance, nil); old.Regular != 3 || old.Holiday != 3 {
		t.Errorf("SplitOvertime lama = %+v", old)
	}
}

func TestLateTiers(t *testing.T) {
	c := DefaultConfig()
	c.LateMode = "tier"
	c.LateMinMinutes = 15
	c.LateTiers = DefaultLateTiers()
	late := func(n int, minutes float64) []float64 {
		out := make([]float64, n)
		for i := range out {
			out[i] = minutes
		}
		return out
	}
	base := func(mod func(*Input)) Input {
		return parityInput(func(in *Input) {
			in.BaseSalary, in.TransportAllowance, in.DivisorDays = 2_600_000, 520_000, 26
			if mod != nil {
				mod(in)
			}
		})
	}
	// 0–3× telat: hanya peringatan.
	eq(t, "3x", Calculate(base(func(in *Input) { in.LateDayMinutes = late(3, 30) }), c).LateDeduction, 0)
	// 4–6×: 1 hari THP (pokok + tunjangan) / pembagi.
	eq(t, "5x", Calculate(base(func(in *Input) { in.LateDayMinutes = late(5, 30) }), c).LateDeduction, 120_000)
	// 7–9×: 2 hari gaji pokok.
	eq(t, "8x", Calculate(base(func(in *Input) { in.LateDayMinutes = late(8, 30) }), c).LateDeduction, 200_000)
	// ≥10×: 3 hari gaji pokok.
	eq(t, "12x", Calculate(base(func(in *Input) { in.LateDayMinutes = late(12, 30) }), c).LateDeduction, 300_000)
	// Telat di bawah batas menit tidak dihitung.
	eq(t, "under threshold", Calculate(base(func(in *Input) { in.LateDayMinutes = append(late(3, 30), late(5, 10)...) }), c).LateDeduction, 0)
	// Input manual LATE_OVERRIDE menggantikan hitungan otomatis.
	r := Calculate(base(func(in *Input) {
		in.LateDayMinutes = late(8, 30)
		in.ManualInputs = []ManualInput{{Code: "LATE_OVERRIDE", Amount: 50_000}}
	}), c)
	eq(t, "override", r.LateDeduction, 50_000)
	// Kebijakan multi-entitas menghapus potongan telat di slip non-utama.
	in := ApplyMultiEntityPolicy(base(func(in *Input) { in.LateDayMinutes = late(8, 30) }), false, PolicyPrimaryEntity)
	eq(t, "non primary", Calculate(in, c).LateDeduction, 0)
}

func TestLeaveDeductions(t *testing.T) {
	c := DefaultConfig()
	in := parityInput(func(in *Input) {
		in.BaseSalary, in.TransportAllowance, in.MealAllowance, in.DivisorDays = 2_600_000, 260_000, 260_000, 26
		in.PersonalLeaveDays, in.SickLeaveDays = 2, 3
	})
	r := Calculate(in, c)
	eq(t, "off by default personal", r.PersonalLeaveDeduction, 0)
	eq(t, "off by default sick", r.SickLeaveDeduction, 0)
	c.PersonalLeaveDeduction, c.SickLeaveDeduction = true, true
	r = Calculate(in, c)
	// PIP: THP / pembagi × hari = 3.120.000 / 26 × 2.
	eq(t, "personal", r.PersonalLeaveDeduction, 240_000)
	// PIS: (transport + makan) / pembagi × hari = 520.000 / 26 × 3.
	eq(t, "sick", r.SickLeaveDeduction, 60_000)
	eq(t, "net", r.NetSalary, r.GrossSalary-r.TotalDeductions)
}

// EPIC-058: Izin Leader tidak memotong apa pun; Izin Pribadi satu hari
// memotong THP / pembagi × 1 (rule PIP EPIC-055, aktif lewat setting).
func TestLeaderAndPersonalLeavePayroll(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2025-01-01", EmploymentStatus: "permanent"}
	sal := Salary{BaseSalary: "2600000.00", WorkingDaysDivisor: "26.00"}
	rec := WorkforceRecords{Leaves: []Leave{
		{ptr("2026-09-08"), ptr("2026-09-08"), "leader"},
		{ptr("2026-09-09"), ptr("2026-09-09"), "personal"},
		{ptr("2026-09-10"), ptr("2026-09-10"), "marriage"},
	}}
	in := BuildInput(emp, sal, rec, nil, nil, 9, 2026, false)
	if in.PersonalLeaveDays != 1 || in.UnpaidLeaveDays != 0 || in.SickLeaveDays != 0 {
		t.Fatalf("leave days personal=%v unpaid=%v sick=%v", in.PersonalLeaveDays, in.UnpaidLeaveDays, in.SickLeaveDays)
	}

	c := DefaultConfig()
	c.PersonalLeaveDeduction = true
	// THP = 2.600.000 + transport flat 260.000 = 2.860.000; / pembagi 26 × 1 hari = 110.000.
	r := Calculate(parityInput(func(in *Input) {
		in.BaseSalary, in.TransportAllowance, in.DivisorDays, in.WorkingDays, in.PersonalLeaveDays = 2_600_000, 260_000, 26, 22, 1
	}), c)
	eq(t, "PIP satu hari = THP/pembagi", r.PersonalLeaveDeduction, 110_000)

	// Izin Leader + marriage saja: tidak ada potongan PIP, unpaid, maupun PIS,
	// meski ketiga setting potongan aktif.
	c.SickLeaveDeduction = true
	onlyNoDeduct := BuildInput(emp, sal, WorkforceRecords{Leaves: []Leave{
		{ptr("2026-09-08"), ptr("2026-09-08"), "leader"},
		{ptr("2026-09-10"), ptr("2026-09-10"), "marriage"},
	}}, nil, nil, 9, 2026, false)
	r = Calculate(onlyNoDeduct, c)
	eq(t, "leader/marriage: PIP", r.PersonalLeaveDeduction, 0)
	eq(t, "leader/marriage: unpaid", r.UnpaidLeaveDeduction, 0)
	eq(t, "leader/marriage: PIS", r.SickLeaveDeduction, 0)
}

func TestManualInputs(t *testing.T) {
	c := DefaultConfig()
	r := Calculate(parityInput(func(in *Input) {
		in.ManualInputs = []ManualInput{
			{Code: "BONUS", Amount: 500_000}, {Code: "INCENTIVE", Amount: 200_000},
			{Code: "OTHER_EARNING_NONTAX", Amount: 75_000},
			{Code: "PENALTY", Amount: 25_000}, {Code: "OTHER_DEDUCTION", Amount: 10_000},
		}
	}), c)
	eq(t, "bonus", r.Bonus, 500_000)
	eq(t, "other earning", r.OtherEarning, 200_000)
	eq(t, "nontax", r.OtherEarningNontax, 75_000)
	eq(t, "gross taxable", r.GrossSalary, 3_000_000+500_000+200_000)
	eq(t, "other deduction", r.OtherDeduction, 35_000)
	eq(t, "net includes nontax", r.NetSalary, r.GrossSalary+75_000-r.TotalDeductions)
}

func TestTERRates(t *testing.T) {
	cases := []struct {
		ptkp string
		dpp  float64
		rate float64
	}{
		{"TK/0", 5_400_000, 0}, {"TK/0", 5_400_001, 0.0025}, {"TK/1", 10_050_000, 0.02}, {"K/0", 10_050_001, 0.0225},
		{"TK/0", 26_450_001, 0.11}, {"TK/0", 1_400_000_001, 0.34},
		{"TK/2", 6_200_000, 0}, {"K/1", 9_200_000, 0.01}, {"K/2", 9_200_001, 0.015}, {"TK/3", 1_405_000_001, 0.34},
		{"K/3", 6_600_000, 0}, {"K/3", 10_950_001, 0.0175}, {"K/3", 12_050_001, 0.03}, {"K/3", 1_419_000_001, 0.34},
		{"X/9", 10_000_000, 0.02}, // status tak dikenal = kategori A
	}
	for _, tc := range cases {
		if got := TERRate(TERCategory(tc.ptkp), tc.dpp); got != tc.rate {
			t.Errorf("TER %s %v = %v, want %v", tc.ptkp, tc.dpp, got, tc.rate)
		}
	}
	if len(terTableA) != 44 || len(terTableB) != 40 || len(terTableC) != 41 {
		t.Fatalf("jumlah lapisan TER %d/%d/%d, PP 58/2023: 44/40/41", len(terTableA), len(terTableB), len(terTableC))
	}
}

func TestPPh21TER(t *testing.T) {
	c := DefaultConfig()
	c.Pph21Method = "ter"
	c.Pph21BpjsBasis = "employer"
	in := parityInput(func(in *Input) { in.BaseSalary, in.BpjsTkEnrolled, in.BpjsKesEnrolled = 10_000_000, false, false })
	r := Calculate(in, c)
	// DPP 10.000.000 (tanpa BPJS), TK/0 → kategori A 2%.
	eq(t, "dpp", r.Pph21Dpp, 10_000_000)
	eq(t, "ter pph", r.Pph21Deduction, 200_000)
	if r.TERCategory != "A" || r.TERRate != 0.02 || r.Pph21Method != "ter" {
		t.Errorf("audit %q %v %q", r.TERCategory, r.TERRate, r.Pph21Method)
	}
	// Basis both: iuran BPJS TK & Kes perusahaan + karyawan masuk DPP (seperti Odoo).
	c.Pph21BpjsBasis = "both"
	r = Calculate(parityInput(func(in *Input) { in.BaseSalary = 5_000_000 }), c)
	// JHT 100k+185k, JP 50k+100k, JKK 12k, JKM 15k, Kes 50k+200k = 712.000.
	eq(t, "dpp both", r.Pph21Dpp, 5_712_000)
	eq(t, "ter both", r.Pph21Deduction, Round(5_712_000*0.005))
	c.Pph21BpjsBasis = "employee"
	eq(t, "dpp employee", Calculate(parityInput(func(in *Input) { in.BaseSalary = 5_000_000 }), c).Pph21Dpp, 5_200_000)
	// Tidak kena pajak.
	eq(t, "not taxable", Calculate(parityInput(func(in *Input) { in.BaseSalary, in.IsTaxable = 20_000_000, false }), c).Pph21Deduction, 0)
}

func TestPPh21TERDecember(t *testing.T) {
	c := DefaultConfig()
	c.Pph21Method, c.Pph21BpjsBasis = "ter", "employer"
	// Bruto 10 jt/bulan tanpa BPJS. Jan–Nov dipotong TER 2% = 200.000 × 11.
	in := parityInput(func(in *Input) {
		in.PeriodMonth, in.BaseSalary, in.BpjsTkEnrolled, in.BpjsKesEnrolled = 12, 10_000_000, false, false
		in.Ytd = YearToDate{Months: 11, Bruto: 110_000_000, Pph21: 2_200_000}
	})
	r := Calculate(in, c)
	// Setahun: bruto 120 jt − biaya jabatan 6 jt = 114 jt − PTKP 54 jt = PKP 60 jt → 5% = 3.000.000.
	eq(t, "annual", r.Pph21Annual, 3_000_000)
	eq(t, "december", r.Pph21Deduction, 800_000)
	// Lebih bayar tidak menjadi potongan negatif.
	in.Ytd.Pph21 = 3_500_000
	eq(t, "overpaid", Calculate(in, c).Pph21Deduction, 0)
}

func TestDefaultsUnchanged(t *testing.T) {
	// Field baru kosong & config default: hasil sama dengan referensi lama.
	c := DefaultConfig()
	r := Calculate(baseInput(nil), c)
	eq(t, "net", r.NetSalary, 10_861_155)
	eq(t, "pph", r.Pph21Deduction, 446_075)
	if r.Pph21Method != "annualized" {
		t.Errorf("method %q", r.Pph21Method)
	}
}

func TestBuildInputParity(t *testing.T) {
	emp := Employee{ID: "e", JoinDate: "2025-06-01", EmploymentStatus: "contract"}
	sal := Salary{BaseSalary: "100000.00", PayBasis: ptr("daily"), WorkingDaysDivisor: "26.00",
		TransportMode: ptr("per_day"), HealthAllowance: "100000.00", HealthMinServiceDays: ptr(365), OvertimeScheme: ptr("freelance")}
	att := func(date, status string, late bool, minutes string) AttendanceRow {
		return AttendanceRow{Date: date, Status: status, HasClockOut: true, IsLate: ptr(late), LateMinutes: minutes}
	}
	rec := WorkforceRecords{
		Attendance: []AttendanceRow{att("2026-09-01", "present", false, "0"), att("2026-09-02", "late", true, "20"),
			att("2026-09-03", "absent", false, "0"), att("2026-09-04", "late", true, "5")},
		Leaves: []Leave{{ptr("2026-09-10"), ptr("2026-09-11"), "personal"}, {ptr("2026-09-14"), ptr("2026-09-14"), "sick"},
			{ptr("2026-09-29"), ptr("2026-10-02"), "sick"}},
	}
	in := BuildInput(emp, sal, rec, nil, nil, 9, 2026, false)
	if in.PayBasis != "daily" || in.DivisorDays != 26 || in.AttendanceDays != 3 || in.AllowanceDays != 3 {
		t.Fatalf("days %+v", in)
	}
	if !in.TransportPerDay || in.MealPerDay || in.OvertimeScheme != "freelance" || !in.HealthEligible || in.HealthAllowance != 100_000 {
		t.Fatalf("modes %+v", in)
	}
	if len(in.LateDayMinutes) != 2 || in.LateDayMinutes[0]+in.LateDayMinutes[1] != 25 {
		t.Fatalf("late %v", in.LateDayMinutes)
	}
	if in.PersonalLeaveDays != 2 || in.SickLeaveDays != 3 {
		t.Fatalf("leave %v %v", in.PersonalLeaveDays, in.SickLeaveDays)
	}
	// Upah harian tanpa pembagi struktur (Odoo: struktur kosong, base_days 0):
	// aturan per hari tidak berlaku, karena hari tidak hadir memang tidak dibayar.
	sal.WorkingDaysDivisor = nil
	if in = BuildInput(emp, sal, rec, nil, nil, 9, 2026, false); in.DivisorDays != 0 {
		t.Fatalf("daily without divisor %v", in.DivisorDays)
	}
	// Bulanan tanpa pembagi di struktur gaji: hari kerja hasil hitung.
	sal.PayBasis = ptr("monthly")
	if in = BuildInput(emp, sal, rec, nil, nil, 9, 2026, false); in.DivisorDays != in.WorkingDays {
		t.Fatalf("divisor fallback %v %v", in.DivisorDays, in.WorkingDays)
	}
	// Struktur gaji lama (tanpa field baru): default perilaku lama.
	in = BuildInput(emp, Salary{BaseSalary: "1000000"}, rec, nil, nil, 9, 2026, false)
	if in.PayBasis != "monthly" || in.OvertimeScheme != "standard" || in.TransportPerDay {
		t.Fatalf("legacy %+v", in)
	}
}

// Temuan review EPIC-055.
func TestReviewFixes(t *testing.T) {
	c := DefaultConfig()
	// #3 upah harian/prorata: cuti tanpa upah tidak dipotong lagi.
	r := Calculate(parityInput(func(in *Input) {
		in.PayBasis, in.BaseSalary, in.AttendanceDays, in.DivisorDays, in.WorkingDays, in.UnpaidLeaveDays = "daily", 100_000, 18, 20, 20, 2
	}), c)
	eq(t, "daily unpaid", r.UnpaidLeaveDeduction, 0)
	eq(t, "daily base", r.BaseSalary, 1_800_000)
	eq(t, "monthly unpaid still", Calculate(parityInput(func(in *Input) { in.WorkingDays, in.UnpaidLeaveDays = 20, 2 }), c).UnpaidLeaveDeduction, 300_000)

	// #5 LATE_OVERRIDE 0 menghapus potongan telat otomatis.
	ct := c
	ct.LateMode, ct.LateTiers = "tier", DefaultLateTiers()
	r = Calculate(parityInput(func(in *Input) {
		in.DivisorDays, in.LateDayMinutes = 26, []float64{30, 30, 30, 30, 30, 30, 30, 30}
		in.ManualInputs = []ManualInput{{Code: "LATE_OVERRIDE", Amount: 0}}
	}), ct)
	eq(t, "late waived", r.LateDeduction, 0)

	// #7 basis employer = standar DJP: JKK + JKM + Kes perusahaan saja.
	ce := c
	ce.Pph21Method, ce.Pph21BpjsBasis = "ter", "employer"
	r = Calculate(parityInput(func(in *Input) { in.BaseSalary = 5_000_000 }), ce)
	eq(t, "dpp employer", r.Pph21Dpp, 5_000_000+12_000+15_000+200_000)

	// #8 total bruto run memuat penghasilan tidak kena pajak (bruto = bersih + potongan).
	r = Calculate(parityInput(func(in *Input) { in.ManualInputs = []ManualInput{{Code: "OTHER_EARNING_NONTAX", Amount: 75_000}} }), c)
	tot := SumRunTotals([]Result{r})
	eq(t, "run gross", tot.TotalGross, tot.TotalNet+tot.TotalDeductions)
}

func TestRestDayFromSchedule(t *testing.T) {
	var schedule []ShiftRow
	for dow := 1; dow <= 5; dow++ {
		schedule = append(schedule, ShiftRow{DayOfWeek: dow, ShiftID: ptr("s"), EffectiveFrom: "2026-09-10"})
	}
	att := []AttendanceRow{{Date: "2026-09-05", HasClockOut: true}, {Date: "2026-09-12", HasClockOut: true}}
	reqs := []OvertimeRequest{{Date: "2026-09-05", Hours: 2}, {Date: "2026-09-12", Hours: 3}} // dua-duanya Sabtu
	// Sabtu sebelum jadwal berlaku (5 Sep) bukan hari istirahat; Sabtu 12 Sep iya.
	s := SplitOvertimeWith(reqs, att, nil, schedule, true)
	eq(t, "before schedule regular", s.Regular, 2)
	eq(t, "rest after schedule", s.Holiday, 3)

	emp := Employee{ID: "e", JoinDate: "2024-01-01", EmploymentStatus: "contract"}
	rec := WorkforceRecords{Attendance: att, Overtime: reqs, Schedule: schedule}
	// Default: jadwal tidak menentukan hari istirahat (perilaku lama).
	in := BuildInput(emp, Salary{BaseSalary: "3460000"}, rec, nil, nil, 9, 2026, false)
	eq(t, "default regular", in.OvertimeHours, 5)
	in = BuildInputRange(emp, Salary{BaseSalary: "3460000"}, rec, nil, nil, 9, 2026, "2026-09-01", "2026-09-30", false,
		BuildOptions{OvertimeRequireClockout: true, OvertimeRestFromSchedule: true})
	eq(t, "setting on rest", in.OvertimeHolidayHrs, 3)
}
