package hris

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
	"nuhabit/backend/internal/platform/xlsx"
)

// uploadBranchCodes posts one step of the Kode & Status Toko import.
func uploadBranchCodes(t *testing.T, h *harness, s *testutil.Staff, fields map[string]string, file []byte) (call, *httptest.ResponseRecorder) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	fw, _ := w.CreateFormFile("file", "toko.xlsx")
	_, _ = fw.Write(file)
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/hris/branches/import", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r = testutil.AsStaff(r, *s)
	h.exec("SAVEPOINT harness_request")
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, r)
	if _, err := h.tx.Exec(context.Background(), "RELEASE SAVEPOINT harness_request"); err != nil {
		h.exec("ROLLBACK TO SAVEPOINT harness_request")
		h.exec("RELEASE SAVEPOINT harness_request")
	}
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return call{rec.Code, out, rec.Body.String()}, rec
}

// branchCodeFile builds a Data Toko workbook: rows of Kode Saat Ini, Kode
// Baru, Nama Baru, Status Baru.
func branchCodeFile(t *testing.T, rows ...[4]string) []byte {
	t.Helper()
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", domain.BranchCodeSheet)
	_ = f.SetSheetRow(domain.BranchCodeSheet, "A1", &domain.BranchCodeHeaders)
	for i, r := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		row := []string{r[0], "", "", "", r[1], r[2], r[3]}
		_ = f.SetSheetRow(domain.BranchCodeSheet, cell, &row)
	}
	var buf bytes.Buffer
	_ = f.Write(&buf)
	return buf.Bytes()
}

func TestBranchCodeImport(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisPlacements, false)
	sfx := strings.ToUpper(testutil.RandomHex(3))
	newBranch := func(code, name string) string {
		return h.scalar(`INSERT INTO configuration.branches (company_id, name, code)
			SELECT id, $1, $2 FROM configuration.companies ORDER BY created_at LIMIT 1 RETURNING id::text`, name, code)
	}
	a, b, c := "KA"+sfx, "KB"+sfx, "KC"+sfx
	aID := newBranch(a, a+" - Toko A")
	bID := newBranch(b, b+" - Toko B")
	cID := newBranch(c, "Toko C")
	emp := h.employee(emp{name: "Penghuni Toko A"})
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, start_date, is_primary) VALUES ($1, $2, '2026-01-01', true)`, emp, aID)
	h.exec(`INSERT INTO hris.attendance (employee_id, branch_id, date, status) VALUES ($1, $2, '2026-01-05', 'present')`, emp, cID)

	// (a) template lists every branch with its code.
	tpl := h.send(&hr, httptest.NewRequest("GET", "/api/hris/branches/template", nil))
	if tpl.Code != 200 || tpl.Header().Get("Content-Type") != xlsx.ContentType {
		t.Fatalf("template = %d %s", tpl.Code, tpl.Body.String())
	}
	matrix, err := xlsx.ParseMatrix(tpl.Body.Bytes(), xlsx.ReadOptions{PreferSheet: domain.BranchCodeSheet})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := json.Marshal(matrix)
	for _, want := range []string{a, b, c, "Kode Saat Ini", "Status Baru"} {
		if !strings.Contains(string(text), want) {
			t.Fatalf("template misses %q: %s", want, text)
		}
	}

	// (b) preview writes nothing.
	file := branchCodeFile(t, [4]string{a, a + "2", "", ""}, [4]string{b, "", "", "Nonaktif"}, [4]string{c, c + "2", "Toko C Baru", ""})
	pre, _ := uploadBranchCodes(t, h, &hr, map[string]string{"step": "preview"}, file)
	expect(t, pre, 200, "")
	d := pre.data()
	if len(d["errors"].([]any)) != 0 || len(d["warnings"].([]any)) != 2 || d["summary"].(map[string]any)["changed"] != float64(3) {
		t.Fatalf("preview = %s", pre.raw)
	}
	if h.scalar(`SELECT code FROM configuration.branches WHERE id = $1`, aID) != a {
		t.Fatal("preview must not write")
	}
	fp := d["fingerprint"].(string)

	// (c) warnings need the acknowledgement.
	nack, _ := uploadBranchCodes(t, h, &hr, map[string]string{"step": "commit", "fingerprint": fp}, file)
	expect(t, nack, 400, `Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`)

	// (d) commit applies all three, history stays on the same rows.
	done, _ := uploadBranchCodes(t, h, &hr, map[string]string{"step": "commit", "fingerprint": fp, "ack_warnings": "true"}, file)
	expect(t, done, 200, "")
	if done.body["message"] != "3 toko diperbarui" {
		t.Fatalf("commit = %s", done.raw)
	}
	if got := h.scalar(`SELECT code || '|' || name FROM configuration.branches WHERE id = $1`, aID); got != a+"2|"+a+"2 - Toko A" {
		t.Fatalf("A = %s", got)
	}
	if h.scalar(`SELECT is_active::text FROM configuration.branches WHERE id = $1`, bID) != "false" {
		t.Fatal("B must be inactive")
	}
	if got := h.scalar(`SELECT code || '|' || name FROM configuration.branches WHERE id = $1`, cID); got != c+"2|Toko C Baru" {
		t.Fatalf("C = %s", got)
	}
	if h.scalar(`SELECT count(*)::text FROM hris.attendance WHERE branch_id = $1`, cID) != "1" {
		t.Fatal("attendance must keep pointing at the branch")
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'branch.update' AND entity_id::text IN ($1, $2, $3)`, aID, bID, cID); n != "3" {
		t.Fatalf("audit rows = %s", n)
	}

	// (e) a branch in use is not deactivated.
	busy, _ := uploadBranchCodes(t, h, &hr, map[string]string{"step": "preview"}, branchCodeFile(t, [4]string{a + "2", "", "", "Nonaktif"}))
	expect(t, busy, 200, "")
	if !strings.Contains(busy.raw, "masih dipakai: 1 karyawan aktif (Penghuni Toko A)") {
		t.Fatalf("busy = %s", busy.raw)
	}

	// (f) a stale fingerprint is refused.
	again := branchCodeFile(t, [4]string{b, "", "", "Aktif"})
	pre2, _ := uploadBranchCodes(t, h, &hr, map[string]string{"step": "preview"}, again)
	h.exec(`UPDATE configuration.branches SET name = 'Diubah orang lain' WHERE id = $1`, bID)
	stale, _ := uploadBranchCodes(t, h, &hr, map[string]string{"step": "commit", "fingerprint": pre2.data()["fingerprint"].(string)}, again)
	expect(t, stale, 409, "Data toko sudah diubah sejak preview — unggah ulang file")

	// (g) the error file carries the messages.
	_, rec := uploadBranchCodes(t, h, &hr, map[string]string{"step": "error_file"}, branchCodeFile(t, [4]string{"TIDAKADA" + sfx, "", "", ""}))
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Disposition"), "Error") {
		t.Fatalf("error file = %d %v", rec.Code, rec.Header())
	}
	em, _ := xlsx.ParseMatrix(rec.Body.Bytes(), xlsx.ReadOptions{PreferSheet: domain.BranchCodeSheet})
	if et, _ := json.Marshal(em); !strings.Contains(string(et), "Keterangan Error") || !strings.Contains(string(et), "tidak ditemukan") {
		t.Fatalf("error file body = %s", et)
	}
}
