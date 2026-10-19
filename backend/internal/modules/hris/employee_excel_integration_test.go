package hris

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

func getFile(t *testing.T, h *harness, s *testutil.Staff, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := testutil.AsStaff(httptest.NewRequest(http.MethodGet, path, nil), *s)
	rec := httptest.NewRecorder()
	h.exec("SAVEPOINT harness_request")
	h.mux.ServeHTTP(rec, r)
	if _, err := h.tx.Exec(context.Background(), "RELEASE SAVEPOINT harness_request"); err != nil {
		h.exec("ROLLBACK TO SAVEPOINT harness_request")
		h.exec("RELEASE SAVEPOINT harness_request")
	}
	return rec
}

func TestEmployeeExport(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	e := h.employee(emp{name: "Ekspor Satu", status: "contract", join: "2025-06-01"})
	h.exec(`UPDATE hris.employees SET nip = '0202505272', ktp = '3273012345678901', gender = 'female', marital_status = 'single', schedule_type = 'non_shift' WHERE id = $1`, e)
	rec := getFile(t, h, &hr, "/api/hris/employees/export?search=Ekspor")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.String())
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := f.GetRows("Data Karyawan")
	if rows[0][1] != "Nama Lengkap *" || rows[1][0] != "0202505272" || rows[1][1] != "Ekspor Satu" {
		t.Fatalf("rows = %v", rows[:2])
	}
	get := func(header string) string {
		for i, h := range rows[0] {
			if h == header || h == header+" *" {
				return rows[1][i]
			}
		}
		return "<missing>"
	}
	checks := map[string]string{"Tanggal Masuk": "01/06/2025", "Status Kepegawaian": "Kontrak (PKWT)", "Tipe Jadwal": "Non-Shift",
		"Jenis Kelamin": "Perempuan", "Status Pernikahan": "Belum Menikah", "NIK (KTP)": "3273012345678901", "Status Aktif": "Aktif"}
	for header, want := range checks {
		if got := get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if list := f.GetSheetList(); list[0] != "Petunjuk" {
		t.Fatalf("first sheet = %v", list)
	}
}

