package domain

import "testing"

func base4(mod func(*Input)) Input {
	return parityInput(func(in *Input) {
		in.EmploymentStatus = "contract"
		in.BaseSalary, in.TransportAllowance, in.MealAllowance, in.PositionAllowance = 2_200_000, 220_000, 220_000, 440_000
		in.DivisorDays, in.WorkingDays, in.PayBasis = 22, 22, "monthly_prorated"
		in.AttendanceDays, in.AllowanceDays, in.PaidDays = 21, 21, 22
		if mod != nil {
			mod(in)
		}
	})
}

func TestThpDayBasisIsFourComponents(t *testing.T) {
	in := base4(func(in *Input) { in.Bonus, in.OvertimeHours = 1_000_000, 5 })
	if got := ThpDayBasis(in); got != 3_080_000 {
		t.Fatalf("ThpDayBasis = %v, want 3.080.000 (pokok+transport+makan+jabatan, tanpa bonus/lembur)", got)
	}
}

// monthly_prorated: gaji pokok & jabatan per hari dibayar (cuti dibayar ikut);
// transport & makan per hari efektif.
func TestProratedUsesPaidDays(t *testing.T) {
	r := Calculate(base4(func(in *Input) { in.TransportPerDay, in.MealPerDay, in.PositionPerDay = true, true, true }), DefaultConfig())
	eq(t, "pokok 22/22", r.BaseSalary, 2_200_000)
	eq(t, "jabatan 22/22", r.PositionAllowance, 440_000)
	eq(t, "transport 21/22", r.TransportAllowance, 210_000)
	eq(t, "makan 21/22", r.MealAllowance, 210_000)
}

// Izin Pribadi 1 hari, transport & makan per_day (sudah tidak dibayar hari itu):
// PIP hanya pokok + jabatan → total hilang = tepat 1 hari 4 komponen (140.000).
func TestPersonalLeaveNoDoubleDeduction(t *testing.T) {
	c := DefaultConfig()
	c.PersonalLeaveDeduction = true
	in := base4(func(in *Input) {
		in.TransportPerDay, in.MealPerDay, in.PositionPerDay = true, true, true
		in.PersonalLeaveDays = 1
	})
	r := Calculate(in, c)
	eq(t, "PIP = (pokok + jabatan)/22", r.PersonalLeaveDeduction, 120_000)
	full := Calculate(base4(func(in *Input) {
		in.TransportPerDay, in.MealPerDay, in.PositionPerDay = true, true, true
		in.AttendanceDays, in.AllowanceDays = 22, 22
	}), c)
	eq(t, "total hilang = 1 hari 4 komponen", full.NetSalary-r.NetSalary, 140_000)

	flat := Calculate(base4(func(in *Input) { in.PersonalLeaveDays = 1 }), c)
	eq(t, "semua flat: PIP = 4 komponen/22", flat.PersonalLeaveDeduction, 140_000)

	intern := Calculate(base4(func(in *Input) { in.EmploymentStatus, in.PersonalLeaveDays = "intern", 1 }), c)
	eq(t, "intern tanpa PIP", intern.PersonalLeaveDeduction, 0)
}

// Sakit & Cuti Melahirkan: potong transport + makan hanya bagian flat.
func TestSickAndMaternityDeductFlatAllowancesOnly(t *testing.T) {
	c := DefaultConfig()
	c.SickLeaveDeduction = true
	flat := Calculate(base4(func(in *Input) { in.SickLeaveDays, in.MaternityLeaveDays = 1, 2 }), c)
	eq(t, "flat: (220k+220k)/22 × 3", flat.SickLeaveDeduction, 60_000)
	perDay := Calculate(base4(func(in *Input) {
		in.TransportPerDay, in.MealPerDay = true, true
		in.SickLeaveDays, in.MaternityLeaveDays = 1, 2
	}), c)
	eq(t, "per_day: sudah tidak dibayar lewat hari efektif", perDay.SickLeaveDeduction, 0)
}

// Telat tier basis thp memakai THP 4 komponen, bukan seluruh slip.
func TestLateTierUsesFourComponentThp(t *testing.T) {
	c := DefaultConfig()
	c.LateMode, c.LateMinMinutes = "tier", 1
	c.LateTiers = []LateTier{{Min: 1, Days: 1, Basis: "thp"}}
	r := Calculate(base4(func(in *Input) {
		in.Bonus = 2_200_000
		in.LateDayMinutes = []float64{30}
	}), c)
	eq(t, "1 hari THP 4 komponen", r.LateDeduction, 140_000)
}
