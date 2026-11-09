package payroll

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-076: prosedur kasbon & Pinjaman Karyawan 01-11-2026.

// setJoin sets the worker's join date in the DB and the fake HRIS port.
func (e *env) setJoin(worker, date string) {
	e.t.Helper()
	e.exec(`UPDATE hris.employees SET join_date = $2 WHERE id = $1`, worker, date)
	b := e.emps.briefs[worker]
	b.JoinDate = date
	e.emps.briefs[worker] = b
}

// plainStaff is a non-HR account (role pos) without HRIS menus.
func (e *env) plainStaff() testutil.Staff {
	e.t.Helper()
	s := testutil.CreateStaff(e.t, testutil.StaffOptions{Role: "pos", Menus: map[string][]string{"go_test_none": nil}})
	// Roll the test transaction back before the account is deleted (it holds
	// rows that reference the account), as hrStaff does.
	e.t.Cleanup(func() { _ = e.tx.Rollback(context.Background()) })
	return s
}

// loanManager gives worker a Manager-level approver: a non-HR account, so
// whatever it reads comes from being the stored approver.
func (e *env) loanManager(worker string) (testutil.Staff, string) {
	e.t.Helper()
	mgrStaff := e.plainStaff()
	mgr := e.employee("Manager Dept", mgrStaff.UserID, true)
	e.exec(`UPDATE hris.employees SET reporting_to = $2 WHERE id = $1`, worker, mgr)
	e.setLevel(mgr, "Manager")
	return mgrStaff, mgr
}

func TestLoanTnCCreate(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "probation") // THP 5.000.000; join 2024-01-15
	post := func(body map[string]any, status int) map[string]any { return e.loan(&e.staff, body, status) }
	probation := "Karyawan masa percobaan (probation) tidak dapat mengajukan kasbon atau pinjaman"
	wantErr(t, post(map[string]any{"loan_type": "loan", "principal_amount": 1000000, "tenor_months": 2}, 400), probation)
	wantErr(t, post(map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 400), probation)

	e.exec(`UPDATE hris.employees SET employment_status = 'daily_worker' WHERE id = $1`, worker)
	b := e.emps.briefs[worker]
	b.EmploymentStatus = "daily_worker"
	e.emps.briefs[worker] = b
	wantErr(t, post(map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 400),
		"Kasbon dan pinjaman hanya untuk karyawan PKWT atau PKWTT")

	b.EmploymentStatus = "pkwt"
	e.emps.briefs[worker] = b
	// fixedNow = 2026-10-04: masuk 2026-07-05 belum genap 3 bulan.
	e.setJoin(worker, "2026-07-05")
	wantErr(t, post(map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 400),
		"Kasbon hanya untuk karyawan dengan masa kerja minimal 3 bulan")
	e.setJoin(worker, "2025-10-05")
	wantErr(t, post(map[string]any{"loan_type": "loan", "principal_amount": 1000000, "tenor_months": 2}, 400),
		"Pinjaman Karyawan hanya untuk karyawan dengan masa kerja minimal 12 bulan")
	e.setJoin(worker, "2025-10-04")
	wantErr(t, post(map[string]any{"loan_type": "loan", "principal_amount": 1000000, "tenor_months": 7}, 400),
		"Cicilan Pinjaman Karyawan 1 sampai 6 kali")
	wantErr(t, post(map[string]any{"loan_type": "loan", "principal_amount": 10000001, "tenor_months": 6}, 400),
		"Pinjaman Karyawan maksimal 200% THP (maks Rp 10.000.000)")

	_, mgr := e.loanManager(worker)
	created := post(map[string]any{"loan_type": "loan", "principal_amount": 10000000, "tenor_months": 6}, 200)["data"].(map[string]any)
	if created["monthly_installment"] != "1666667.00" || created["manager_approver_id"] != mgr ||
		len(created["collateral_attachments"].([]any)) != 1 || len(created["attachments"].([]any)) != 0 {
		t.Fatalf("pinjaman 200%% THP × 6: %v", created)
	}
}

