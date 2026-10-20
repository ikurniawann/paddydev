package payroll

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/modules/payroll/domain"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/testutil"
)

// Integration tests against TEST_DATABASE_URL. Every write runs in one
// transaction that is rolled back; staff fixtures come from testutil. The
// HRIS ports are in-memory fakes over employees the test inserts (the SQL
// adapters are exercised in internal/app).

type fakeEmployees struct {
	byUser    map[string]string
	briefs    map[string]EmployeeBrief
	order     []string
	roles     map[string]string // employee id → KPI role
	companies map[string]string // employee id → company id
}

func (f *fakeEmployees) Companies(_ context.Context, _ database.Querier, ids []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range ids {
		if c, ok := f.companies[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

func (f *fakeEmployees) IDByEmail(_ context.Context, _ database.Querier, email string) (string, error) {
	for _, id := range f.order {
		if b := f.briefs[id]; b.Email != nil && *b.Email == email {
			return id, nil
		}
	}
	return "", nil
}

func (f *fakeEmployees) DirectReports(_ context.Context, _ database.Querier, managerID string) ([]string, error) {
	var out []string
	for _, id := range f.order {
		if b := f.briefs[id]; b.IsActive && b.ReportingTo != nil && *b.ReportingTo == managerID {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fakeEmployees) KPIRole(_ context.Context, _ database.Querier, id string) (string, bool, error) {
	if _, ok := f.briefs[id]; !ok {
		return "", false, nil
	}
	if role, ok := f.roles[id]; ok {
		return role, true, nil
	}
	return "employee", true, nil
}

func (f *fakeEmployees) DepartmentMembers(_ context.Context, _ database.Querier, dept string) ([]Member, error) {
	var out []Member
	for _, id := range f.order {
		if b := f.briefs[id]; b.IsActive && b.DepartmentID != nil && *b.DepartmentID == dept {
			out = append(out, Member{b.ID, b.FullName})
		}
	}
	return out, nil
}

type fakeDepartments struct {
	all []Department
}

func (f *fakeDepartments) All(context.Context, database.Querier) ([]Department, error) {
	return f.all, nil
}

func (f *fakeDepartments) ByIDs(_ context.Context, _ database.Querier, ids []string) (map[string]Department, error) {
	out := map[string]Department{}
	for _, d := range f.all {
		for _, id := range ids {
			if d.ID == id {
				out[id] = d
			}
		}
	}
	return out, nil
}

func (f *fakeEmployees) IDByUser(_ context.Context, _ database.Querier, userID string) (string, error) {
	return f.byUser[userID], nil
}

func (f *fakeEmployees) Briefs(_ context.Context, _ database.Querier, ids []string) (map[string]EmployeeBrief, error) {
	out := map[string]EmployeeBrief{}
	for _, id := range ids {
		if b, ok := f.briefs[id]; ok {
			out[id] = b
		}
	}
	return out, nil
}

func (f *fakeEmployees) Active(context.Context, database.Querier) ([]EmployeeBrief, error) {
	var out []EmployeeBrief
	for _, id := range f.order {
		if b := f.briefs[id]; b.IsActive {
			out = append(out, b)
		}
	}
	return out, nil
}

type fakeWorkforce struct {
	// lastRange is the period PeriodRecords was last asked for.
	lastRange  [2]string
	records    map[string]domain.WorkforceRecords
	holidays   map[string]bool
	placements map[string][]domain.Placement
}

func (f *fakeWorkforce) PeriodRecords(_ context.Context, _ database.Querier, id, start, end string) (domain.WorkforceRecords, error) {
	f.lastRange = [2]string{start, end}
	return f.records[id], nil
}

func (f *fakeWorkforce) Holidays(context.Context, database.Querier, string, string) (map[string]bool, error) {
	return f.holidays, nil
}

func (f *fakeWorkforce) Placements(_ context.Context, _ database.Querier, id, _, _ string) ([]domain.Placement, error) {
	return f.placements[id], nil
}

// fakeSettings is configuration.app_settings with the company identity.
type fakeSettings struct{}

func (fakeSettings) GetMany(_ context.Context, _ database.Querier, keys []string) (map[string]*string, error) {
	name := "CV Uji Payroll"
	return map[string]*string{"company_legal_name": &name}, nil
}

// fakeEntities is the business hierarchy the test registers.
type fakeEntities struct {
	list     []Entity
	branches map[string]string // branch id → company id
}

func (f *fakeEntities) List(context.Context, database.Querier) ([]Entity, error) { return f.list, nil }

func (f *fakeEntities) BranchCompanies(context.Context, database.Querier) (map[string]string, error) {
	return f.branches, nil
}

// entity inserts a holding, company and one store, and registers them.
func (e *env) entity(name string) (company, branch string) {
	e.t.Helper()
	sfx := testutil.RandomHex(4)
	var holding string
	e.scalar(&holding, `INSERT INTO configuration.holdings (name, code) VALUES ($1, $2) RETURNING id::text`, name+" Group", "H-"+sfx)
	e.scalar(&company, `INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, $2, $3) RETURNING id::text`,
		holding, name, "CO-"+sfx)
	e.scalar(&branch, `INSERT INTO configuration.branches (company_id, name, code) VALUES ($1, $2, $3) RETURNING id::text`,
		company, name+" Toko", "BR-"+sfx)
	e.ents.list = append(e.ents.list, Entity{ID: company, Code: "CO-" + sfx, Name: name, HoldingName: name + " Group", IsActive: true})
	e.ents.branches[branch] = company
	return company, branch
}

// salary inserts an active salary of employeeID paid by companyID.
func (e *env) salary(employeeID, companyID string, base int) {
	e.t.Helper()
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, ptkp_status, effective_date)
		VALUES ($1, $2, $3, 'TK/0', '2026-01-01')`, employeeID, companyID, base)
}

// newRun creates a draft run of companyID and returns its id.
func (e *env) newRun(month, year int, companyID string) string {
	e.t.Helper()
	body := map[string]any{"period_month": month, "period_year": year, "company_id": companyID}
	return e.call(&e.hr, "POST", "/api/hris/payroll", body, 200)["data"].(map[string]any)["id"].(string)
}

type env struct {
	t         *testing.T
	ctx       context.Context
	tx        pgx.Tx
	mux       http.Handler
	emps      *fakeEmployees
	depts     *fakeDepartments
	work      *fakeWorkforce
	hr        testutil.Staff
	staff     testutil.Staff
	ents      *fakeEntities
	co, store string // default entity and its store
}

var fixedNow = time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)

func setup(t *testing.T, periods ...AttendancePeriods) *env {
	t.Helper()
	deps := testutil.Deps(t, func() time.Time { return fixedNow })
	hr := testutil.CreateStaff(t, testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{
		"hris.compensation": nil, "hris.performance": nil, "hris.performance.kpi-config": nil,
		"hris.kepegawaian.loans": {"read", "create", "update", "approve"},
	}})
	staff := testutil.CreateStaff(t, testutil.StaffOptions{Role: "pos", Menus: map[string][]string{"hris.workforce": nil}})
	ctx := context.Background()
	tx, err := deps.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	e := &env{t: t, ctx: ctx, tx: tx, hr: hr, staff: staff,
		emps:  &fakeEmployees{byUser: map[string]string{}, briefs: map[string]EmployeeBrief{}, roles: map[string]string{}, companies: map[string]string{}},
		depts: &fakeDepartments{},
		ents:  &fakeEntities{branches: map[string]string{}},
		work:  &fakeWorkforce{records: map[string]domain.WorkforceRecords{}, holidays: map[string]bool{}}}
	ports := Ports{Employees: e.emps, Departments: e.depts, Workforce: e.work, Settings: fakeSettings{}, Entities: e.ents}
	if len(periods) > 0 {
		ports.AttendancePeriods = periods[0]
	}
	e.mux = testutil.Mux(NewOn(deps, tx, ports))
	e.co, e.store = e.entity("Entitas Uji")
	return e
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.tx.Exec(e.ctx, sql, args...); err != nil {
		e.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (e *env) scalar(dst any, sql string, args ...any) {
	e.t.Helper()
	if err := e.tx.QueryRow(e.ctx, sql, args...).Scan(dst); err != nil {
		e.t.Fatalf("query %q: %v", sql, err)
	}
}

// employee inserts an hris.employees row and registers it with the fakes.
func (e *env) employee(name string, userID string, active bool) string {
	return e.emp(empOpts{name: name, user: userID, inactive: !active})
}

type empOpts struct {
	name, user, dept, reportsTo, email string
	inactive                           bool
}

func (e *env) emp(o empOpts) string {
	e.t.Helper()
	var id string
	nip := "GOT-" + testutil.RandomHex(4)
	if o.email == "" {
		o.email = nip + "@test.local"
	}
	e.scalar(&id, `INSERT INTO hris.employees (full_name, nip, email, phone, join_date, employment_status, is_active,
		user_id, department_id, reporting_to)
		VALUES ($1, $2, $3, '0812000111', '2024-01-15', 'permanent', $4, NULLIF($5, '')::uuid, NULLIF($6, '')::uuid,
		NULLIF($7, '')::uuid) RETURNING id::text`, o.name, nip, o.email, !o.inactive, o.user, o.dept, o.reportsTo)
	phone, email := "0812000111", o.email
	b := EmployeeBrief{ID: id, FullName: o.name, NIP: &nip, Phone: &phone, Email: &email, IsActive: !o.inactive,
		EmploymentStatus: "permanent", JoinDate: "2024-01-15"}
	if o.dept != "" {
		dept := o.dept
		b.DepartmentID, b.HasDepartment = &dept, true
		for _, d := range e.depts.all {
			if d.ID == dept {
				name := d.Name
				b.Department = &name
			}
		}
	}
	if o.reportsTo != "" {
		rt := o.reportsTo
		b.ReportingTo = &rt
	}
	e.emps.briefs[id] = b
	e.emps.order = append(e.emps.order, id)
	if o.user != "" {
		e.emps.byUser[o.user] = id
	}
	return id
}

// department inserts an hris.departments row and registers it.
func (e *env) department(name string) string {
	e.t.Helper()
	var id string
	e.scalar(&id, `INSERT INTO hris.departments (name, code) VALUES ($1, $2) RETURNING id::text`, name, "GOT-"+testutil.RandomHex(4))
	e.depts.all = append(e.depts.all, Department{id, name})
	return id
}

func (e *env) call(s *testutil.Staff, method, path string, body any, status int) map[string]any {
	e.t.Helper()
	r := testutil.Request(method, path, body)
	if s != nil {
		r = testutil.AsStaff(r, *s)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	if rec.Code != status {
		e.t.Fatalf("%s %s = %d %s, want %d", method, path, rec.Code, rec.Body.String(), status)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		e.t.Fatalf("%s %s: body %q", method, path, rec.Body.String())
	}
	return out
}

func (e *env) raw(s *testutil.Staff, method, path string, body any) *httptest.ResponseRecorder {
	r := testutil.Request(method, path, body)
	if s != nil {
		r = testutil.AsStaff(r, *s)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	return rec
}

func wantErr(t *testing.T, body map[string]any, msg string) {
	t.Helper()
	if body["success"] != false || body["error"] != msg {
		t.Fatalf("error body = %v, want %q", body, msg)
	}
}

func TestRunLifecycle(t *testing.T) {
	e := setup(t)
	hrEmp := e.employee("HR Payroll", e.hr.UserID, true)
	worker := e.employee("Budi Gaji", "", true)
	e.employee("Tanpa Gaji", "", true)
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, fixed_allowance, ptkp_status, effective_date)
		VALUES ($1, $2, 10000000, 2000000, 'TK/0', '2026-01-01')`, worker, e.co)
	e.exec(`INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months, monthly_installment,
		remaining_balance, first_installment_month, first_installment_year, status, approved_at)
		VALUES ($1, 'kasbon', 1500000, 3, 500000, 1500000, 6, 2099, 'approved', now())`, worker)

	// Guards and validation.
	if rec := e.raw(&e.staff, "GET", "/api/hris/payroll", nil); rec.Code != 403 {
		t.Fatalf("non-compensation = %d", rec.Code)
	}
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/payroll", "nope", 400), "Bulan dan tahun periode wajib diisi dengan benar")
	bad := e.call(&e.hr, "POST", "/api/hris/payroll", map[string]any{"period_month": 13, "period_year": 2099}, 400)
	wantErr(t, bad, "Bulan dan tahun periode wajib diisi dengan benar")
	if d := bad["details"].([]any)[0].(map[string]any); d["message"] != "Too big: expected number to be <=12" {
		t.Fatalf("details %v", d)
	}
	wantErr(t, e.call(&e.hr, "GET", "/api/hris/payroll?year=abc", nil, 400), "Invalid input: expected number, received NaN")

	created := e.call(&e.hr, "POST", "/api/hris/payroll", map[string]any{"period_month": "6", "period_year": 2099, "company_id": e.co}, 200)
	run := created["data"].(map[string]any)
	if created["message"] != "Payroll run berhasil dibuat" || run["run_name"] != "Payroll Juni 2099" ||
		run["status"] != "draft" || run["processed_by"] != hrEmp || run["total_gross"] != "0.00" {
		t.Fatalf("created %v", created)
	}
	runID := run["id"].(string)
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/payroll", map[string]any{"period_month": 6, "period_year": 2099, "company_id": e.co}, 400),
		"Payroll untuk periode ini sudah ada")

	list := e.call(&e.hr, "GET", "/api/hris/payroll?year=2099&status=draft", nil, 200)
	rows := list["data"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["processed_by"].(map[string]any)["full_name"] != "HR Payroll" ||
		rows[0].(map[string]any)["approved_by"] != nil {
		t.Fatalf("list %v", list)
	}
	if rec := e.raw(&e.hr, "GET", "/api/hris/payroll?year=2099", nil); rec.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatal("cache header")
	}
	// Key order follows the table, embeds in place.
	rec := e.raw(&e.hr, "GET", "/api/hris/payroll?year=2099", nil)
	if !strings.HasPrefix(rec.Body.String(), `{"data":[{"id":"`+runID+`","run_name":"Payroll Juni 2099","period_month":6,"period_year":2099,"status":"draft","processed_by":{"id":"`+hrEmp+`","full_name":"HR Payroll","nip":"`) {
		t.Fatalf("list body %s", rec.Body.String())
	}

	// Calculate.
	calc := e.call(&e.hr, "POST", "/api/hris/payroll/"+runID+"/calculate", nil, 200)
	summary := calc["summary"].(map[string]any)
	if calc["message"] != "Payroll berhasil dihitung untuk 1 karyawan, 2 dilewati" || summary["total_employees"] != 1.0 ||
		summary["total_gross"] != 12000000.0 || len(summary["skipped"].([]any)) != 2 {
		t.Fatalf("calc %v", calc)
	}
	if sk := summary["skipped"].([]any)[0].(map[string]any); sk["reason"] != "Belum ada struktur gaji aktif" || sk["full_name"] != "HR Payroll" {
		t.Fatalf("skipped %v", sk)
	}
	detail := calc["data"].([]any)[0].(map[string]any)
	// Loan installment 500.000 due (first installment June 2099), fallback 20 days.
	if detail["gross_salary"] != "12000000.00" || detail["loan_deduction"] != "500000.00" || detail["working_days"] != 20.0 ||
		detail["prorate_factor"] != "1.000000" || detail["employee"].(map[string]any)["full_name"] != "Budi Gaji" {
		t.Fatalf("detail %v", detail)
	}
	if ld := detail["loan_details"].([]any)[0].(map[string]any); ld["installment_no"] != 1.0 || ld["remaining_after"] != 1000000.0 {
		t.Fatalf("loan_details %v", ld)
	}
	var totalNet string
	e.scalar(&totalNet, `SELECT total_net::text FROM hris.payroll_runs WHERE id = $1`, runID)
	if totalNet != "10361155.00" {
		t.Fatalf("total_net %s", totalNet)
	}

	got := e.call(&e.hr, "GET", "/api/hris/payroll/"+runID, nil, 200)["data"].(map[string]any)
	details := got["payroll_details"].([]any)
	if len(details) != 1 || details[0].(map[string]any)["net_salary"] != 10361155.0 ||
		details[0].(map[string]any)["employee"].(map[string]any)["department"] != nil {
		t.Fatalf("get %v", got)
	}
	wantErr(t, e.call(&e.hr, "GET", "/api/hris/payroll/"+"00000000-0000-0000-0000-000000000000", nil, 404), "Payroll run tidak ditemukan")

	// Transitions.
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/payroll/"+runID, map[string]any{"status": "paid"}, 400),
		"Transisi status 'draft' → 'paid' tidak diizinkan")
	up := e.call(&e.hr, "PUT", "/api/hris/payroll/"+runID, map[string]any{"status": "processing", "notes": "cek"}, 200)
	if up["message"] != "Payroll run berhasil diupdate" || up["data"].(map[string]any)["processed_at"] != "2026-10-04T10:00:00.000Z" ||
		up["data"].(map[string]any)["notes"] != "cek" {
		t.Fatalf("processing %v", up)
	}
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/payroll/"+runID+"/calculate", map[string]any{}, 400), "Hanya payroll draft yang bisa dihitung")
	e.call(&e.hr, "PUT", "/api/hris/payroll/"+runID, map[string]any{"status": "completed"}, 200)
	e.exec(`UPDATE hris.payroll_details SET tapera_employer = 30000, tapera_deduction = 25000 WHERE payroll_run_id = $1`, runID)
	paid := e.call(&e.hr, "PUT", "/api/hris/payroll/"+runID, map[string]any{"status": "paid"}, 200)
	if paid["message"] != "Payroll ditandai dibayar — 1 cicilan pinjaman dipotong dari saldo" ||
		paid["data"].(map[string]any)["approved_by"].(map[string]any)["id"] != hrEmp {
		t.Fatalf("paid %v", paid)
	}
	var remaining, paidAmount string
	e.scalar(&remaining, `SELECT remaining_balance::text FROM hris.loans WHERE employee_id = $1`, worker)
	e.scalar(&paidAmount, `SELECT paid_amount::text FROM hris.loans WHERE employee_id = $1`, worker)
	if remaining != "1000000.00" || paidAmount != "500000.00" {
		t.Fatalf("loan after pay %s %s", remaining, paidAmount)
	}
	var payload string
	e.scalar(&payload, `SELECT payload::text FROM platform.outbox_events WHERE topic = 'payroll.run.paid' AND key = $1`, runID)
	if !strings.Contains(payload, `"total_net": "10361155.00"`) || !strings.Contains(payload, `"total_loan_deduction": 500000`) ||
		!strings.Contains(payload, `"user_id": "`+e.hr.UserID+`"`) {
		t.Fatalf("event %s", payload)
	}
	// Employer BPJS totals and the per-company split ride along for the journals.
	var tk, kes float64
	e.scalar(&tk, `SELECT (bpjs_tk_jht_employer + bpjs_tk_jp_employer + bpjs_tk_jkk_employer + bpjs_tk_jkm_employer)::float8
		FROM hris.payroll_details WHERE payroll_run_id = $1`, runID)
	e.scalar(&kes, `SELECT bpjs_kes_employer::float8 FROM hris.payroll_details WHERE payroll_run_id = $1`, runID)
	var event struct {
		TotalBpjsTkEmployer  float64 `json:"total_bpjs_tk_employer"`
		TotalBpjsKesEmployer float64 `json:"total_bpjs_kes_employer"`
		Companies            []struct {
			CompanyID            *string `json:"company_id"`
			TotalGross           float64 `json:"total_gross"`
			TotalNet             float64 `json:"total_net"`
			TotalLoanDeduction   float64 `json:"total_loan_deduction"`
			TotalBpjsTkEmployer  float64 `json:"total_bpjs_tk_employer"`
			TotalBpjsKesEmployer float64 `json:"total_bpjs_kes_employer"`
		} `json:"companies"`
	}
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatal(err)
	}
	if tk <= 0 || kes <= 0 || event.TotalBpjsTkEmployer != tk || event.TotalBpjsKesEmployer != kes || len(event.Companies) != 1 {
		t.Fatalf("bpjs %v %v event %+v", tk, kes, event)
	}
	if c := event.Companies[0]; c.CompanyID == nil || *c.CompanyID != e.co || c.TotalGross != 12000000 ||
		c.TotalNet != 10361155 || c.TotalLoanDeduction != 500000 || c.TotalBpjsTkEmployer != tk || c.TotalBpjsKesEmployer != kes {
		t.Fatalf("company split %+v", c)
	}
	// The employee BPJS deductions and both Tapera shares ride along too.
	var tkEmployee, kesEmployee float64
	e.scalar(&tkEmployee, `SELECT (bpjs_tk_jht_deduction + bpjs_tk_jp_deduction)::float8 FROM hris.payroll_details WHERE payroll_run_id = $1`, runID)
	e.scalar(&kesEmployee, `SELECT bpjs_kes_deduction::float8 FROM hris.payroll_details WHERE payroll_run_id = $1`, runID)
	var shares struct {
		TotalBpjsTkEmployee  float64 `json:"total_bpjs_tk_employee"`
		TotalBpjsKesEmployee float64 `json:"total_bpjs_kes_employee"`
		TotalTaperaEmployer  float64 `json:"total_tapera_employer"`
		TotalTaperaEmployee  float64 `json:"total_tapera_employee"`
		Companies            []struct {
			TotalBpjsTkEmployee  float64 `json:"total_bpjs_tk_employee"`
			TotalBpjsKesEmployee float64 `json:"total_bpjs_kes_employee"`
			TotalTaperaEmployer  float64 `json:"total_tapera_employer"`
			TotalTaperaEmployee  float64 `json:"total_tapera_employee"`
		} `json:"companies"`
	}
	if err := json.Unmarshal([]byte(payload), &shares); err != nil {
		t.Fatal(err)
	}
	if tkEmployee <= 0 || kesEmployee <= 0 || shares.TotalBpjsTkEmployee != tkEmployee || shares.TotalBpjsKesEmployee != kesEmployee ||
		shares.TotalTaperaEmployer != 30000 || shares.TotalTaperaEmployee != 25000 || len(shares.Companies) != 1 ||
		shares.Companies[0].TotalBpjsTkEmployee != tkEmployee || shares.Companies[0].TotalBpjsKesEmployee != kesEmployee ||
		shares.Companies[0].TotalTaperaEmployer != 30000 || shares.Companies[0].TotalTaperaEmployee != 25000 {
		t.Fatalf("employee shares %v %v event %+v", tkEmployee, kesEmployee, shares)
	}
	wantErr(t, e.call(&e.hr, "DELETE", "/api/hris/payroll/"+runID, nil, 400), "Payroll yang sudah dibayar tidak bisa dihapus")

	// Payslips: the employee sees paid slips only; HR may notify.
	slips := e.call(&e.hr, "GET", "/api/hris/payslips?employee_id="+worker, nil, 200)["data"].([]any)
	slip := slips[0].(map[string]any)
	if slip["payroll_run"].(map[string]any)["status"] != "paid" || slip["employee"].(map[string]any)["position"] != nil {
		t.Fatalf("slip %v", slip)
	}
	if !strings.Contains(e.raw(&e.hr, "GET", "/api/hris/payslips?employee_id="+worker, nil).Body.String(), `"employee":{"id":"`+worker) {
		t.Fatal("employee embed precedes payroll_run")
	}
	// year and month filter on the run's period.
	if got := e.call(&e.hr, "GET", "/api/hris/payslips?employee_id="+worker+"&year=2099&month=6", nil, 200)["data"].([]any); len(got) != 1 {
		t.Fatalf("payslips of June 2099: %v", got)
	}
	if got := e.call(&e.hr, "GET", "/api/hris/payslips?employee_id="+worker+"&year=2099&month=7", nil, 200)["data"].([]any); len(got) != 0 {
		t.Fatalf("payslips of July 2099: %v", got)
	}
	if mine := e.call(&e.staff, "GET", "/api/hris/payslips", nil, 200)["data"].([]any); len(mine) != 0 {
		t.Fatalf("staff without employee %v", mine)
	}
	notify := e.call(&e.hr, "POST", "/api/hris/payslips/notify", map[string]any{"payroll_detail_id": slip["id"]}, 200)
	if notify["message"] != "Slip ditandai terkirim — buka WhatsApp untuk mengirim notifikasi" ||
		!strings.HasPrefix(notify["data"].(map[string]any)["wa_link"].(string), "https://wa.me/62812000111?text=Halo%20Budi%20Gaji%2C%20slip%20gaji%20Anda%20periode%20Juni%202099") {
		t.Fatalf("notify %v", notify)
	}
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/payslips/notify", map[string]any{"payroll_detail_id": "x"}, 400), "Invalid UUID")
}

