package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"nuhabit/backend/internal/modules/payroll"
	"nuhabit/backend/internal/platform/extract"
	"nuhabit/backend/internal/platform/testutil"
)

// The payslip PDF and the KPI snapshot run on the real adapters inside a
// rolled-back transaction.

var payrollFilesNow = time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)

func payrollFilesSetup(t *testing.T, staff ...testutil.StaffOptions) ([]testutil.Staff, func(string, ...any) string,
	func(s *testutil.Staff, method, path string, body any) *httptest.ResponseRecorder) {
	t.Helper()
	accounts := make([]testutil.Staff, len(staff))
	for i, o := range staff {
		accounts[i] = testutil.CreateStaff(t, o)
	}
	deps := testutil.Deps(t, func() time.Time { return payrollFilesNow })
	tx := testutil.Tx(t)
	ctx := context.Background()
	scalar := func(sql string, args ...any) string {
		t.Helper()
		var v *string
		if err := tx.QueryRow(ctx, sql, args...).Scan(&v); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		if v == nil {
			return ""
		}
		return *v
	}
	mux := testutil.Mux(payroll.NewOn(deps, tx, payrollPorts()))
	serve := func(s *testutil.Staff, method, path string, body any) *httptest.ResponseRecorder {
		r := testutil.Request(method, path, body)
		if s != nil {
			r = testutil.AsStaff(r, *s)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec
	}
	return accounts, scalar, serve
}

func errorOf(t *testing.T, rec *httptest.ResponseRecorder, status int, msg string) {
	t.Helper()
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != status || (msg != "" && body["error"] != msg) {
		t.Fatalf("got %d %s, want %d %q", rec.Code, rec.Body, status, msg)
	}
}

func TestPayslipPDFRoute(t *testing.T) {
	none := map[string][]string{"go_test_none": nil}
	// HR sees any slip only with the Penggajian menu (EPIC-057); without it
	// an hrd account is limited to its own like everyone else.
	staff, scalar, serve := payrollFilesSetup(t,
		testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.compensation": nil}},
		testutil.StaffOptions{Role: "pos", Menus: none},
		testutil.StaffOptions{Role: "pos", Menus: none},
		testutil.StaffOptions{Role: "hrd", Menus: none})
	hr, owner, other, hrNoPayroll := &staff[0], &staff[1], &staff[2], &staff[3]
	emp := scalar(`INSERT INTO hris.employees (full_name, nip, email, phone, join_date, user_id)
		VALUES ('Budi Slip', 'SLIP-1', $2, '08', '2024-01-01', $1) RETURNING id::text`, owner.UserID, testutil.RandomHex(4)+"@slip.test")
	scalar(`INSERT INTO hris.employees (full_name, nip, email, phone, join_date, user_id)
		VALUES ('Lain', 'SLIP-2', $2, '08', '2024-01-01', $1) RETURNING id::text`, other.UserID, testutil.RandomHex(4)+"@slip.test")
	run := scalar(`INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status, paid_at)
		VALUES ('Juni 2098', 6, 2098, 'paid', '2098-06-25T03:00:00Z') RETURNING id::text`)
	slip := scalar(`INSERT INTO hris.payroll_details (payroll_run_id, employee_id, base_salary, meal_allowance, gross_salary,
		bpjs_tk_jht_deduction, loan_deduction, total_deductions, net_salary, working_days, present_days, overtime_hours, loan_details)
		VALUES ($1, $2, 5000000, 250000, 5250000, 105000, 500000, 605000, 4645000, 22, 21, 0,
		'[{"loan_id":"l1","loan_type":"kasbon","installment_no":2,"tenor_months":3,"amount":500000,"remaining_before":1000000,"remaining_after":500000}]')
		RETURNING id::text`, run, emp)
	path := "/api/hris/payslips/" + slip + "/pdf"

	errorOf(t, serve(nil, "GET", path, nil), 401, "Unauthorized")
	errorOf(t, serve(owner, "GET", "/api/hris/payslips/abc/pdf", nil), 400, "ID slip tidak valid")
	errorOf(t, serve(owner, "GET", "/api/hris/payslips/11111111-1111-4111-8111-111111111111/pdf", nil), 404, "Slip gaji tidak ditemukan")
	errorOf(t, serve(other, "GET", path, nil), 403, "Insufficient permissions")
	errorOf(t, serve(hrNoPayroll, "GET", path, nil), 403, "Insufficient permissions")
	if rec := serve(hr, "GET", path, nil); rec.Code != 200 {
		t.Fatalf("hr = %d %s", rec.Code, rec.Body)
	}
	rec := serve(owner, "GET", path, nil)
	h := rec.Header()
	if rec.Code != 200 || h.Get("Content-Type") != "application/pdf" || h.Get("Cache-Control") != "no-store, private" ||
		h.Get("Content-Length") != strconv.Itoa(rec.Body.Len()) ||
		h.Get("Content-Disposition") != `attachment; filename="Slip-Gaji-Budi-Slip-Juni-2098.pdf"` {
		t.Fatalf("pdf = %d %v", rec.Code, h)
	}
	text, err := extract.PDFText(rec.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SLIP GAJI — JUNI 2098", "Budi Slip", "SLIP-1", "21 dari 22 hari kerja", "Gaji Pokok",
		"Rp 5.000.000", "Tunjangan Makan", "Rp 5.250.000", "BPJS TK (JHT)", "2,0%", "-Rp 105.000",
		"Cicilan Kasbon (2/3)", "Total Potongan", "-Rp 605.000", "Rp 4.645.000", "Dibayarkan pada 25 Juni 2098."} {
		if !strings.Contains(text, want) {
			t.Fatalf("payslip misses %q: %s", want, text)
		}
	}
	if strings.Contains(text, "Tunjangan Tetap") || strings.Contains(text, "Cicilan Pinjaman") {
		t.Fatalf("zero rows printed: %s", text)
	}

	// 30 PDFs per minute per account.
	scalar(`UPDATE platform.rate_limits SET count = 30 WHERE key = $1 RETURNING key`, "payslip_pdf_"+owner.UserID)
	errorOf(t, serve(owner, "GET", path, nil), 429, "Terlalu banyak permintaan, coba lagi sebentar lagi")
}

func TestKPISnapshotRoute(t *testing.T) {
	staff, scalar, serve := payrollFilesSetup(t,
		testutil.StaffOptions{Role: "hrd", Menus: map[string][]string{"hris.performance": nil}},
		testutil.StaffOptions{Role: "pos", Menus: map[string][]string{"go_test_none": nil}},
		testutil.StaffOptions{Role: "pos", Menus: map[string][]string{"go_test_none": nil}})
	hr, outsider, worker := &staff[0], &staff[1], &staff[2]
	dept := scalar(`INSERT INTO hris.departments (name, code) VALUES ('KPI Go', $1) RETURNING id::text`, "KPI-"+testutil.RandomHex(3))
	emp := scalar(`INSERT INTO hris.employees (full_name, nip, email, phone, join_date, user_id, department_id)
		VALUES ('Kpi Worker', $3, $4, '08', '2024-01-01', $1, $2) RETURNING id::text`,
		worker.UserID, dept, "KPI-"+testutil.RandomHex(3), testutil.RandomHex(4)+"@kpi.test")
	shift := scalar(`INSERT INTO hris.shifts (name, start_time, end_time, break_minutes) VALUES ('KPI Pagi', '08:00', '17:00', 60) RETURNING id::text`)
	// Mondays only; June 2099 has five (1, 8, 15, 22, 29), one on approved leave.
	scalar(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from) VALUES ($1, 1, $2, '2099-01-01') RETURNING id::text`, emp, shift)
	scalar(`INSERT INTO hris.attendance (employee_id, date, status, is_late, late_minutes) VALUES
		($1, '2099-06-01', 'present', false, 0), ($1, '2099-06-08', 'present', true, 15) RETURNING id::text`, emp)
	scalar(`INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status)
		VALUES ($1, 'annual', '2099-06-15', '2099-06-15', 1, 'kpi', 'approved') RETURNING id::text`, emp)
	scalar(`INSERT INTO performance.kpi_department_indicators (department_id, indicator_id, weight)
		SELECT $1, id, CASE code WHEN 'att_ontime' THEN 60 ELSE 40 END
		FROM performance.kpi_indicators WHERE code IN ('att_ontime', 'att_late_ratio') RETURNING id::text`, dept)

	errorOf(t, serve(outsider, "POST", "/api/hris/kpi/snapshot", map[string]any{}), 403, "")
	errorOf(t, serve(hr, "POST", "/api/hris/kpi/snapshot", map[string]any{"period_month": 13}), 400, "Periode tidak valid")
	errorOf(t, serve(hr, "POST", "/api/hris/kpi/snapshot", map[string]any{"period_year": 2101}), 400, "Periode tidak valid")

	snap := func() map[string]any {
		t.Helper()
		rec := serve(hr, "POST", "/api/hris/kpi/snapshot", map[string]any{"period_month": "6", "period_year": 2099})
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 200 {
			t.Fatalf("snapshot = %d %s", rec.Code, rec.Body)
		}
		return body
	}
	body := snap()
	data := body["data"].(map[string]any)
	if data["period_year"] != 2099.0 || data["period_month"] != 6.0 || data["scorecards_skipped_final"] != 0.0 ||
		data["collectors"].(map[string]any)["att_late_ratio"].(float64) < 1 ||
		body["message"] != "Snapshot KPI 6/2099: "+strconv.Itoa(int(data["scorecards_upserted"].(float64)))+" scorecard diperbarui" {
		t.Fatalf("summary = %v", body)
	}
	// on time 1 of 4 scheduled Mondays (one on leave); 15 late minutes of 5 × 480.
	got := scalar(`SELECT string_agg(i.code || '=' || s.actual::text || '/' || s.attainment::text || '/' || s.sample_size::text
		|| '/' || COALESCE(s.source_detail->>'leaveDays', '-'), ',' ORDER BY i.code)
		FROM performance.kpi_snapshots s JOIN performance.kpi_indicators i ON i.id = s.indicator_id
		WHERE s.employee_id = $1 AND s.period_year = 2099 AND s.period_month = 6`, emp)
	if !strings.Contains(got, "att_late_ratio=0.0063/1.2000/5.00/-") || !strings.Contains(got, "att_ontime=0.2500/0.2632/4.00/1") {
		t.Fatalf("snapshots %s", got)
	}
	if card := scalar(`SELECT score::text || '/' || status || '/' || role_code FROM performance.kpi_scorecards
		WHERE employee_id = $1 AND period_year = 2099 AND period_month = 6`, emp); card != "63.79/draft/pos" {
		t.Fatalf("scorecard %s", card)
	}

	// A final scorecard is frozen and reported.
	scalar(`UPDATE performance.kpi_scorecards SET status = 'final', score = 50 WHERE employee_id = $1 RETURNING id::text`, emp)
	body = snap()
	if !strings.HasSuffix(body["message"].(string), ", 1 final dilewati") {
		t.Fatalf("message %v", body["message"])
	}
	if score := scalar(`SELECT score::text FROM performance.kpi_scorecards WHERE employee_id = $1 AND period_year = 2099`, emp); score != "50.00" {
		t.Fatalf("final overwritten: %s", score)
	}
}

func TestHrisCompanyAdapter(t *testing.T) {
	tx := testutil.Tx(t)
	ctx := context.Background()
	if _, err := tx.Exec(ctx, `INSERT INTO configuration.app_settings (key, value) VALUES ('company_city', 'Bandung')
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`); err != nil {
		t.Fatal(err)
	}
	got, err := hrisCompany{}.GetMany(ctx, tx, []string{"company_city", "company_signer_name"})
	if err != nil || *got["company_city"] != "Bandung" {
		t.Fatalf("settings %v %v", got, err)
	}
	if _, err := (hrisCompany{}).FirstCompanyName(ctx, tx); err != nil {
		t.Fatal(err)
	}
}