func TestLoanAttachments(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "permanent")
	other := e.employee("Orang Lain", "", true)
	post := func(body map[string]any) map[string]any {
		return e.call(&e.staff, "POST", "/api/hris/loans", body, 400)
	}
	kasbon := func(extra map[string]any) map[string]any {
		body := map[string]any{"loan_type": "kasbon", "principal_amount": 500000, "tenor_months": 1}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}
	wantErr(t, post(kasbon(nil)), "Lampirkan minimal 1 dokumen pendukung kasbon")
	wantErr(t, post(map[string]any{"loan_type": "loan", "principal_amount": 500000, "tenor_months": 1,
		"attachments": []string{e.loanDoc(worker)}}), "Lampirkan minimal 1 dokumen jaminan (surat berharga)")
	wantErr(t, post(kasbon(map[string]any{"attachments": []string{e.loanDoc(worker)},
		"collateral_attachments": []string{e.loanDoc(worker)}})), "Jaminan hanya untuk Pinjaman Karyawan")
	invalid := "Lampiran pinjaman tidak valid — unggah ulang filenya"
	wantErr(t, post(kasbon(map[string]any{"attachments": []string{e.loanDoc(other)}})), invalid)
	wantErr(t, post(kasbon(map[string]any{"attachments": []string{"loan-attachments/" + worker + "/1-0123456789abcdef.pdf"}})), invalid)
	wantErr(t, post(kasbon(map[string]any{"attachments": []string{"../" + e.loanDoc(worker)}})), invalid)
	doc := e.loanDoc(worker)
	wantErr(t, post(kasbon(map[string]any{"attachments": []string{doc, doc}})), "Lampiran tidak boleh sama")
	wantErr(t, post(kasbon(map[string]any{"attachments": []string{e.loanDoc(worker), e.loanDoc(worker), e.loanDoc(worker),
		e.loanDoc(worker)}})), "Maksimal 3 file per lampiran")
	e.call(&e.staff, "POST", "/api/hris/loans", kasbon(map[string]any{"attachments": []string{doc}}), 200)
}

func (e *env) upload(s *testutil.Staff, employeeID string, data []byte) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if employeeID != "" {
		_ = mw.WriteField("employee_id", employeeID)
	}
	fw, _ := mw.CreateFormFile("file", "jaminan.pdf")
	_, _ = fw.Write(data)
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/hris/loans/attachments", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r = testutil.AsStaff(r, *s)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	return rec
}

func TestLoanAttachmentUploadAndRead(t *testing.T) {
	e := setup(t)
	worker := kasbonWorker(e, "permanent")
	hrEmp := e.employee("HR", e.hr.UserID, true)
	mgrStaff, _ := e.loanManager(worker)
	stranger := e.plainStaff()
	e.employee("Rekan", stranger.UserID, true)

	pathOf := func(rec *httptest.ResponseRecorder) string {
		t.Helper()
		if rec.Code != 200 {
			t.Fatalf("upload = %d %s", rec.Code, rec.Body)
		}
		_, after, _ := strings.Cut(rec.Body.String(), `"path":"`)
		p, _, _ := strings.Cut(after, `"`)
		return p
	}
	pdf := []byte("%PDF-1.4 jaminan")
	own := pathOf(e.upload(&e.staff, "", pdf))
	if !strings.HasPrefix(own, "loan-attachments/"+worker+"/") {
		t.Fatalf("path karyawan = %s", own)
	}
	// Non-HR tidak bisa mengunggah ke folder orang lain: employee_id diabaikan.
	if p := pathOf(e.upload(&e.staff, hrEmp, pdf)); !strings.HasPrefix(p, "loan-attachments/"+worker+"/") {
		t.Fatalf("employee_id dari non-HR dipakai: %s", p)
	}
	if p := pathOf(e.upload(&e.hr, strings.ToUpper(worker), pdf)); !strings.HasPrefix(p, "loan-attachments/"+worker+"/") {
		t.Fatalf("HR untuk karyawan = %s", p)
	}
	if rec := e.upload(&e.staff, "", []byte("bukan dokumen")); rec.Code != 400 {
		t.Fatalf("file bukan PDF/gambar = %d", rec.Code)
	}

	loan := e.loan(&e.staff, map[string]any{"loan_type": "loan", "principal_amount": 1000000, "tenor_months": 2,
		"collateral_attachments": []string{own}}, 200)["data"].(map[string]any)
	read := func(s *testutil.Staff, want int) {
		t.Helper()
		if rec := e.raw(s, "GET", "/api/hris/loans/attachment/"+own, nil); rec.Code != want {
			t.Fatalf("baca lampiran = %d, want %d (%s)", rec.Code, want, rec.Body)
		}
	}
	read(&e.staff, 200)
	read(&mgrStaff, 200)
	read(&e.hr, 200)
	read(&stranger, 403)
	if rec := e.raw(&e.hr, "GET", "/api/hris/loans/attachment/loan-attachments/%2E%2E/"+worker+"/x.pdf", nil); rec.Code != 400 {
		t.Fatalf("path traversal = %d", rec.Code)
	}

	// Antrean Manager memuat pinjaman ini beserta jaminannya.
	queue := e.call(&mgrStaff, "GET", "/api/hris/loans?scope=approvals", nil, 200)["data"].([]any)
	if len(queue) != 1 || queue[0].(map[string]any)["id"] != loan["id"] ||
		queue[0].(map[string]any)["collateral_attachments"].([]any)[0] != own {
		t.Fatalf("antrean manager = %v", queue)
	}
}

