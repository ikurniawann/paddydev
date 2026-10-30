package payroll

import (
	"os"
	"strings"
	"testing"

	"nuhabit/backend/internal/modules/payroll/domain"
)

func TestPayrollEntities(t *testing.T) {
	e := setup(t)
	e.entity("Pusat Uji")
	if rec := e.raw(&e.staff, "GET", "/api/hris/payroll/entities", nil); rec.Code != 403 {
		t.Fatalf("non-compensation = %d", rec.Code)
	}
	list := e.call(&e.hr, "GET", "/api/hris/payroll/entities", nil, 200)["data"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["name"] != "Entitas Uji" ||
		list[1].(map[string]any)["holding_name"] != "Pusat Uji Group" {
		t.Fatalf("entities %v", list)
	}
}

func TestRunPerEntityCreate(t *testing.T) {
	e := setup(t)
	e.employee("HR Payroll", e.hr.UserID, true)
	pusat, _ := e.entity("Pusat Uji")

	wantErr(t, e.call(&e.hr, "POST", "/api/hris/payroll", map[string]any{"period_month": 6, "period_year": 2099}, 400),
		"Entitas wajib dipilih")
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/payroll",
		map[string]any{"period_month": 6, "period_year": 2099, "company_id": "00000000-0000-0000-0000-000000000000"}, 400),
		"Entitas tidak ditemukan atau tidak aktif")

	runA := e.newRun(6, 2099, e.co)
	e.newRun(6, 2099, pusat) // another entity, same period: allowed
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/payroll",
		map[string]any{"period_month": 6, "period_year": 2099, "company_id": e.co}, 400), "Payroll untuk periode ini sudah ada")

	got := e.call(&e.hr, "GET", "/api/hris/payroll/"+runA, nil, 200)["data"].(map[string]any)
	if got["run_name"] != "Payroll Juni 2099" || got["company"].(map[string]any)["name"] != "Entitas Uji" {
		t.Fatalf("run %v", got)
	}
	rows := e.call(&e.hr, "GET", "/api/hris/payroll?year=2099", nil, 200)["data"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["company"] == nil {
		t.Fatalf("list %v", rows)
	}

	// A legacy run without entity blocks entity runs of its period and
	// cannot be recalculated.
	var legacy string
	e.scalar(&legacy, `INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status)
		VALUES ('Payroll Juli 2099', 7, 2099, 'draft') RETURNING id::text`)
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/payroll",
		map[string]any{"period_month": 7, "period_year": 2099, "company_id": e.co}, 400),
		"Periode ini masih punya run gabungan sebelum payroll per entitas (Payroll Juli 2099) — hapus run itu dulu untuk memproses per entitas")
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/payroll/"+legacy+"/calculate", nil, 400),
		"Run gabungan sebelum payroll per entitas tidak bisa dihitung ulang — hapus lalu buat payroll per entitas")
	if c := e.call(&e.hr, "GET", "/api/hris/payroll/"+legacy, nil, 200)["data"].(map[string]any)["company"]; c != nil {
		t.Fatalf("legacy company %v", c)
	}
}

// A legacy salary with no entity yet is superseded once HR assigns one, so
// totals are not double counted; it still counts for the loan limit until then.
func TestSalaryNullCompanySuperseded(t *testing.T) {
	e := setup(t)
	emp := e.employee("Tanpa Entitas", "", true)
	e.exec(`INSERT INTO hris.employee_salary (employee_id, base_salary, effective_date) VALUES ($1, 5000000, '2026-01-01')`, emp)
	// Only a NULL-company salary: the loan limit still sees it (30% of 5.000.000).
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans", map[string]any{"employee_id": emp, "loan_type": "loan",
		"principal_amount": 6000000, "tenor_months": 3}, 400),
		"Cicilan Rp 2.000.000/bulan melebihi batas 30% dari gaji pokok (maks Rp 1.500.000)")

	e.call(&e.hr, "POST", "/api/hris/employee-salary",
		map[string]any{"employee_id": emp, "base_salary": 6_000_000, "effective_date": "2026-03-01", "company_id": e.co}, 200)
	var active int
	var company string
	e.scalar(&active, `SELECT count(*)::int FROM hris.employee_salary WHERE employee_id = $1 AND is_active`, emp)
	e.scalar(&company, `SELECT COALESCE(company_id::text, '') FROM hris.employee_salary WHERE employee_id = $1 AND is_active`, emp)
	if active != 1 || company != e.co {
		t.Fatalf("active rows = %d (company %q), want only the %s one", active, company, e.co)
	}
}

