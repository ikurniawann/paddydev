package app

import (
	"context"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/hris"
	"nuhabit/backend/internal/platform/testutil"
)

// The hris adapters run SQL against other contexts' tables; this checks the
// queries against the real schema inside a rolled-back transaction.
func TestHrisAdapters(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	var emp string
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('Adapter Test', 'T-'||md5(random()::text), 'adapter@hris.test', '', '2026-01-01') RETURNING id::text`).Scan(&emp); err != nil {
		t.Fatal(err)
	}

	payroll := hrisPayrollSQL{}
	if p, err := payroll.LatestPaidPayslip(ctx, tx, emp); err != nil || p != nil {
		t.Fatalf("payslip = %v, %v", p, err)
	}
	if s, err := payroll.ActiveLoans(ctx, tx, emp); err != nil || s.Count != 0 || s.TotalRemaining != "0" {
		t.Fatalf("loans = %+v, %v", s, err)
	}
	if _, err := payroll.RecentLoans(ctx, tx, emp); err != nil {
		t.Fatal(err)
	}
	if k, err := payroll.LatestKPISummary(ctx, tx, emp); err != nil || k.Count != 0 || k.AvgScore != nil {
		t.Fatalf("kpi = %+v, %v", k, err)
	}
	if _, err := payroll.PendingLoanCount(ctx, tx); err != nil {
		t.Fatal(err)
	}
	since := time.Now()
	if _, err := payroll.LoanUpdatesSince(ctx, tx, emp, &since); err != nil {
		t.Fatal(err)
	}

	salary := hrisSalarySQL{}
	if err := salary.SyncFromContract(ctx, tx, hris.SalarySync{EmployeeID: emp, BaseSalary: 5_000_000, StartDate: "2026-01-01", ContractNumber: "0001/PKWT/I/2026"}); err != nil {
		t.Fatal(err)
	}
	// A second contract versions the salary, carrying the allowances.
	if err := salary.SyncFromContract(ctx, tx, hris.SalarySync{EmployeeID: emp, BaseSalary: 5_500_000.5, StartDate: "2026-07-01", ContractNumber: "0002/PKWT/VII/2026"}); err != nil {
		t.Fatal(err)
	}
	if base, err := salary.ActiveBaseSalary(ctx, tx, emp); err != nil || base == nil || *base != "5500000.50" {
		t.Fatalf("base salary = %v, %v", base, err)
	}

	if c, err := (hrisRecruitmentSQL{}).PromotedCandidate(ctx, tx, emp); err != nil || c != nil {
		t.Fatalf("candidate = %v, %v", c, err)
	}
	dir := hrisDirectorySQL{}
	if a, err := dir.Account(ctx, tx, "00000000-0000-0000-0000-000000000000"); err != nil || a != nil {
		t.Fatalf("account = %v, %v", a, err)
	}
	if _, err := dir.SuperAdminUserIDs(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if _, err := dir.BrandNames(ctx, tx, []string{"00000000-0000-0000-0000-000000000000"}); err != nil {
		t.Fatal(err)
	}

	wa := &hrisWhatsApp{db: tx}
	id, claimed, err := wa.Claim(ctx, tx, "cuti", "leave:adapter-test", "pesan", []string{"628123"})
	if err != nil || !claimed {
		t.Fatalf("claim = %v, %v", claimed, err)
	}
	if _, again, _ := wa.Claim(ctx, tx, "cuti", "leave:adapter-test", "pesan", nil); again {
		t.Fatal("duplicate claim")
	}
	if err := wa.Release(ctx, tx, id); err != nil {
		t.Fatal(err)
	}
}

// An employee paid by several entities keeps every salary version: the
// contract sync leaves them alone and the base salary is the total.
func TestSalarySyncMultiEntity(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	var emp, holding, coA, coB string
	must(tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('Sinkron', 'SYN-1', 'syn1@test.local', '08', '2024-01-01') RETURNING id::text`).Scan(&emp))
	must(tx.QueryRow(ctx, `INSERT INTO configuration.holdings (name, code) VALUES ('Sinkron Group', 'H-SYN') RETURNING id::text`).Scan(&holding))
	must(tx.QueryRow(ctx, `INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, 'Sinkron A', 'SYN-A') RETURNING id::text`, holding).Scan(&coA))
	must(tx.QueryRow(ctx, `INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, 'Sinkron B', 'SYN-B') RETURNING id::text`, holding).Scan(&coB))
	_, err := tx.Exec(ctx, `INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, effective_date)
		VALUES ($1, $2, 1000000, '2026-01-01'), ($1, $3, 1000000, '2026-01-01')`, emp, coA, coB)
	must(err)

	port := hrisSalarySQL{}
	if base, err := port.ActiveBaseSalary(ctx, tx, emp); err != nil || base == nil || *base != "2000000.00" {
		t.Fatalf("total base = %v %v", base, err)
	}
	// Two entities: the contract sync leaves both versions untouched.
	must(port.SyncFromContract(ctx, tx, hris.SalarySync{EmployeeID: emp, BaseSalary: 5000000, StartDate: "2026-06-01"}))
	var n int
	must(tx.QueryRow(ctx, `SELECT count(*)::int FROM hris.employee_salary WHERE employee_id = $1 AND is_active AND base_salary = 1000000`, emp).Scan(&n))
	if n != 2 {
		t.Fatalf("multi-entity versions after sync = %d, want 2", n)
	}
}