func TestRunDeleteAndPaidMismatch(t *testing.T) {
	e := setup(t)
	worker := e.employee("Siti", "", true)
	var runID string
	e.scalar(&runID, `INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status) VALUES ('X', 7, 2098, 'completed') RETURNING id::text`)
	e.exec(`INSERT INTO hris.payroll_details (payroll_run_id, employee_id, loan_deduction) VALUES ($1, $2, 300000)`, runID, worker)
	res := e.call(&e.hr, "PUT", "/api/hris/payroll/"+runID, map[string]any{"status": "paid"}, 409)
	if !strings.HasPrefix(res["error"].(string), "Cicilan pinjaman di slip tidak lagi cocok") ||
		res["details"].(map[string]any)["shortfalls"].([]any)[0].(map[string]any)["amount"] != 300000.0 {
		t.Fatalf("mismatch %v", res)
	}
	var status string
	e.scalar(&status, `SELECT status FROM hris.payroll_runs WHERE id = $1`, runID)
	if status != "completed" {
		t.Fatal("paid must roll back")
	}
	wantErr(t, e.call(&e.hr, "DELETE", "/api/hris/payroll/nope", nil, 404), "Payroll run tidak ditemukan")
	e.exec(`UPDATE hris.payroll_runs SET status = 'draft' WHERE id = $1`, runID)
	if e.call(&e.hr, "DELETE", "/api/hris/payroll/"+runID, nil, 200)["message"] != "Payroll run berhasil dihapus" {
		t.Fatal("delete")
	}
	var n int
	e.scalar(&n, `SELECT count(*) FROM hris.payroll_details WHERE payroll_run_id = $1`, runID)
	if n != 0 {
		t.Fatal("details cascade")
	}
}