func TestSalaryPerEntity(t *testing.T) {
	e := setup(t)
	pusat, _ := e.entity("Pusat Uji")
	emp := e.employee("Dua Entitas", "", true)
	post := func(company string, base int, status int) map[string]any {
		body := map[string]any{"employee_id": emp, "base_salary": base, "effective_date": "2026-01-01"}
		if company != "" {
			body["company_id"] = company
		}
		return e.call(&e.hr, "POST", "/api/hris/employee-salary", body, status)
	}
	wantErr(t, post("", 1_000_000, 400), "Entitas wajib dipilih")
	post(e.co, 1_000_000, 200)
	post(pusat, 2_000_000, 200)
	// A new version of one entity deactivates only that entity's version.
	e.call(&e.hr, "POST", "/api/hris/employee-salary",
		map[string]any{"employee_id": emp, "base_salary": 1_500_000, "effective_date": "2026-03-01", "company_id": e.co}, 200)

	var active int
	e.scalar(&active, `SELECT count(*)::int FROM hris.employee_salary WHERE employee_id = $1 AND is_active`, emp)
	if active != 2 {
		t.Fatalf("active versions = %d, want 2 (one per entity)", active)
	}
	rows := e.call(&e.hr, "GET", "/api/hris/employee-salary?employee_id="+emp, nil, 200)["data"].([]any)
	names := map[string]bool{}
	for _, r := range rows {
		if c, ok := r.(map[string]any)["company"].(map[string]any); ok {
			names[c["name"].(string)] = true
		}
	}
	if !names["Entitas Uji"] || !names["Pusat Uji"] {
		t.Fatalf("salary rows company embed %v", rows)
	}
}