func TestEmployeeExportImportRoundTrip(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	e := h.employee(emp{name: "Bolak Balik", status: "contract"})
	branch := h.scalar(`SELECT id::text FROM configuration.branches WHERE is_active LIMIT 1`)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2025-01-10')`, e, branch)
	h.exec(`UPDATE hris.employees SET nip = '0202500001', ktp = '3273012345678901', gender = 'male', marital_status = 'married' WHERE id = $1`, e)
	// Data nyata live: No. HP tersimpan 62…, nama jabatan kembar di dua departemen.
	h.exec(`INSERT INTO hris.positions (title, department, is_active) VALUES ('Kasir Uji', 'Dept A', true), ('Kasir Uji', 'Dept B', true)`)
	h.exec(`UPDATE hris.employees SET phone = '6289522422791',
		job_title_id = (SELECT id FROM hris.positions WHERE title = 'Kasir Uji' AND department = 'Dept B') WHERE id = $1`, e)
	rec := getFile(t, h, &hr, "/api/hris/employees/export?search=Bolak")
	pre := uploadEmployees(t, h, &hr, map[string]string{"mode": "replace", "step": "preview"}, rec.Body.Bytes())
	expect(t, pre, http.StatusOK, "")
	sum := pre.data()["summary"].(map[string]any)
	// Spec §3/§Pengujian: export → import Timpa tanpa perubahan = 0 baris diubah.
	if sum["update"] != float64(0) || sum["skip"] != float64(1) || len(pre.data()["errors"].([]any)) != 0 {
		t.Fatalf("summary %v errors %v", sum, pre.data()["errors"])
	}
	commit := uploadEmployees(t, h, &hr, map[string]string{"mode": "replace", "step": "commit"}, rec.Body.Bytes())
	expect(t, commit, http.StatusOK, "")
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity_id = $1 AND action = 'employee.import' AND after <> '{}'::jsonb`, e) != "0" {
		t.Fatal("round trip must change nothing (no audit with changes)")
	}
}

func uploadEmployees(t *testing.T, h *harness, hr *testutil.Staff, fields map[string]string, file []byte) call {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	fw, _ := w.CreateFormFile("file", "data.xlsx")
	_, _ = fw.Write(file)
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/hris/employees/import", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r = testutil.AsStaff(r, *hr)
	h.exec("SAVEPOINT harness_request")
	rec, out := testutil.Do(t, h.mux, r)
	if _, err := h.tx.Exec(context.Background(), "RELEASE SAVEPOINT harness_request"); err != nil {
		h.exec("ROLLBACK TO SAVEPOINT harness_request")
		h.exec("RELEASE SAVEPOINT harness_request")
	}
	return call{rec.Code, out, rec.Body.String()}
}

// employeeFile builds a minimal Data Karyawan workbook from a header and rows.
func employeeFile(t *testing.T, header []string, rows ...[]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Data Karyawan")
	for r, row := range append([][]string{header}, rows...) {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			_ = f.SetCellStr("Data Karyawan", cell, v)
		}
	}
	var buf bytes.Buffer
	_ = f.Write(&buf)
	return buf.Bytes()
}

func TestEmployeeImportAddAndReplace(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	branch := h.scalar(`INSERT INTO configuration.branches (company_id, name, code)
		SELECT id, 'Cabang Import', 'CI'||substr(md5(random()::text),1,4) FROM configuration.companies LIMIT 1 RETURNING id::text`)
	h.exec(`INSERT INTO hris.employment_statuses (code, name, is_active) VALUES ('contract', 'Kontrak (PKWT)', true) ON CONFLICT (code) DO NOTHING`)
	header := []string{"NIP", "Nama Lengkap *", "Email *", "Tanggal Masuk *", "Status Kepegawaian *", "Tipe Jadwal *", "Cabang Utama *", "Jenis Kelamin"}
	add := employeeFile(t, header, []string{"", "Baru Import", "baru.import@test.id", "21/10/2026", "Kontrak (PKWT)", "Shift", "Cabang Import", "Perempuan"})
	pre := uploadEmployees(t, h, &hr, map[string]string{"mode": "add", "step": "preview"}, add)
	expect(t, pre, http.StatusOK, "")
	if pre.data()["summary"].(map[string]any)["create"] != float64(1) {
		t.Fatalf("preview = %v", pre.data())
	}
	commit := uploadEmployees(t, h, &hr, map[string]string{"mode": "add", "step": "commit"}, add)
	expect(t, commit, http.StatusOK, "")
	id := h.scalar(`SELECT id::text FROM hris.employees WHERE email = 'baru.import@test.id'`)
	if got := h.scalar(`SELECT nip || '|' || gender::text || '|' || schedule_type || '|' || phone FROM hris.employees WHERE id = $1`, id); !strings.HasPrefix(got, "EMP-") || !strings.HasSuffix(got, "|female|shift|") {
		t.Fatalf("created = %s", got)
	}
	if h.scalar(`SELECT branch_id::text FROM hris.employee_placements WHERE employee_id = $1 AND is_primary`, id) != branch {
		t.Fatal("primary placement missing")
	}
	nip := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, id)
	// Timpa: nama & tipe jadwal berubah, sel kosong tidak menimpa.
	rep := employeeFile(t, header, []string{nip, "Baru Import Dua", "", "", "", "Non-Shift", "", ""})
	pre2 := uploadEmployees(t, h, &hr, map[string]string{"mode": "replace", "step": "preview"}, rep)
	changes := pre2.data()["changes"].([]any)
	if len(changes) != 2 {
		t.Fatalf("changes = %v", changes)
	}
	c2 := uploadEmployees(t, h, &hr, map[string]string{"mode": "replace", "step": "commit"}, rep)
	expect(t, c2, http.StatusOK, "")
	if got := h.scalar(`SELECT full_name || '|' || schedule_type || '|' || email FROM hris.employees WHERE id = $1`, id); got != "Baru Import Dua|non_shift|baru.import@test.id" {
		t.Fatalf("replaced = %s", got)
	}
	if h.scalar(`SELECT count(*)::text FROM hris.employee_shifts WHERE employee_id = $1`, id) != "7" {
		t.Fatal("Non-Shift via import must apply the fixed schedule")
	}
	// Mode Tambah menolak NIP yang sudah ada.
	dup := uploadEmployees(t, h, &hr, map[string]string{"mode": "add", "step": "commit"}, employeeFile(t, header,
		[]string{nip, "X", "x.import@test.id", "21/10/2026", "Kontrak (PKWT)", "Shift", "Cabang Import", ""}))
	expect(t, dup, http.StatusBadRequest, "File masih punya error — perbaiki lalu unggah ulang")
}