// The ESS "latest payslip" covers the whole newest paid period: one paid run
// per entity sums into a single net salary.
func TestLatestPaidPayslipSumsEntityRuns(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	var emp, holding string
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('Payslip Sum', 'T-'||md5(random()::text), 'payslipsum@hris.test', '', '2026-01-01') RETURNING id::text`).Scan(&emp); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO configuration.holdings (name, code) VALUES ('Payslip Holding', 'PSH-'||substr(md5(random()::text),1,6)) RETURNING id::text`).Scan(&holding); err != nil {
		t.Fatal(err)
	}
	var runs [3]string
	// Newest paid period (2099-12): two entity runs. An older period (2099-11)
	// must not be added in.
	rows := []struct {
		name         string
		month        int
		status       string
		net          float64
		paidInterval string
	}{
		{"Run Des A", 12, "paid", 1000000, "2 hours"},
		{"Run Des B", 12, "paid", 2500000.5, "1 hour"},
		{"Run Nov", 11, "paid", 9000000, "3 hours"},
	}
	for i, r := range rows {
		var company string
		if err := tx.QueryRow(ctx, `INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, $2, $3) RETURNING id::text`,
			holding, "Payslip Co "+r.name, "PS"+r.name).Scan(&company); err != nil {
			t.Fatal(err)
		}
		if err := tx.QueryRow(ctx, `INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status, company_id, paid_at)
			VALUES ($1, $2, 2099, $3, $4, now() - $5::interval) RETURNING id::text`,
			r.name, r.month, r.status, company, r.paidInterval).Scan(&runs[i]); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO hris.payroll_details (payroll_run_id, employee_id, net_salary) VALUES ($1, $2, $3)`,
			runs[i], emp, r.net); err != nil {
			t.Fatal(err)
		}
	}
	p, err := (hrisPayrollSQL{}).LatestPaidPayslip(ctx, tx, emp)
	if err != nil || p == nil {
		t.Fatalf("payslip = %v, %v", p, err)
	}
	if p.PeriodMonth != 12 || p.PeriodYear != 2099 || p.NetSalary != "3500000.50" {
		t.Fatalf("payslip = %+v, want Des 2099 summing both entity runs", p)
	}
	if p.RunName == nil || *p.RunName != "Run Des B" {
		t.Fatalf("run name = %v, want the latest paid run of the period", p.RunName)
	}
}

// PendingLoanCount is HR's queue: manager-approved kasbon and pending loans
// with no manager stage. A pending kasbon waiting for its manager is excluded.
func TestPendingLoanCountFollowsHrStage(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	payroll := hrisPayrollSQL{}
	before, err := payroll.PendingLoanCount(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	var emp, mgr string
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('Count Loan', 'T-'||md5(random()::text), 'cl@hris.test', '', '2026-01-01') RETURNING id::text`).Scan(&emp); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date)
		VALUES ('Count Mgr', 'T-'||md5(random()::text), 'cm@hris.test', '', '2026-01-01') RETURNING id::text`).Scan(&mgr); err != nil {
		t.Fatal(err)
	}
	for _, r := range []struct{ status, manager string }{
		{"manager_approved", mgr},
		{"pending", ""},
		{"pending", mgr},
		{"pending", mgr},
		{"approved", mgr},
	} {
		if _, err := tx.Exec(ctx, `INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months,
			monthly_installment, remaining_balance, status, manager_approver_id)
			VALUES ($1, 'kasbon', 100000, 2, 50000, 100000, $2, NULLIF($3, '')::uuid)`, emp, r.status, r.manager); err != nil {
			t.Fatal(err)
		}
	}
	after, err := payroll.PendingLoanCount(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	if after-before != 2 {
		t.Fatalf("pending delta = %d, want 2", after-before)
	}
}
