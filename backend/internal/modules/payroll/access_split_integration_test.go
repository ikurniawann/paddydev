package payroll

import (
	"context"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// hrStaff is an hrd account holding only the given menus. The rollback is
// registered after CreateStaff so the fixture goes before the account.
func hrStaff(e *env, menus map[string][]string) testutil.Staff {
	e.t.Helper()
	s := testutil.CreateStaff(e.t, testutil.StaffOptions{Role: "hrd", Menus: menus})
	e.t.Cleanup(func() { _ = e.tx.Rollback(context.Background()) })
	return s
}

func TestHRWithoutPayrollMenuSeesOnlyOwnPayslips(t *testing.T) {
	e := setup(t)
	nonPay := hrStaff(e, map[string][]string{"hris.kepegawaian": nil})
	worker := e.employee("Slip Uji", "", true)
	e.salary(worker, e.co, 5000000)
	run := e.newRun(6, 2099, e.co)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+run+"/calculate", nil, 200)

	slips := e.call(&e.hr, "GET", "/api/hris/payslips?payroll_run_id="+run, nil, 200)["data"].([]any)
	if len(slips) == 0 {
		t.Fatal("HR with Penggajian should list the run's slips")
	}
	id := slips[0].(map[string]any)["id"].(string)
	if got := e.call(&nonPay, "GET", "/api/hris/payslips?employee_id="+worker, nil, 200)["data"].([]any); len(got) != 0 {
		t.Fatalf("HR without Penggajian sees %d slips, want 0", len(got))
	}
	if rec := e.raw(&nonPay, "GET", "/api/hris/payslips/"+id+"/pdf", nil); rec.Code != 403 {
		t.Fatalf("pdf = %d, want 403", rec.Code)
	}
}

func TestLoanApprovalFollowsLoansMenu(t *testing.T) {
	e := setup(t)
	loansHR := hrStaff(e, map[string][]string{"hris.kepegawaian.loans": {"read", "create", "update", "approve"}})
	e.employee("HR Pinjaman", loansHR.UserID, true)
	worker := e.employee("Peminjam", e.staff.UserID, true)
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, effective_date) VALUES ($1, $2, 5000000, '2026-01-01')`, worker, e.co)
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{
		"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)
	e.call(&loansHR, "POST", "/api/hris/loans/"+loan["id"].(string)+"/approve", map[string]any{"approved": true}, 200)
}

func TestHRWithoutKinerjaMenuIsNotPerformanceHR(t *testing.T) {
	e := setup(t)
	opsHR := hrStaff(e, map[string][]string{"hris.kepegawaian": nil})
	if got := e.call(&opsHR, "GET", "/api/hris/performance/cycles", nil, 200)["data"].(map[string]any); got["is_hr"] != false {
		t.Fatalf("HR without Kinerja is_hr = %v, want false", got["is_hr"])
	}
	if got := e.call(&e.hr, "GET", "/api/hris/performance/cycles", nil, 200)["data"].(map[string]any); got["is_hr"] != true {
		t.Fatalf("HR with Kinerja is_hr = %v, want true", got["is_hr"])
	}
}