func TestPayrollSettings(t *testing.T) {
	e := setup(t)
	got := e.call(&e.hr, "GET", "/api/hris/payroll-settings?tax_year=2097", nil, 200)["data"].(map[string]any)
	if got["tax_year"] != 2097.0 || got["tax_config"] != nil {
		t.Fatalf("get %v", got)
	}
	if rec := e.raw(&e.hr, "GET", "/api/hris/payroll-settings", nil); !strings.Contains(rec.Body.String(), `"tax_year":2026`) {
		t.Fatalf("default year %s", rec.Body.String())
	}
	admin := testutil.CreateStaff(t, testutil.StaffOptions{Role: "admin", Menus: map[string][]string{"hris.compensation": nil}})
	wantErr(t, e.call(&admin, "PUT", "/api/hris/payroll-settings", map[string]any{"settings": map[string]any{}}, 403),
		"Hanya super admin dan HRD yang boleh mengubah pengaturan payroll")
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/payroll-settings", map[string]any{}, 400), "Tidak ada perubahan yang dikirim")
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/payroll-settings", map[string]any{"settings": map[string]any{
		"pph21_bracket_1": 5, "pph21_bracket_2": 4}}, 400), "pph21_bracket_2 harus lebih besar dari pph21_bracket_1")
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/payroll-settings", map[string]any{"settings": map[string]any{
		"bpjs_tk_jht_employee": 101}}, 400), "Too big: expected number to be <=100")
	saved := e.call(&e.hr, "PUT", "/api/hris/payroll-settings", map[string]any{
		"settings":   map[string]any{"bpjs_tk_jht_employee": "3", "late_deduction_mode": "flat", "npwp": nil},
		"tax_config": map[string]any{"tax_year": 2097, "bracket_1_rate": 6},
	}, 200)
	data := saved["data"].(map[string]any)
	if saved["message"] != "Pengaturan payroll tersimpan" || data["settings"].(map[string]any)["company_name"] != "Perusahaan" ||
		data["settings"].(map[string]any)["bpjs_tk_jht_employee"] != "3.00" || data["tax_config"].(map[string]any)["bracket_1_rate"] != "6.00" {
		t.Fatalf("saved %v", saved)
	}
	// The second save updates the same rows.
	e.call(&e.hr, "PUT", "/api/hris/payroll-settings", map[string]any{"settings": map[string]any{"company_name": "PT Nu"}}, 200)
	var n int
	e.scalar(&n, `SELECT count(*) FROM hris.payroll_settings`)
	if n != 1 {
		t.Fatalf("settings rows %d", n)
	}
}

