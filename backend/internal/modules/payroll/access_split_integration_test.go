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
	own := e.employee("HR Tanpa Penggajian", nonPay.UserID, true)
	worker := e.employee("Slip Uji", "", true)
	e.salary(own, e.co, 6000000)
	e.salary(worker, e.co, 5000000)
	run := e.newRun(6, 2099, e.co)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+run+"/calculate", nil, 200)
	// Personal views list paid runs only.
	for _, st := range []string{"processing", "completed", "paid"} {
		e.call(&e.hr, "PUT", "/api/hris/payroll/"+run, map[string]any{"status": st}, 200)
	}
	slipOf := map[string]string{}
	for _, s := range e.call(&e.hr, "GET", "/api/hris/payslips?payroll_run_id="+run, nil, 200)["data"].([]any) {
		m := s.(map[string]any)
		slipOf[m["employee_id"].(string)] = m["id"].(string)
	}
	if slipOf[own] == "" || slipOf[worker] == "" {
		t.Fatalf("HR with Penggajian should see both slips, got %v", slipOf)
	}
	for _, q := range []string{"?employee_id=" + worker, ""} {
		got := e.call(&nonPay, "GET", "/api/hris/payslips"+q, nil, 200)["data"].([]any)
		if len(got) != 1 || got[0].(map[string]any)["employee_id"] != own {
			t.Fatalf("HR without Penggajian (%q) = %v, want only its own slip", q, got)
		}
	}
	if rec := e.raw(&nonPay, "GET", "/api/hris/payslips/"+slipOf[worker]+"/pdf", nil); rec.Code != 403 {
		t.Fatalf("other's pdf = %d, want 403", rec.Code)
	}
	if rec := e.raw(&nonPay, "GET", "/api/hris/payslips/"+slipOf[own]+"/pdf", nil); rec.Code != 200 {
		t.Fatalf("own pdf = %d, want 200", rec.Code)
	}
}

// The over-limit message must not reveal another employee's base salary
// (30% cap) to an account without Penggajian.
func TestLoanLimitMessageHidesCapWithoutPayrollMenu(t *testing.T) {
	e := setup(t)
	loansHR := hrStaff(e, map[string][]string{"hris.kepegawaian.loans": {"read", "create", "update", "approve"}})
	e.employee("HR Pinjaman", loansHR.UserID, true)
	worker := e.employee("Peminjam", "", true)
	e.salary(worker, e.co, 5000000)
	body := map[string]any{"employee_id": worker, "loan_type": "loan", "principal_amount": 6000000, "tenor_months": 3}
	wantErr(t, e.call(&loansHR, "POST", "/api/hris/loans", body, 400),
		"Cicilan Rp 2.000.000/bulan melebihi batas 30% dari gaji pokok")
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans", body, 400),
		"Cicilan Rp 2.000.000/bulan melebihi batas 30% dari gaji pokok (maks Rp 1.500.000)")
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

// Konfigurasi KPI dan rubric termasuk menu Kinerja: HR tanpa menu itu ditolak.
func TestKPIConfigAndRubricNeedKinerjaMenu(t *testing.T) {
	e := setup(t)
	opsHR := hrStaff(e, map[string][]string{"hris.kepegawaian": nil})
	worker := e.employee("Dinilai", "", true)
	wantErr(t, e.call(&opsHR, "GET", "/api/hris/kpi-config", nil, 403), "Insufficient permissions")
	wantErr(t, e.call(&opsHR, "PUT", "/api/hris/kpi-config", map[string]any{}, 403), "Insufficient permissions")
	wantErr(t, e.call(&opsHR, "POST", "/api/hris/kpi/rubric", map[string]any{
		"employee_id": worker, "period_month": 10, "period_year": 2026, "value": 4}, 403),
		"Hanya HRD atau atasan langsung yang boleh menilai")
	// HR with Kinerja still reads the configuration.
	e.call(&e.hr, "GET", "/api/hris/kpi-config", nil, 200)
}

// Approving re-checks the cap; the approver without Penggajian does not see it.
func TestLoanApprovalHidesCapWithoutPayrollMenu(t *testing.T) {
	e := setup(t)
	loansHR := hrStaff(e, map[string][]string{"hris.kepegawaian.loans": {"read", "create", "update", "approve"}})
	e.employee("HR Pinjaman", loansHR.UserID, true)
	worker := e.employee("Peminjam", e.staff.UserID, true)
	e.salary(worker, e.co, 5000000)
	loan := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{
		"loan_type": "kasbon", "principal_amount": 3000000, "tenor_months": 3}, 200)["data"].(map[string]any)
	e.exec(`UPDATE hris.employee_salary SET base_salary = 2000000 WHERE employee_id = $1`, worker)
	wantErr(t, e.call(&loansHR, "POST", "/api/hris/loans/"+loan["id"].(string)+"/approve", map[string]any{"approved": true}, 400),
		"Cicilan Rp 1.000.000/bulan melebihi batas 30% dari gaji pokok")
}