func TestMultiEntityCalculate(t *testing.T) {
	e := setup(t)
	e.employee("HR Payroll", e.hr.UserID, true)
	coA, storeA := e.co, e.store
	coB, storeB := e.entity("Entitas B")
	shared := e.employee("Dua Entitas", "", true)
	refA := e.employee("Hanya A", "", true)
	refB := e.employee("Hanya B", "", true)
	// Only an active salary without a paying entity: excluded from every
	// entity run, with a reason that tells HR what to fix.
	noEntity := e.employee("Tanpa Entitas", "", true)
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, ptkp_status, effective_date)
		VALUES ($1, NULL, 4000000, 'TK/0', '2026-01-01')`, noEntity)
	e.salary(shared, coA, 1_000_000)
	e.salary(shared, coB, 9_000_000)
	e.salary(refA, coA, 1_000_000)
	e.salary(refB, coB, 9_000_000)
	e.exec(`INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months, monthly_installment,
		remaining_balance, first_installment_month, first_installment_year, status, approved_at)
		VALUES ($1, 'kasbon', 300000, 3, 100000, 300000, 6, 2099, 'approved', now())`, shared)
	// Placed at A's store: A is the primary entity (B pays more).
	e.work.placements = map[string][]domain.Placement{
		shared: {{BranchID: storeA, IsPrimary: true, Start: "2026-01-01"}},
		refA:   {{BranchID: storeA, IsPrimary: true, Start: "2026-01-01"}},
		refB:   {{BranchID: storeB, IsPrimary: true, Start: "2026-01-01"}},
	}

	runA, runB := e.newRun(6, 2099, coA), e.newRun(6, 2099, coB)
	sumA := e.call(&e.hr, "POST", "/api/hris/payroll/"+runA+"/calculate", nil, 200)["summary"].(map[string]any)
	sumB := e.call(&e.hr, "POST", "/api/hris/payroll/"+runB+"/calculate", nil, 200)["summary"].(map[string]any)
	if sumA["total_employees"].(float64) != 2 || sumB["total_employees"].(float64) != 2 {
		t.Fatalf("A %v / B %v", sumA, sumB)
	}
	if sumA["other_entity_count"].(float64) != 1 { // Hanya B
		t.Fatalf("A other entity %v", sumA["other_entity_count"])
	}
	if sumB["other_entity_count"].(float64) != 1 { // Hanya A
		t.Fatalf("B other entity %v", sumB["other_entity_count"])
	}
	hrSkipped := false
	for _, sk := range sumA["skipped"].([]any) {
		m := sk.(map[string]any)
		if m["full_name"] == "HR Payroll" && m["reason"] == "Belum ada struktur gaji aktif" {
			hrSkipped = true
		}
	}
	if !hrSkipped {
		t.Fatalf("HR Payroll not skipped in A: %v", sumA["skipped"])
	}
	skipReason := func(sum map[string]any, name string) string {
		for _, sk := range sum["skipped"].([]any) {
			if m := sk.(map[string]any); m["full_name"] == name {
				return m["reason"].(string)
			}
		}
		return ""
	}
	const noEntityReason = "Struktur gaji belum punya entitas — buat versi dengan entitas pembayar"
	for name, sum := range map[string]map[string]any{"A": sumA, "B": sumB} {
		if got := skipReason(sum, "Tanpa Entitas"); got != noEntityReason {
			t.Fatalf("run %s skip reason for the NULL-company salary = %q, want %q", name, got, noEntityReason)
		}
	}

	type slip struct{ gross, pph, bpjsKes, bpjsTk, loan string }
	read := func(runID, emp string) slip {
		var s slip
		if err := e.tx.QueryRow(e.ctx, `SELECT gross_salary::text, pph21_deduction::text, bpjs_kes_deduction::text,
			bpjs_tk_jht_deduction::text, loan_deduction::text FROM hris.payroll_details
			WHERE payroll_run_id = $1 AND employee_id = $2`, runID, emp).Scan(&s.gross, &s.pph, &s.bpjsKes, &s.bpjsTk, &s.loan); err != nil {
			t.Fatalf("slip %s/%s: %v", runID, emp, err)
		}
		return s
	}
	a, b := read(runA, shared), read(runB, shared)
	ra, rb := read(runA, refA), read(runB, refB)
	// K4: each slip's tax/BPJS equals that of an employee paid only by that entity.
	if a.gross != ra.gross || a.pph != ra.pph || a.bpjsKes != ra.bpjsKes || a.bpjsTk != ra.bpjsTk {
		t.Fatalf("A slip %+v vs reference %+v", a, ra)
	}
	if b.gross != rb.gross || b.pph != rb.pph || b.bpjsKes != rb.bpjsKes || b.bpjsTk != rb.bpjsTk {
		t.Fatalf("B slip %+v vs reference %+v", b, rb)
	}
	// K5 (interim policy): the installment is on the primary entity's slip only.
	if a.loan != "100000.00" || b.loan != "0.00" {
		t.Fatalf("loan A %s B %s", a.loan, b.loan)
	}
	// B's slip is never allocated to A's store.
	var leaked int
	e.scalar(&leaked, `SELECT count(*)::int FROM hris.payroll_detail_allocations
		WHERE payroll_run_id = $1 AND branch_id = $2`, runB, storeA)
	if leaked != 0 {
		t.Fatalf("%d allocations of run B at A's store", leaked)
	}
}

// A once-per-period item (loan installment, late deduction) already on a slip
// of another entity's run of the same period is never repeated, even when the
// primary entity changed between the two calculations.
func TestOnceItemsNotRepeatedAcrossRuns(t *testing.T) {
	e := setup(t)
	e.employee("HR Payroll", e.hr.UserID, true)
	coA, storeA := e.co, e.store
	coB, storeB := e.entity("Entitas B")
	shared := e.employee("Dua Entitas", "", true)
	e.salary(shared, coA, 1_000_000)
	e.salary(shared, coB, 9_000_000)
	e.exec(`INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months, monthly_installment,
		remaining_balance, first_installment_month, first_installment_year, status, approved_at)
		VALUES ($1, 'kasbon', 300000, 3, 100000, 300000, 6, 2099, 'approved', now())`, shared)
	e.work.placements = map[string][]domain.Placement{
		shared: {{BranchID: storeA, IsPrimary: true, Start: "2026-01-01"}},
	}
	runA, runB := e.newRun(6, 2099, coA), e.newRun(6, 2099, coB)
	loan := func(runID string) string {
		var v string
		e.scalar(&v, `SELECT loan_deduction::text FROM hris.payroll_details WHERE payroll_run_id = $1 AND employee_id = $2`, runID, shared)
		return v
	}

	e.call(&e.hr, "POST", "/api/hris/payroll/"+runA+"/calculate", nil, 200)
	if got := loan(runA); got != "100000.00" {
		t.Fatalf("A loan %s", got)
	}
	// The employee moves to B's store: B is now the primary entity, but the
	// installment is already on A's slip.
	e.work.placements[shared] = []domain.Placement{{BranchID: storeB, IsPrimary: true, Start: "2026-01-01"}}
	e.call(&e.hr, "POST", "/api/hris/payroll/"+runB+"/calculate", nil, 200)
	if a, b := loan(runA), loan(runB); a != "100000.00" || b != "0.00" {
		t.Fatalf("loan A %s B %s", a, b)
	}
	// Recalculating A drops its own old details first; A is no longer the
	// primary entity, so the installment is on neither slip (never twice)...
	e.call(&e.hr, "POST", "/api/hris/payroll/"+runA+"/calculate", nil, 200)
	if a, b := loan(runA), loan(runB); a != "0.00" || b != "0.00" {
		t.Fatalf("after recalc A: loan A %s B %s", a, b)
	}
	// ...and recalculating the new primary entity picks it up again.
	e.call(&e.hr, "POST", "/api/hris/payroll/"+runB+"/calculate", nil, 200)
	if a, b := loan(runA), loan(runB); a != "0.00" || b != "100000.00" {
		t.Fatalf("after recalc B: loan A %s B %s", a, b)
	}
}

// The late deduction is also once per period: charged on one entity's slip,
// never repeated on the other's, however the primary entity moves.
func TestLateDeductionNotRepeatedAcrossRuns(t *testing.T) {
	e := setup(t)
	e.employee("HR Payroll", e.hr.UserID, true)
	coA, storeA := e.co, e.store
	coB, storeB := e.entity("Entitas B")
	shared := e.employee("Dua Entitas", "", true)
	e.salary(shared, coA, 1_000_000)
	e.salary(shared, coB, 9_000_000)
	// A flat deduction per late day; the test database has no settings row yet.
	e.call(&e.hr, "PUT", "/api/hris/payroll-settings", map[string]any{
		"settings": map[string]any{"late_deduction_mode": "flat", "late_deduction_amount": 50_000}}, 200)
	late := true
	minutes := "15"
	e.work.records[shared] = domain.WorkforceRecords{Attendance: []domain.AttendanceRow{
		{Date: "2099-06-10", Status: "late", IsLate: &late, LateMinutes: minutes}}}
	e.work.placements = map[string][]domain.Placement{
		shared: {{BranchID: storeA, IsPrimary: true, Start: "2026-01-01"}},
	}
	runA, runB := e.newRun(6, 2099, coA), e.newRun(6, 2099, coB)
	lateOf := func(runID string) string {
		var v string
		e.scalar(&v, `SELECT late_deduction::text FROM hris.payroll_details WHERE payroll_run_id = $1 AND employee_id = $2`, runID, shared)
		return v
	}

	e.call(&e.hr, "POST", "/api/hris/payroll/"+runA+"/calculate", nil, 200)
	if got := lateOf(runA); got != "50000.00" {
		t.Fatalf("A late deduction %s", got)
	}
	// The employee moves to B's store: B is now the primary entity, but the
	// late deduction is already on A's slip.
	e.work.placements[shared] = []domain.Placement{{BranchID: storeB, IsPrimary: true, Start: "2026-01-01"}}
	e.call(&e.hr, "POST", "/api/hris/payroll/"+runB+"/calculate", nil, 200)
	if a, b := lateOf(runA), lateOf(runB); a != "50000.00" || b != "0.00" {
		t.Fatalf("late A %s B %s", a, b)
	}
	// Recalculating A drops its own old details first and A is no longer
	// primary: the deduction is on neither slip (never twice)...
	e.call(&e.hr, "POST", "/api/hris/payroll/"+runA+"/calculate", nil, 200)
	if a, b := lateOf(runA), lateOf(runB); a != "0.00" || b != "0.00" {
		t.Fatalf("after recalc A: late A %s B %s", a, b)
	}
	// ...and recalculating the new primary entity picks it up again.
	e.call(&e.hr, "POST", "/api/hris/payroll/"+runB+"/calculate", nil, 200)
	if a, b := lateOf(runA), lateOf(runB); a != "0.00" || b != "50000.00" {
		t.Fatalf("after recalc B: late A %s B %s", a, b)
	}
}

func TestMultiEntityPaidAndPayslip(t *testing.T) {
	e := setup(t)
	e.employee("HR Payroll", e.hr.UserID, true)
	crew := e.employee("Crew", "", true) // no account → no company via Companies()
	e.salary(crew, e.co, 5_000_000)
	e.work.placements = map[string][]domain.Placement{crew: {{BranchID: e.store, IsPrimary: true, Start: "2026-01-01"}}}

	id := e.newRun(6, 2099, e.co)
	e.call(&e.hr, "POST", "/api/hris/payroll/"+id+"/calculate", nil, 200)
	for _, st := range []string{"processing", "completed", "paid"} {
		e.call(&e.hr, "PUT", "/api/hris/payroll/"+id, map[string]any{"status": st}, 200)
	}
	var companies string
	e.scalar(&companies, `SELECT (payload->'companies')::text FROM platform.outbox_events
		WHERE topic = 'payroll.run.paid' AND key = $1`, id)
	// jsonb renders `"key": value` with a space, so compare ignoring whitespace.
	flat := strings.ReplaceAll(companies, " ", "")
	if !strings.Contains(flat, `"company_id":"`+e.co+`"`) || strings.Contains(flat, `"company_id":null`) {
		t.Fatalf("paid event companies %s", companies)
	}
	slips := e.call(&e.hr, "GET", "/api/hris/payslips?payroll_run_id="+id, nil, 200)["data"].([]any)
	if run := slips[0].(map[string]any)["payroll_run"].(map[string]any); run["company_name"] != "Entitas Uji" {
		t.Fatalf("payslip run embed %v", run)
	}
}

// TestBackfillCombinedRunEntity runs delta 20261016110000: a run from before payroll per
// entity gets an entity only when every slip belongs to that one entity and nothing
// contradicts it; running it again changes nothing.
func TestBackfillCombinedRunEntity(t *testing.T) {
	e := setup(t)
	delta, err := os.ReadFile("../../../database/migrations/deltas/20261016110000_payroll_runs_backfill_entity.sql")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := e.entity("Entitas Lain")
	solo := e.employee("Satu Entitas", "", true)
	e.salary(solo, e.co, 1_000_000)
	both := e.employee("Dua Entitas", "", true)
	e.salary(both, e.co, 1_000_000)
	e.salary(both, other, 1_000_000)
	unknown := e.employee("Tanpa Entitas", "", true) // no salary with an entity, no placement

	combinedRun := func(month int, employees ...string) string {
		var id string
		e.scalar(&id, `INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status)
			VALUES ('Run uji', $1, 2097, 'draft') RETURNING id::text`, month)
		for _, emp := range employees {
			e.exec(`INSERT INTO hris.payroll_details (payroll_run_id, employee_id) VALUES ($1, $2)`, id, emp)
		}
		return id
	}
	oneEntity := combinedRun(1, solo)
	multiEntity := combinedRun(2, solo, both)
	journaled := combinedRun(3, solo)
	taken := combinedRun(4, solo)
	empty := combinedRun(5)
	unknownEntity := combinedRun(6, solo, unknown)

	// The entity already has a run in April.
	e.exec(`INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status, company_id)
		VALUES ('Run entitas', 4, 2097, 'draft', $1)`, e.co)
	// A journal of the March run was posted to the other entity.
	var year, period string
	e.scalar(&year, `INSERT INTO accounting.fiscal_years (company_id, code, name, start_date, end_date, is_active)
		VALUES ($1, 'FY-UJI', 'Tahun Uji', '2097-01-01', '2097-12-31', true) RETURNING id::text`, other)
	e.scalar(&period, `INSERT INTO accounting.fiscal_periods (fiscal_year_id, period_no, name, start_date, end_date, status)
		VALUES ($1, 3, 'Maret 2097', '2097-03-01', '2097-03-31', 'OPEN') RETURNING id::text`, year)
	e.exec(`INSERT INTO accounting.journal_entries (company_id, entry_no, entry_date, fiscal_period_id, source_document_id)
		VALUES ($1, 'JU-UJI-1', '2097-03-31', $2, $3)`, other, period, journaled)

	for i := 0; i < 2; i++ {
		e.exec(string(delta))
	}

	companyOf := func(id string) string {
		var c *string
		e.scalar(&c, `SELECT company_id::text FROM hris.payroll_runs WHERE id = $1`, id)
		if c == nil {
			return ""
		}
		return *c
	}
	if got := companyOf(oneEntity); got != e.co {
		t.Fatalf("one-entity run company = %q, want %q", got, e.co)
	}
	for name, id := range map[string]string{"multi-entity employee": multiEntity, "journal elsewhere": journaled,
		"entity run exists": taken, "no slips": empty, "unknown entity": unknownEntity} {
		if got := companyOf(id); got != "" {
			t.Fatalf("%s: run company = %q, want combined (NULL)", name, got)
		}
	}
}

// EPIC-075 R7/R8: a new version with the same entity and effective date as an
// existing one is refused with a clear message and changes nothing; before,
// the active version was deactivated first and the failed insert left the
// employee with no active salary (kasbon then said "belum punya struktur gaji
// aktif").
func TestSalaryDuplicateEffectiveKeepsActive(t *testing.T) {
	e := setup(t)
	emp := e.employee("Gaji Kembar", "", true)
	post := func(date string, base, status int) map[string]any {
		return e.call(&e.hr, "POST", "/api/hris/employee-salary",
			map[string]any{"employee_id": emp, "base_salary": base, "effective_date": date, "company_id": e.co}, status)
	}
	activeBase := func() (n int, base string) {
		e.scalar(&n, `SELECT count(*)::int FROM hris.employee_salary WHERE employee_id = $1 AND is_active`, emp)
		e.scalar(&base, `SELECT coalesce(max(base_salary)::text, '') FROM hris.employee_salary WHERE employee_id = $1 AND is_active`, emp)
		return
	}
	post("2026-01-01", 3_000_000, 200)
	wantErr(t, post("2026-01-01", 3_500_000, 409), "Struktur gaji karyawan ini untuk entitas tersebut dengan tanggal efektif 2026-01-01 sudah ada. Ubah lewat Edit atau pilih tanggal efektif lain.")
	if n, base := activeBase(); n != 1 || base != "3000000.00" {
		t.Fatalf("after duplicate: %d active, base %s; want the first version still active", n, base)
	}
	post("2026-02-01", 3_500_000, 200)
	if n, base := activeBase(); n != 1 || base != "3500000.00" {
		t.Fatalf("after new date: %d active, base %s", n, base)
	}
}

// EPIC-075 sisa: a same-date version that is no longer active is not on the
// active-only list, so the message must not send HR to "Edit".
func TestSalaryDuplicateEffectiveInactive(t *testing.T) {
	e := setup(t)
	emp := e.employee("Gaji Lama", "", true)
	post := func(date string, status int) map[string]any {
		return e.call(&e.hr, "POST", "/api/hris/employee-salary",
			map[string]any{"employee_id": emp, "base_salary": 3_000_000, "effective_date": date, "company_id": e.co}, status)
	}
	post("2026-01-01", 200)
	post("2026-03-01", 200) // 2026-01-01 is now inactive
	wantErr(t, post("2026-01-01", 409), "Tanggal efektif 2026-01-01 sudah dipakai versi struktur gaji lama yang sudah tidak aktif. Pilih tanggal efektif lain.")
}