func TestEmployeeSalary(t *testing.T) {
	e := setup(t)
	emp := e.employee("Rina", "", true)
	bad := e.call(&e.hr, "POST", "/api/hris/employee-salary", map[string]any{"base_salary": 0}, 400)
	wantErr(t, bad, salaryRequired)
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/employee-salary", map[string]any{
		"employee_id": "00000000-0000-0000-0000-000000000000", "base_salary": 1}, 404), "Karyawan tidak ditemukan")
	first := e.call(&e.hr, "POST", "/api/hris/employee-salary", map[string]any{
		"employee_id": emp, "base_salary": "8000000", "effective_date": "2026-01-01", "fixed_allowance": 0, "company_id": e.co}, 200)
	if first["message"] != "Salary structure berhasil dibuat" || first["data"].(map[string]any)["base_salary"] != "8000000.00" ||
		first["data"].(map[string]any)["ptkp_status"] != "TK/0" || first["data"].(map[string]any)["employee"] != nil {
		t.Fatalf("first %v", first)
	}
	firstID := first["data"].(map[string]any)["id"].(string)
	second := e.call(&e.hr, "POST", "/api/hris/employee-salary", map[string]any{
		"employee_id": emp, "base_salary": 9000000, "effective_date": "2026-07-01", "notes": "naik", "company_id": e.co}, 200)["data"].(map[string]any)
	var active bool
	var end string
	e.scalar(&active, `SELECT is_active FROM hris.employee_salary WHERE id = $1`, firstID)
	e.scalar(&end, `SELECT end_date::text FROM hris.employee_salary WHERE id = $1`, firstID)
	if active || end != "2026-07-01" {
		t.Fatalf("previous version %v %s", active, end)
	}

	list := e.call(&e.hr, "GET", "/api/hris/employee-salary?employee_id="+emp, nil, 200)["data"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["employee"].(map[string]any)["full_name"] != "Rina" {
		t.Fatalf("list %v", list)
	}
	got := e.call(&e.hr, "GET", "/api/hris/employee-salary/"+second["id"].(string), nil, 200)["data"].(map[string]any)
	if emb := got["employee"].(map[string]any); emb["phone"] != "0812000111" || emb["position"] != nil {
		t.Fatalf("get %v", got)
	}
	wantErr(t, e.call(&e.hr, "GET", "/api/hris/employee-salary/00000000-0000-0000-0000-000000000000", nil, 404), "Data salary tidak ditemukan")

	upd := e.call(&e.hr, "PUT", "/api/hris/employee-salary/"+second["id"].(string), map[string]any{"base_salary": 9500000, "notes": nil}, 200)
	if upd["data"].(map[string]any)["base_salary"] != "9500000.00" || upd["data"].(map[string]any)["notes"] != nil {
		t.Fatalf("update %v", upd)
	}
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/employee-salary/"+second["id"].(string), map[string]any{"base_salary": -1}, 400), "Data salary tidak valid")
	wantErr(t, e.call(&e.hr, "PUT", "/api/hris/employee-salary/00000000-0000-0000-0000-000000000000", map[string]any{}, 500), "Terjadi kesalahan server")
	if e.call(&e.hr, "DELETE", "/api/hris/employee-salary/"+second["id"].(string), nil, 200)["message"] != "Data salary berhasil dihapus" {
		t.Fatal("delete")
	}
}