func TestLoanTwoStepForPinjamanKaryawan(t *testing.T) {
	e := setup(t)
	e.employee("HR", e.hr.UserID, true)
	worker := kasbonWorker(e, "contract")
	mgrStaff, mgr := e.loanManager(worker)
	id := e.loan(&e.staff, map[string]any{"loan_type": "loan", "principal_amount": 3000000, "tenor_months": 3}, 200)["data"].(map[string]any)["id"].(string)
	// EPIC-092 R5 (menggantikan EPIC-076 R7): HR yang menyetujui saat tahap
	// Manager berjalan bertindak mewakili Manager — diuji di
	// TestLoanHROnBehalfOfStages; di sini Manager sendiri yang memutus.
	step1 := e.call(&mgrStaff, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 200)
	if step1["message"] != "Pinjaman Karyawan disetujui Manager — menunggu persetujuan HR" ||
		step1["data"].(map[string]any)["manager_approved_by"] != mgr {
		t.Fatalf("tahap 1: %v", step1)
	}
	step2 := e.call(&e.hr, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 200)
	if step2["data"].(map[string]any)["status"] != "approved" {
		t.Fatalf("tahap 2: %v", step2)
	}
}

func TestLoanFirstInstallmentFollowsSubmission(t *testing.T) {
	for _, c := range []struct {
		name, createdAt, run string
		month, year          float64
		message              string
	}{
		{"20 Nov 23.30 WIB", "2026-11-20T16:30:00Z", "", 11, 2026, "Kasbon disetujui — dipotong mulai payroll November 2026"},
		{"21 Nov 00.05 WIB", "2026-11-20T17:05:00Z", "", 12, 2026, "Kasbon disetujui — dipotong mulai payroll Desember 2026"},
		{"21 Des", "2026-12-21T02:00:00Z", "", 1, 2027, "Kasbon disetujui — dipotong mulai payroll Januari 2027"},
		{"run November selesai", "2026-11-10T02:00:00Z", "completed", 12, 2026, "Kasbon disetujui — dipotong mulai payroll Desember 2026"},
		{"run November dibayar", "2026-11-10T02:00:00Z", "paid", 12, 2026, "Kasbon disetujui — dipotong mulai payroll Desember 2026"},
		{"run November diproses", "2026-11-10T02:00:00Z", "processing", 12, 2026, "Kasbon disetujui — dipotong mulai payroll Desember 2026"},
		{"run November draft", "2026-11-10T02:00:00Z", "draft", 11, 2026, "Kasbon disetujui — dipotong mulai payroll November 2026"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := setup(t)
			e.employee("HR", e.hr.UserID, true)
			kasbonWorker(e, "permanent")
			id := e.loan(&e.staff, map[string]any{"loan_type": "kasbon", "principal_amount": 1000000, "tenor_months": 2}, 200)["data"].(map[string]any)["id"].(string)
			e.exec(`UPDATE hris.loans SET created_at = $2 WHERE id = $1`, id, c.createdAt)
			if c.run != "" {
				e.exec(`INSERT INTO hris.payroll_runs (run_name, period_month, period_year, status)
					VALUES ('Uji November', 11, 2026, $1)`, c.run)
			}
			ok := e.call(&e.hr, "POST", "/api/hris/loans/"+id+"/approve", map[string]any{"approved": true}, 200)
			d := ok["data"].(map[string]any)
			if d["first_installment_month"] != c.month || d["first_installment_year"] != c.year || ok["message"] != c.message {
				t.Fatalf("got %v/%v %q", d["first_installment_month"], d["first_installment_year"], ok["message"])
			}
		})
	}
}

// Pending requests filed before EPIC-076 are re-checked against the new terms
// when HR decides, but not asked for attachments (R12).
func TestLoanApproveRechecksTnC(t *testing.T) {
	e := setup(t)
	e.employee("HR", e.hr.UserID, true)
	worker := kasbonWorker(e, "permanent")
	var long, short string
	e.scalar(&long, `INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months, monthly_installment,
		remaining_balance, status) VALUES ($1, 'loan', 1200000, 12, 100000, 1200000, 'pending') RETURNING id::text`, worker)
	wantErr(t, e.call(&e.hr, "POST", "/api/hris/loans/"+long+"/approve", map[string]any{"approved": true}, 400),
		"Cicilan Pinjaman Karyawan 1 sampai 6 kali")
	e.call(&e.hr, "POST", "/api/hris/loans/"+long+"/approve", map[string]any{"approved": false}, 200)
	e.scalar(&short, `INSERT INTO hris.loans (employee_id, loan_type, principal_amount, tenor_months, monthly_installment,
		remaining_balance, status) VALUES ($1, 'loan', 1000000, 2, 500000, 1000000, 'pending') RETURNING id::text`, worker)
	e.call(&e.hr, "POST", "/api/hris/loans/"+short+"/approve", map[string]any{"approved": true}, 200)
}
