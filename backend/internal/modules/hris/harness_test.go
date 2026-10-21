package hris

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/module"
	"nuhabit/backend/internal/platform/testutil"
)

// Integration tests run every route against TEST_DATABASE_URL. Staff
// accounts and sessions are committed by testutil (and removed in cleanup)
// because the auth service reads them from the pool; every HRIS row is
// written inside one transaction that is rolled back, the module included.

var fixedNow = time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC) // Monday 10:00 WIB

type harness struct {
	t    *testing.T
	deps module.Deps
	tx   pgx.Tx
	svc  *Service
	mux  http.Handler
	wa   *fakeWhatsApp
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	deps := testutil.Deps(t, func() time.Time { return fixedNow })
	tx := testutil.Tx(t)
	wa := &fakeWhatsApp{}
	ports := Ports{Payroll: fakePayroll{}, Salary: fakeSalary{}, Recruitment: fakeRecruitment{}, Directory: fakeDirectory{},
		WhatsApp: wa, HolidayCalendar: fakeCalendar{}}
	svc := NewService(newStore(tx), ports, deps.Log, deps.Now)
	deps.Events.Subscribe(TopicLeaveRequested, "hris.leave-request-whatsapp", svc.notifyLeaveRequest)
	deps.Events.Subscribe(TopicOvertimeRequested, "hris.overtime-request-whatsapp", svc.notifyOvertimeRequest)
	if err := deps.Events.Register(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, deps: deps, tx: tx, svc: svc, mux: testutil.Mux(mod{handlers{svc: svc, auth: deps.Auth}}), wa: wa}
}