func TestLoans(t *testing.T) {
	e := setup(t)
	hrEmp := e.employee("HR Loans", e.hr.UserID, true)
	worker := e.employee("Andi", e.staff.UserID, true)
	inactive := e.employee("Keluar", "", false)

	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans", map[string]any{"loan_type": "", "principal_amount": 0, "tenor_months": 61}, 400), invalidLoan)
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans", map[string]any{"employee_id": inactive, "loan_type": "loan",
		"principal_amount": 1000000, "tenor_months": 2}, 400), "Karyawan sudah tidak aktif")
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans", map[string]any{"employee_id": worker, "loan_type": "loan",
		"principal_amount": 1000000, "tenor_months": 2}, 400),
		"Karyawan belum punya struktur gaji aktif — atur gaji dulu sebelum mengajukan pinjaman")
	e.exec(`INSERT INTO hris.employee_salary (employee_id, company_id, base_salary, effective_date) VALUES ($1, $2, 5000000, '2026-01-01')`, worker, e.co)
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans", map[string]any{"employee_id": worker, "loan_type": "loan",
		"principal_amount": 6000000, "tenor_months": 3}, 400),
		"Cicilan Rp 2.000.000/bulan melebihi batas 30% dari gaji pokok (maks Rp 1.500.000)")

	// The employee files for themselves: interest forced to 0.
	created := e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"employee_id": hrEmp, "loan_type": "kasbon",
		"principal_amount": "3000000", "tenor_months": 3, "interest_rate": 5}, 200)
	loan := created["data"].(map[string]any)
	if created["message"] != "Pengajuan pinjaman berhasil dibuat, menunggu approval" || loan["employee_id"] != worker ||
		loan["interest_rate"] != "0.00" || loan["monthly_installment"] != "1000000.00" || loan["status"] != "pending" {
		t.Fatalf("created %v", created)
	}
	wantErr(t, e.call(&e.staff, "POST", "/api/hris/loans", map[string]any{"loan_type": "kasbon",
		"principal_amount": 100000, "tenor_months": 1}, 400), "Karyawan sudah punya 1 pinjaman aktif (maks 1)")

	mine := e.call(&e.staff, "GET", "/api/hris/loans?employee_id="+hrEmp, nil, 200)["data"].([]any)
	if len(mine) != 1 || mine[0].(map[string]any)["employee"].(map[string]any)["department"] != nil {
		t.Fatalf("mine %v", mine)
	}
	if all := e.call(&e.hr, "GET", "/api/hris/loans?employee_id="+hrEmp, nil, 200)["data"].([]any); len(all) != 0 {
		t.Fatalf("hr filter %v", all)
	}

	loanID := loan["id"].(string)
	if rec := e.raw(&e.staff, "POST", "/api/hris/loans/"+loanID+"/approve", map[string]any{"approved": true}); rec.Code != 403 {
		t.Fatalf("staff approve = %d", rec.Code)
	}
	ok := e.call(&e.hr, "POST", "/api/hris/loans/"+loanID+"/approve", map[string]any{"approved": true}, 200)
	data := ok["data"].(map[string]any)
	if ok["message"] != "Pinjaman disetujui — cicilan mulai bulan depan" || data["approved_by"] != hrEmp ||
		data["first_installment_month"] != 11.0 || data["first_installment_year"] != 2026.0 {
		t.Fatalf("approve %v", ok)
	}
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans/"+loanID+"/approve", map[string]any{"approved": false}, 400), "Pinjaman sudah diproses")
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans/nope/approve", map[string]any{}, 404), "Pinjaman tidak ditemukan")

	var other string
	e.scalar(&other, `INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months, status)
		VALUES ($1, 'loan', 100, 1, 'pending') RETURNING id::text`, hrEmp)
	rej := e.call(&e.hr, "POST", "/api/hris/loans/"+other+"/approve", map[string]any{}, 200)
	if rej["message"] != "Pinjaman ditolak" || rej["data"].(map[string]any)["rejection_reason"] != "Tidak disetujui" ||
		rej["data"].(map[string]any)["is_active"] != false {
		t.Fatalf("reject %v", rej)
	}
}

