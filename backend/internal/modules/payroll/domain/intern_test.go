package domain

import "testing"

func TestIsInternStatus(t *testing.T) {
	for status, want := range map[string]bool{
		"intern": true, "internship": true, "Intern": true, " intern ": true, "INTERNSHIP": true,
		"contract": false, "permanent": false, "probation": false, "": false, "internal": false,
	} {
		if got := IsInternStatus(status); got != want {
			t.Errorf("IsInternStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

// Keputusan user 20-10-2026: Izin Pribadi intern tidak memotong THP; tunjangan
// per hari tetap mengikuti kehadiran. Selisih intern vs kontrak setara hanya PIP.
func TestInternPersonalLeaveSkipsPIP(t *testing.T) {
	c := DefaultConfig()
	c.PersonalLeaveDeduction = true
	// Makan per hari (MealPerDay) dibayar menurut AllowanceDays (hari hadir):
	// 21 hari hadir dari 22 hari kerja, satu hari Izin Pribadi.
	in := func(status string) Input {
		return parityInput(func(in *Input) {
			in.EmploymentStatus = status
			in.BaseSalary, in.TransportAllowance, in.DivisorDays, in.WorkingDays, in.PersonalLeaveDays = 2_600_000, 260_000, 26, 22, 1
			in.MealAllowance, in.MealPerDay, in.AllowanceDays = 520_000, true, 21
		})
	}
	contract := Calculate(in("contract"), c)
	if contract.PersonalLeaveDeduction <= 0 {
		t.Fatalf("kontrak harus tetap kena PIP, got %v", contract.PersonalLeaveDeduction)
	}
	for _, status := range []string{"intern", "internship"} {
		r := Calculate(in(status), c)
		eq(t, status+": PIP", r.PersonalLeaveDeduction, 0)
		eq(t, status+": tunjangan makan per hari sama", r.MealAllowance, contract.MealAllowance)
		eq(t, status+": net hanya beda PIP", r.NetSalary, contract.NetSalary+contract.PersonalLeaveDeduction)
	}
}
