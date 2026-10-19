package hris

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/platform/iam"
	"nuhabit/backend/internal/platform/testutil"
)

// fixedNow = Senin 2026-10-05 → rentang uji 2026-10-05..2026-10-11.

func scheduleFixture(h *harness) (branch, shiftEmp, other string) {
	branch = h.scalar(`INSERT INTO configuration.branches (company_id, name, code)
		SELECT id, 'Cabang Uji', 'CU'||substr(md5(random()::text),1,4) FROM configuration.companies LIMIT 1 RETURNING id::text`)
	branch2 := h.scalar(`INSERT INTO configuration.branches (company_id, name, code)
		SELECT id, 'Cabang Dua', 'CD'||substr(md5(random()::text),1,4) FROM configuration.companies LIMIT 1 RETURNING id::text`)
	shiftEmp = h.employee(emp{name: "Ana Shift"})
	other = h.employee(emp{name: "Budi Tambahan"})
	ns := h.employee(emp{name: "Cici Kantor"})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, ns)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES
		($1, $4, true, '2026-01-01'),
		($2, $5, true, '2026-01-01'), ($2, $4, false, '2026-10-08'),
		($3, $4, true, '2026-01-01')`, shiftEmp, other, ns, branch, branch2)
	return
}

func upload(t *testing.T, h *harness, hr *testutil.Staff, fields map[string]string, file []byte) call {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	fw, _ := w.CreateFormFile("file", "jadwal.xlsx")
	_, _ = fw.Write(file)
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/hris/schedules/import", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r = testutil.AsStaff(r, *hr)
	h.exec("SAVEPOINT harness_request")
	rec, out := testutil.Do(t, h.mux, r)
	h.exec("RELEASE SAVEPOINT harness_request")
	return call{rec.Code, out, rec.Body.String()}
}

func TestScheduleTemplateIncludesSecondaryPlacement(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	branch, _, _ := scheduleFixture(h)
	grid := h.do(&hr, http.MethodGet, "/api/hris/schedules?branch_id="+branch+"&from=2026-10-05&to=2026-10-11", nil)
	expect(t, grid, http.StatusOK, "")
	emps := grid.data()["employees"].([]any)
	if len(emps) != 2 || grid.data()["non_shift_count"] != float64(1) {
		t.Fatalf("employees %d non_shift %v (want Ana + Budi, Cici excluded)", len(emps), grid.data()["non_shift_count"])
	}
	for _, e := range emps {
		m := e.(map[string]any)
		if m["name"] == "Budi Tambahan" {
			cells := m["cells"].([]any)
			if cells[0].(map[string]any)["code"] != "-" || cells[3].(map[string]any)["code"] != "" {
				t.Fatalf("Budi cells = %v (not placed until 10-08)", cells)
			}
		}
	}
}

func TestScheduleUploadPreviewCommit(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	branch, ana, _ := scheduleFixture(h)
	params := "?branch_id=" + branch + "&from=2026-10-05&to=2026-10-11"
	r := testutil.AsStaff(httptest.NewRequest(http.MethodGet, "/api/hris/schedules/template"+params, nil), hr)
	rec := httptest.NewRecorder()
	h.exec("SAVEPOINT harness_request")
	h.mux.ServeHTTP(rec, r)
	h.exec("RELEASE SAVEPOINT harness_request")
	if rec.Code != http.StatusOK {
		t.Fatalf("template status %d %s", rec.Code, rec.Body.String())
	}
	f, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := f.GetCellValue("Jadwal", "D3"); v != "Sen 05/10" {
		t.Fatalf("D3 = %q", v)
	}
	// Ana: SPG? → pakai DEF Sen–Jum, L Sab–Min. Budi: '-' sampai 07/10 lalu DEF.
	// Data mulai baris Excel 4 (2 baris judul + header). Sen–Jum DEF, Sab–Min L; sel "-" dibiarkan.
	rows, _ := f.GetRows("Jadwal")
	for i := 3; i < len(rows); i++ {
		for j, v := range []string{"DEF", "DEF", "DEF", "DEF", "DEF", "L", "L"} {
			cell, _ := excelize.CoordinatesToCellName(4+j, i+1)
			if cur, _ := f.GetCellValue("Jadwal", cell); cur != "-" {
				_ = f.SetCellStr("Jadwal", cell, v)
			}
		}
	}
	var buf bytes.Buffer
	_ = f.Write(&buf)
	file := buf.Bytes()

	pre := upload(t, h, &hr, map[string]string{"branch_id": branch, "from": "2026-10-05", "to": "2026-10-11", "step": "preview"}, file)
	expect(t, pre, http.StatusOK, "")
	if errs := pre.data()["errors"].([]any); len(errs) != 0 {
		t.Fatalf("errors %v", errs)
	}
	fp := pre.data()["fingerprint"].(string)
	noAck := upload(t, h, &hr, map[string]string{"branch_id": branch, "from": "2026-10-05", "to": "2026-10-11", "step": "commit", "fingerprint": fp}, file)
	expect(t, noAck, http.StatusBadRequest, "Centang \"Saya sudah memeriksa peringatan\" sebelum menyimpan")
	ok := upload(t, h, &hr, map[string]string{"branch_id": branch, "from": "2026-10-05", "to": "2026-10-11", "step": "commit", "fingerprint": fp, "ack_warnings": "true"}, file)
	expect(t, ok, http.StatusOK, "")
	if h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1`, ana) != "7" {
		t.Fatal("Ana must have 7 per-date rows")
	}
	stale := upload(t, h, &hr, map[string]string{"branch_id": branch, "from": "2026-10-05", "to": "2026-10-11", "step": "commit", "fingerprint": fp, "ack_warnings": "true"}, file)
	expect(t, stale, http.StatusConflict, "Jadwal sudah diubah orang lain sejak preview — unggah ulang file")
	if h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'schedule.bulk_upload'`) == "0" {
		t.Fatal("audit missing")
	}
}

func TestScheduleUploadReportsReplacedCellsAndErrorFile(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	branch, ana, _ := scheduleFixture(h)
	nip := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, ana)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	// Jadwal lama Ana: DEF tiap hari.
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		SELECT $1, d::date, $2, gen_random_uuid() FROM generate_series('2026-10-05'::date, '2026-10-11', interval '1 day') d`, ana, def)
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Jadwal")
	head := []string{"NIP", "Nama Karyawan", "Jabatan", "Sen 05/10", "Sel 06/10", "Rab 07/10", "Kam 08/10", "Jum 09/10", "Sab 10/10", "Min 11/10"}
	for i, v := range head {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = f.SetCellStr("Jadwal", cell, v)
	}
	for i, v := range append([]string{nip, "Ana Shift", ""}, "DEF", "L", "DEF", "DEF", "DEF", "L", "DEF") {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		_ = f.SetCellStr("Jadwal", cell, v)
	}
	// Baris kedua: NIP tidak dikenal → error baris, tidak menahan diff baris Ana.
	for i, v := range []string{"999999", "Orang Lain", "", "DEF", "DEF", "DEF", "DEF", "DEF", "L", "L"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 5)
		_ = f.SetCellStr("Jadwal", cell, v)
	}
	_, _ = f.NewSheet("Info")
	for i, kv := range [][2]string{{"Cabang ID", branch}, {"Dari", "2026-10-05"}, {"Sampai", "2026-10-11"}} {
		_ = f.SetCellStr("Info", "A"+strconv.Itoa(i+1), kv[0])
		_ = f.SetCellStr("Info", "B"+strconv.Itoa(i+1), kv[1])
	}
	var buf bytes.Buffer
	_ = f.Write(&buf)
	fields := map[string]string{"branch_id": branch, "from": "2026-10-05", "to": "2026-10-11", "step": "preview"}
	pre := upload(t, h, &hr, fields, buf.Bytes())
	expect(t, pre, http.StatusOK, "")
	sum := pre.data()["summary"].(map[string]any)
	if sum["changed"] != float64(2) || sum["same"] != float64(5) {
		t.Fatalf("summary = %v (06/10 & 10/10 DEF→L)", sum)
	}
	if sum["employees"] != float64(1) {
		t.Fatalf("employees = %v", sum["employees"])
	}
	ch := pre.data()["changes"].([]any)[0].(map[string]any)
	if ch["before"] != "DEF" || ch["after"] != "L" || ch["date"] != "2026-10-06" {
		t.Fatalf("change = %v", ch)
	}
	if errs := pre.data()["errors"].([]any); len(errs) != 1 {
		t.Fatalf("errors = %v (NIP 999999)", errs)
	}
	fields["step"] = "error_file"
	ef := upload(t, h, &hr, fields, buf.Bytes())
	if ef.status != http.StatusOK || !strings.Contains(ef.raw, "PK") {
		t.Fatalf("error file status %d", ef.status)
	}
}