func (h *harness) exec(sql string, args ...any) {
	h.t.Helper()
	if _, err := h.tx.Exec(context.Background(), sql, args...); err != nil {
		h.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (h *harness) scalar(sql string, args ...any) string {
	h.t.Helper()
	var v *string
	if err := h.tx.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		h.t.Fatalf("query %q: %v", sql, err)
	}
	if v == nil {
		return ""
	}
	return *v
}

func (h *harness) department(name string) string {
	return h.scalar(`INSERT INTO hris.departments (name, code) VALUES ($1, $2) RETURNING id::text`,
		name, "T"+testutil.RandomHex(4))
}

type emp struct {
	name, status, join string
	user, dept, boss   *string
	inactive           bool
	phone              string
}

func (h *harness) employee(e emp) string {
	if e.status == "" {
		e.status = "permanent"
	}
	if e.join == "" {
		e.join = "2025-01-10"
	}
	if e.name == "" {
		e.name = "Karyawan " + testutil.RandomHex(3)
	}
	return h.scalar(`INSERT INTO hris.employees (full_name, nip, email, phone, join_date, employment_status,
		user_id, department_id, reporting_to, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id::text`,
		e.name, "T-"+testutil.RandomHex(5), testutil.RandomHex(5)+"@hris.test", e.phone, e.join, e.status,
		e.user, e.dept, e.boss, !e.inactive)
}

// staff creates a signed-in account with role and menus, optionally linked
// to a new employee record (returned id, "" when unlinked).
func (h *harness) staff(role string, menus []string, linked bool, opts ...emp) (testutil.Staff, string) {
	h.t.Helper()
	grant := map[string][]string{}
	for _, m := range menus {
		grant[m] = []string{"read", "create", "update", "delete", "approve"}
	}
	if len(grant) == 0 {
		grant = map[string][]string{"go_test_none": nil}
	}
	s := testutil.CreateStaff(h.t, testutil.StaffOptions{Role: role, Menus: grant})
	// Cleanups run last-in first-out: roll the fixture transaction back
	// before testutil deletes the account its rows reference.
	h.t.Cleanup(func() { _ = h.tx.Rollback(context.Background()) })
	if !linked {
		return s, ""
	}
	e := emp{}
	if len(opts) > 0 {
		e = opts[0]
	}
	e.user = &s.UserID
	return s, h.employee(e)
}

type call struct {
	status int
	body   map[string]any
	raw    string
}

func (c call) data() map[string]any { m, _ := c.body["data"].(map[string]any); return m }
func (c call) list() []any          { l, _ := c.body["data"].([]any); return l }

func (h *harness) do(s *testutil.Staff, method, path string, body any) call {
	h.t.Helper()
	r := testutil.Request(method, path, body)
	if s != nil {
		r = testutil.AsStaff(r, *s)
	}
	// Each request runs in a savepoint so a request that fails in the
	// database (a 409 on a unique violation) does not abort the fixture
	// transaction, as autocommit statements would not in production.
	ctx := context.Background()
	h.exec("SAVEPOINT harness_request")
	rec, out := testutil.Do(h.t, h.mux, r)
	if _, err := h.tx.Exec(ctx, "RELEASE SAVEPOINT harness_request"); err != nil {
		h.exec("ROLLBACK TO SAVEPOINT harness_request")
		h.exec("RELEASE SAVEPOINT harness_request")
	}
	return call{rec.Code, out, rec.Body.String()}
}

func expect(t *testing.T, c call, status int, errMsg string) {
	t.Helper()
	if c.status != status {
		t.Fatalf("status %d, want %d (body %s)", c.status, status, c.raw)
	}
	if errMsg != "" && c.body["error"] != errMsg {
		t.Fatalf("error %q, want %q (body %s)", c.body["error"], errMsg, c.raw)
	}
}

// keys lists a JSON object's keys in document order.
func keys(t *testing.T, raw string, path ...string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	v, err := decodeOrdered(dec)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range path {
		row, _ := v.(*Row)
		if row == nil {
			t.Fatalf("no object at %v", path)
		}
		v = row.Get(p)
		if list, ok := v.([]any); ok && len(list) > 0 {
			v = list[0]
		}
	}
	row, _ := v.(*Row)
	if row == nil {
		t.Fatalf("no object at %v in %s", path, raw)
	}
	return row.keys
}

/* ── Fake ports ──────────────────────────────────────────────────────── */

type fakePayroll struct{}

func (fakePayroll) LatestPaidPayslip(context.Context, database.Querier, string) (*Payslip, error) {
	name := "Payroll September"
	paid := "2026-09-30 10:00:00+07"
	return &Payslip{NetSalary: "5250000.00", RunName: &name, PeriodMonth: 9, PeriodYear: 2026, PaidAt: &paid}, nil
}
func (fakePayroll) ActiveLoans(context.Context, database.Querier, string) (LoanSummary, error) {
	return LoanSummary{Count: 1, TotalRemaining: "1000000.00", MonthlyInstallment: "250000.00"}, nil
}
func (fakePayroll) RecentLoans(context.Context, database.Querier, string) ([]RecentLoan, error) {
	return []RecentLoan{{ID: "loan-1", LoanType: "kasbon", PrincipalAmount: "1500000.00", Status: "pending",
		CreatedAt: "2026-10-04 09:00:00+07"}}, nil
}
func (fakePayroll) LatestKPISummary(context.Context, database.Querier, string) (KPISummary, error) {
	return KPISummary{}, nil
}
func (fakePayroll) PendingLoanCount(context.Context, database.Querier) (int64, error) { return 2, nil }
func (fakePayroll) LoanUpdatesSince(context.Context, database.Querier, string, *time.Time) (int64, error) {
	return 0, nil
}

// fakePeriodRuns are the payroll runs fakePayroll reports (tests set it).
var fakePeriodRuns []PeriodRun

func (fakePayroll) PeriodRuns(_ context.Context, _ database.Querier, year int) ([]PeriodRun, error) {
	var out []PeriodRun
	for _, r := range fakePeriodRuns {
		if r.Year == year {
			out = append(out, r)
		}
	}
	return out, nil
}

type fakeSalary struct{}

func (fakeSalary) ActiveBaseSalary(context.Context, database.Querier, string) (*string, error) {
	v := "4800000.00"
	return &v, nil
}
func (fakeSalary) SyncFromContract(context.Context, database.Querier, SalarySync) error { return nil }

type fakeRecruitment struct{}

func (fakeRecruitment) PromotedCandidate(context.Context, database.Querier, string) (*PromotedCandidate, error) {
	return nil, nil
}

type fakeDirectory struct{}

func (fakeDirectory) Account(context.Context, database.Querier, string) (*Account, error) {
	return &Account{Email: "akun@hris.test", CreatedAt: fixedNow}, nil
}
func (fakeDirectory) SuperAdminUserIDs(context.Context, database.Querier) ([]string, error) {
	return []string{}, nil
}
func (fakeDirectory) BrandNames(context.Context, database.Querier, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

type fakeWhatsApp struct {
	sent []string
}

func (f *fakeWhatsApp) Claim(ctx context.Context, q database.Querier, _, key, _ string, _ []string) (string, bool, error) {
	return key, true, nil
}
func (f *fakeWhatsApp) Release(context.Context, database.Querier, string) error { return nil }
func (f *fakeWhatsApp) Configured(context.Context) bool                         { return true }
func (f *fakeWhatsApp) SendText(_ context.Context, target, message string) (bool, bool, string) {
	f.sent = append(f.sent, target+"|"+message)
	return true, false, ""
}

type fakeCalendar struct{}

func (fakeCalendar) FetchICS(context.Context) (string, error) {
	return "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20261225\r\nUID:natal@test\r\nSUMMARY:Hari Raya Natal\r\nEND:VEVENT\r\nEND:VCALENDAR", nil
}