// Each case fails a statement, which aborts the test transaction, so each
// gets its own environment. In production every statement stands alone.
func TestDatabaseErrorMapping(t *testing.T) {
	cases := []struct {
		method, path string
		body         any
		status       int
		msg          string
	}{
		// query-builder errors surface as plain Errors in TS: 500 even for 22P02
		{"GET", "/api/hris/payroll/not-a-uuid", nil, 500, "Terjadi kesalahan server"},
		{"GET", "/api/hris/loans?employee_id=bad", nil, 500, "Terjadi kesalahan server"},
		// the salary repo keeps the SQLSTATE: mapped to the friendly 4xx
		{"GET", "/api/hris/employee-salary?employee_id=bad", nil, 400, "Format data tidak valid"},
		{"GET", "/api/hris/employee-salary/bad", nil, 400, "Format data tidak valid"},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			e := setup(t)
			wantErr(t, e.call(&e.hr, c.method, c.path, c.body, c.status), c.msg)
		})
	}
	t.Run("duplicate salary version", func(t *testing.T) {
		e := setup(t)
		emp := e.employee("Dup", "", true)
		body := map[string]any{"employee_id": emp, "base_salary": 1, "effective_date": "2026-07-01", "company_id": e.co}
		e.call(&e.hr, "POST", "/api/hris/employee-salary", body, 200)
		wantErr(t, e.call(&e.hr, "POST", "/api/hris/employee-salary", body, 409), "Data sudah ada di sistem")
	})
}
