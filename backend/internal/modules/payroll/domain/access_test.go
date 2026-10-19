package domain

import "testing"

func TestResolvePayslipScopeNeedsPayrollMenu(t *testing.T) {
	me := "emp-1"
	other := "emp-2"
	if got := ResolvePayslipScope("hrd", false, &me, &other); !got.MeView || got.EmployeeID != me {
		t.Fatalf("hrd without Penggajian = %+v, want own slips", got)
	}
	if got := ResolvePayslipScope("hrd", true, &me, &other); got.MeView || got.EmployeeID != other {
		t.Fatalf("hrd with Penggajian = %+v, want the requested employee", got)
	}
}

// Non-HR full-view roles (finance_staff) keep their old behaviour.
func TestResolvePayslipScopeKeepsFinanceStaff(t *testing.T) {
	me, other := "emp-1", "emp-2"
	if got := ResolvePayslipScope("finance_staff", false, &me, &other); got.MeView || got.EmployeeID != other {
		t.Fatalf("finance_staff = %+v, want the requested employee", got)
	}
}