// Review C1: upload cabang A tidak boleh menghapus jadwal per tanggal
// karyawan dua cabang pada tanggal ia ditempatkan di cabang lain.
func TestScheduleUploadKeepsOtherBranchDays(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", iam.HrisKepegawaian, false)
	branch, _, budi := scheduleFixture(h)
	def := h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	// Jadwal Budi dari cabang utamanya (branch2) untuk 05–07/10.
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id)
		SELECT $1, d::date, $2, gen_random_uuid() FROM generate_series('2026-10-05'::date, '2026-10-07', interval '1 day') d`, budi, def)
	nip := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, budi)
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Jadwal")
	head := []string{"NIP", "Nama Karyawan", "Jabatan", "Sen 05/10", "Sel 06/10", "Rab 07/10", "Kam 08/10", "Jum 09/10", "Sab 10/10", "Min 11/10"}
	row := []string{nip, "Budi", "", "-", "-", "-", "DEF", "DEF", "L", "L"}
	for i := range head {
		c1, _ := excelize.CoordinatesToCellName(i+1, 3)
		c2, _ := excelize.CoordinatesToCellName(i+1, 4)
		_ = f.SetCellStr("Jadwal", c1, head[i])
		_ = f.SetCellStr("Jadwal", c2, row[i])
	}
	_, _ = f.NewSheet("Info")
	for i, kv := range [][2]string{{"Cabang ID", branch}, {"Dari", "2026-10-05"}, {"Sampai", "2026-10-11"}} {
		_ = f.SetCellStr("Info", "A"+strconv.Itoa(i+1), kv[0])
		_ = f.SetCellStr("Info", "B"+strconv.Itoa(i+1), kv[1])
	}
	var buf bytes.Buffer
	_ = f.Write(&buf)
	fields := map[string]string{"branch_id": branch, "from": "2026-10-05", "to": "2026-10-11", "step": "preview"}
	pre := upload(t, h, &hr, fields, buf.Bytes())
	expect(t, pre, http.StatusOK, "")
	fields["step"], fields["fingerprint"], fields["ack_warnings"] = "commit", pre.data()["fingerprint"].(string), "true"
	expect(t, upload(t, h, &hr, fields, buf.Bytes()), http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1 AND work_date <= '2026-10-07'`, budi); got != "3" {
		t.Fatalf("Budi's other-branch days = %s, want 3 kept", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1 AND work_date >= '2026-10-08'`, budi); got != "4" {
		t.Fatalf("Budi's uploaded days = %s, want 4", got)
	}
}
