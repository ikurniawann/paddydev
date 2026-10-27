package hris

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-068 Task 3: the leave-balance Excel template and import. fixedNow is
// 2026-10-05 WIB, so the editable years are 2026 and 2027.

const (
	saldoSheet = "Saldo Cuti"
	// Template columns (R10): A NIP, B Nama, C Cabang, D Terpakai, E Jatah,
	// F Penyesuaian, G Sisa Bawaan, H Kedaluwarsa, I Alasan.
	colTotal, colAdjust, colCarry, colExpires, colReason = "E", "F", "G", "H", "I"
)

type saldoImportFixture struct {
	hr                       testutil.Staff
	branch, other            string
	ana, budi, cici, dodi    string // ana: row 12/5 used; budi: no row (default 9,5); cici: inactive; dodi: other branch
	anaNIP, budiNIP, dodiNIP string
}

func (h *harness) saldoImportFixture() saldoImportFixture {
	h.t.Helper()
	hr, _ := h.saldoStaff("pos", false, "read", "import", "export")
	f := saldoImportFixture{hr: hr, branch: h.branch("Toko Impor Saldo"), other: h.branch("Toko Impor Lain")}
	f.ana = h.employee(emp{name: "Ana Impor", join: "2024-01-10"})
	f.budi = h.employee(emp{name: "Budi Impor", join: "2025-03-10"})
	f.cici = h.employee(emp{name: "Cici Impor", join: "2024-01-10", inactive: true})
	f.dodi = h.employee(emp{name: "Dodi Impor", join: "2024-01-10"})
	h.setBalance(f.ana, 2026, 12, 0, 5, 0, 0, nil)
	h.exec(`DELETE FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.budi)
	for _, p := range [][2]string{{f.ana, f.branch}, {f.budi, f.branch}, {f.cici, f.branch}, {f.dodi, f.other}} {
		h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2026-01-01')`, p[0], p[1])
	}
	f.anaNIP = h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, f.ana)
	f.budiNIP = h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, f.budi)
	f.dodiNIP = h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, f.dodi)
	return f
}

func (h *harness) saldoTemplate(hr *testutil.Staff, query string) (*httptest.ResponseRecorder, *excelize.File) {
	h.t.Helper()
	rec := h.send(hr, httptest.NewRequest(http.MethodGet, "/api/hris/leave-balances/template"+query, nil))
	if rec.Code != http.StatusOK {
		h.t.Fatalf("template status %d %s", rec.Code, rec.Body.String())
	}
	x, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		h.t.Fatal(err)
	}
	return rec, x
}

// rowOf finds the Excel row of nip in the Saldo Cuti sheet (0 = absent).
func rowOf(t *testing.T, x *excelize.File, nip string) int {
	t.Helper()
	rows, err := x.GetRows(saldoSheet)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range rows {
		if len(r) > 0 && r[0] == nip {
			return i + 1
		}
	}
	return 0
}

func fileBytes(t *testing.T, x *excelize.File) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := x.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func setCell(t *testing.T, x *excelize.File, col string, row int, v any) {
	t.Helper()
	cell, _ := excelize.CoordinatesToCellName(int(col[0]-'A')+1, row)
	var err error
	if s, ok := v.(string); ok {
		err = x.SetCellStr(saldoSheet, cell, s)
	} else {
		err = x.SetCellValue(saldoSheet, cell, v)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func (h *harness) saldoUpload(t *testing.T, s *testutil.Staff, name string, fields map[string]string, file []byte) (*httptest.ResponseRecorder, call) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	fw, _ := w.CreateFormFile("file", name)
	_, _ = fw.Write(file)
	_ = w.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/hris/leave-balances/import", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	rec := h.send(s, r)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, call{rec.Code, out, rec.Body.String()}
}

func (h *harness) importStep(t *testing.T, f saldoImportFixture, step string, fields map[string]string, file []byte) call {
	t.Helper()
	m := map[string]string{"year": "2026", "branch_id": f.branch, "step": step}
	for k, v := range fields {
		m[k] = v
	}
	_, c := h.saldoUpload(t, &f.hr, "saldo-cuti.xlsx", m, file)
	return c
}

func issueMessages(c call, key string) string {
	var out []string
	for _, it := range c.data()[key].([]any) {
		m := it.(map[string]any)
		out = append(out, m["column"].(string)+": "+m["message"].(string)+" @"+itoa(int(m["line"].(float64))))
	}
	return strings.Join(out, "\n")
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestLeaveBalanceTemplate(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	noExport, _ := h.saldoStaff("pos", false, "read", "import")

	if rec := h.send(&noExport, httptest.NewRequest(http.MethodGet, "/api/hris/leave-balances/template", nil)); rec.Code != 403 {
		t.Fatalf("without export: %d", rec.Code)
	}
	if rec := h.send(nil, httptest.NewRequest(http.MethodGet, "/api/hris/leave-balances/template", nil)); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	for q, msg := range map[string]string{"?branch_id=bukan-uuid": "", "?branch_id=00000000-0000-0000-0000-000000000000": "Cabang tidak ditemukan", "?year=abc": "Tahun tidak valid"} {
		rec := h.send(&f.hr, httptest.NewRequest(http.MethodGet, "/api/hris/leave-balances/template"+q, nil))
		if rec.Code != 400 || (msg != "" && !strings.Contains(rec.Body.String(), msg)) {
			t.Fatalf("%s: %d %s", q, rec.Code, rec.Body.String())
		}
	}

	// One branch: its active employees only, with the stored or default figures.
	rec, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("content type %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "Saldo Cuti") || !strings.Contains(cd, "2026") || !strings.Contains(cd, ".xlsx") {
		t.Fatalf("content disposition %q", cd)
	}
	sheets := x.GetSheetList()
	if len(sheets) < 3 || sheets[0] != "Petunjuk" || sheets[1] != saldoSheet || sheets[len(sheets)-1] != "Info" {
		t.Fatalf("sheets = %v", sheets)
	}
	if vis, _ := x.GetSheetVisible("Info"); vis {
		t.Fatal("Info sheet must be hidden")
	}
	info, _ := x.GetRows("Info")
	if len(info) < 2 || info[0][0] != "Tahun" || info[0][1] != "2026" || info[1][0] != "Cabang ID" || info[1][1] != f.branch {
		t.Fatalf("info = %v", info)
	}
	rows, _ := x.GetRows(saldoSheet)
	var header []string
	for _, r := range rows {
		if len(r) > 0 && r[0] == "NIP" {
			header = r
		}
	}
	want := []string{"NIP", "Nama", "Cabang", "Terpakai", "Jatah", "Penyesuaian", "Sisa Bawaan", "Kedaluwarsa", "Alasan"}
	if strings.Join(header, "|") != strings.Join(want, "|") {
		t.Fatalf("header = %v", header)
	}
	if rowOf(t, x, f.anaNIP) == 0 || rowOf(t, x, f.budiNIP) == 0 || rowOf(t, x, f.dodiNIP) != 0 {
		t.Fatalf("branch rows wrong: %v", rows)
	}
	get := func(col string, row int) string {
		cell, _ := excelize.CoordinatesToCellName(int(col[0]-'A')+1, row)
		v, _ := x.GetCellValue(saldoSheet, cell)
		return v
	}
	ar, br := rowOf(t, x, f.anaNIP), rowOf(t, x, f.budiNIP)
	if get("B", ar) != "Ana Impor" || get("C", ar) != "Toko Impor Saldo" || get("D", ar) != "5" || get(colTotal, ar) != "12" || get(colAdjust, ar) != "0" || get(colCarry, ar) != "0" {
		t.Fatalf("ana row = %v", rows[ar-1])
	}
	// Budi has no stored row: the template shows the R1 default (9,5 for join 2025-03-10).
	if get(colTotal, br) != "9.5" || get("D", br) != "0" {
		t.Fatalf("budi row = %v", rows[br-1])
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.budi); n != "0" {
		t.Fatalf("the template wrote %s rows", n)
	}

	// All branches: the info sheet carries an empty branch, everyone active is listed.
	_, all := h.saldoTemplate(&f.hr, "?year=2026")
	allInfo, _ := all.GetRows("Info")
	if len(allInfo) < 2 || allInfo[1][0] != "Cabang ID" || (len(allInfo[1]) > 1 && allInfo[1][1] != "") {
		t.Fatalf("all-branch info = %v", allInfo)
	}
	if rowOf(t, all, f.anaNIP) == 0 || rowOf(t, all, f.dodiNIP) == 0 {
		t.Fatal("all-branch template must list every active employee")
	}
	if rowOf(t, all, h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, f.cici)) != 0 {
		t.Fatal("inactive employees must not be listed")
	}
}

func TestLeaveBalanceImportPreviewCommit(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	ar, br := rowOf(t, x, f.anaNIP), rowOf(t, x, f.budiNIP)
	setCell(t, x, colTotal, ar, 14)
	setCell(t, x, colAdjust, ar, 1)
	setCell(t, x, colCarry, ar, 2)
	setCell(t, x, colExpires, ar, time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC))
	setCell(t, x, colReason, ar, "Koreksi jatah")
	setCell(t, x, colTotal, br, "10,5") // typed with a decimal comma
	setCell(t, x, colReason, br, "Jatah khusus")
	file := fileBytes(t, x)

	pre := h.importStep(t, f, "preview", nil, file)
	expect(t, pre, 200, "")
	d := pre.data()
	sum := d["summary"].(map[string]any)
	if sum["employees"] != 2.0 || sum["new"] != 1.0 || sum["changed"] != 1.0 || sum["same"] != 0.0 {
		t.Fatalf("summary = %v", sum)
	}
	if len(d["errors"].([]any)) != 0 {
		t.Fatalf("errors = %v", d["errors"])
	}
	// The expiry (carry > 0) and total <= 24 raise no warning here.
	if len(d["warnings"].([]any)) != 0 {
		t.Fatalf("warnings = %v", d["warnings"])
	}
	changes := d["changes"].([]any)
	if len(changes) != 2 {
		t.Fatalf("changes = %v", changes)
	}
	var anaChange map[string]any
	for _, c := range changes {
		if m := c.(map[string]any); m["nip"] == f.anaNIP {
			anaChange = m
		}
	}
	before, after := anaChange["before"].(map[string]any), anaChange["after"].(map[string]any)
	if anaChange["name"] != "Ana Impor" || before["annual_leave_total"] != 12.0 || after["annual_leave_total"] != 14.0 ||
		after["annual_leave_adjustment"] != 1.0 || after["carry_over_days"] != 2.0 || after["carry_over_expires_on"] != "2027-03-31" {
		t.Fatalf("ana change = %v", anaChange)
	}
	fp := d["fingerprint"].(string)
	if fp == "" {
		t.Fatal("no fingerprint")
	}
	// A preview writes nothing.
	if n := h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.budi); n != "0" {
		t.Fatalf("preview wrote %s rows", n)
	}

	ok := h.importStep(t, f, "commit", map[string]string{"fingerprint": fp}, file)
	expect(t, ok, 200, "")
	if ok.data()["summary"] == nil || ok.body["message"] == nil {
		t.Fatalf("commit body = %s", ok.raw)
	}
	if got := h.scalar(`SELECT concat_ws('|', annual_leave_total, annual_leave_adjustment, annual_leave_used, carry_over_days, carry_over_expires_on)
		FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != "14.00|1.00|5.00|2.00|2027-03-31" {
		t.Fatalf("ana = %s (used must stay 5)", got)
	}
	// Budi's missing row was created with the figures of the file.
	if got := h.scalar(`SELECT concat_ws('|', annual_leave_total, annual_leave_used) FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.budi); got != "10.50|0.00" {
		t.Fatalf("budi = %s", got)
	}
	// One audit per changed employee with before/after and the row's reason.
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_balance.import' AND entity = 'hris.employees'
		AND entity_id IN ($1, $2)`, f.ana, f.budi); n != "2" {
		t.Fatalf("audit rows = %s", n)
	}
	if got := h.scalar(`SELECT concat_ws('|', reason, before->>'annual_leave_total', after->>'annual_leave_total', after->>'carry_over_expires_on')
		FROM audit.audit_log WHERE action = 'leave_balance.import' AND entity_id = $1`, f.ana); got != "Koreksi jatah|12|14|2027-03-31" {
		t.Fatalf("ana audit = %s", got)
	}
	if got := h.scalar(`SELECT concat_ws('|', reason, before->>'annual_leave_total', after->>'annual_leave_total')
		FROM audit.audit_log WHERE action = 'leave_balance.import' AND entity_id = $1`, f.budi); got != "Jatah khusus|9.5|10.5" {
		t.Fatalf("budi audit = %s", got)
	}

	// The preview's fingerprint is stale once the balances changed.
	stale := h.importStep(t, f, "commit", map[string]string{"fingerprint": fp}, file)
	expect(t, stale, 409, "Saldo cuti sudah diubah orang lain sejak preview — unggah ulang file")
}

func TestLeaveBalanceImportTwiceChangesNothing(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	ar := rowOf(t, x, f.anaNIP)
	setCell(t, x, colTotal, ar, 13)
	setCell(t, x, colReason, ar, "Sekali saja")
	file := fileBytes(t, x)

	pre := h.importStep(t, f, "preview", nil, file)
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": pre.data()["fingerprint"].(string)}, file), 200, "")
	audits := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_balance.import'`)
	stamp := h.scalar(`SELECT updated_at::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana)

	// Same file again: everything is already there, so nothing is a change.
	again := h.importStep(t, f, "preview", nil, file)
	expect(t, again, 200, "")
	sum := again.data()["summary"].(map[string]any)
	if sum["changed"] != 0.0 || sum["new"] != 0.0 || sum["same"] != 2.0 || len(again.data()["changes"].([]any)) != 0 {
		t.Fatalf("second preview = %v", again.data())
	}
	second := h.importStep(t, f, "commit", map[string]string{"fingerprint": again.data()["fingerprint"].(string)}, file)
	expect(t, second, 200, "")
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_balance.import'`); n != audits {
		t.Fatalf("audit rows %s -> %s", audits, n)
	}
	if got := h.scalar(`SELECT updated_at::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != stamp {
		t.Fatalf("updated_at %s -> %s", stamp, got)
	}
	// A fresh template, untouched, imports as nothing as well, and rows left
	// at the default never create a row.
	_, fresh := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	setCell(t, fresh, colReason, rowOf(t, fresh, f.budiNIP), "alasan tanpa perubahan")
	pre2 := h.importStep(t, f, "preview", nil, fileBytes(t, fresh))
	if s := pre2.data()["summary"].(map[string]any); s["same"] != 2.0 || s["new"] != 0.0 {
		t.Fatalf("fresh summary = %v", s)
	}
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": pre2.data()["fingerprint"].(string)}, fileBytes(t, fresh)), 200, "")
	if n := h.scalar(`SELECT count(*)::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.budi); n != "0" {
		t.Fatalf("an unchanged default row was created (%s)", n)
	}
}

func TestLeaveBalanceImportErrors(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	h.setBalance(f.dodi, 2026, 12, 0, 0, 3, 2, "2027-03-31") // carry used 2
	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	ar, br := rowOf(t, x, f.anaNIP), rowOf(t, x, f.budiNIP)
	ciciNIP := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, f.cici)

	// Append rows below the data: unknown, inactive, other branch, duplicate.
	last := max(ar, br) + 1
	add := func(nip string) int {
		setCell(t, x, "A", last, nip)
		last++
		return last - 1
	}
	unknown := add("TIDAK-ADA")
	inactive := add(ciciNIP)
	outside := add(f.dodiNIP)
	dupFirst := add("DUP-" + f.anaNIP) // distinct prefix, not a real NIP: unknown, but duplicated below
	dup := add("DUP-" + f.anaNIP)
	_ = dupFirst
	setCell(t, x, colTotal, ar, 12.25) // not a multiple of 0,5
	setCell(t, x, colReason, ar, "uji")
	setCell(t, x, colTotal, br, 3) // fine: 3 + 0 - 0
	setCell(t, x, colAdjust, br, -4)
	setCell(t, x, colReason, br, "uji") // remaining -1
	pre := h.importStep(t, f, "preview", nil, fileBytes(t, x))
	expect(t, pre, 200, "")
	msgs := issueMessages(pre, "errors")
	for _, w := range []string{
		"Jatah harus kelipatan 0,5 @" + itoa(ar),
		"Sisa jatah tidak boleh kurang dari 0 (jatah 3 + penyesuaian -4 − terpakai 0 = -1) @" + itoa(br),
		"NIP TIDAK-ADA tidak dikenal atau karyawan sudah nonaktif @" + itoa(unknown),
		"tidak dikenal atau karyawan sudah nonaktif @" + itoa(inactive),
		"Karyawan tidak termasuk cabang Toko Impor Saldo @" + itoa(outside),
		"ganda (sudah ada di baris " + itoa(dupFirst) + ") @" + itoa(dup),
	} {
		if !strings.Contains(msgs, w) {
			t.Errorf("missing error %q in:\n%s", w, msgs)
		}
	}
	// Nothing commits while an error remains, and nothing is stored.
	commit := h.importStep(t, f, "commit", map[string]string{"fingerprint": pre.data()["fingerprint"].(string)}, fileBytes(t, x))
	expect(t, commit, 400, "File masih punya error — perbaiki lalu unggah ulang")
	if got := h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != "12.00" {
		t.Fatalf("ana changed to %s", got)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'leave_balance.import'`); n != "0" {
		t.Fatalf("audit rows = %s", n)
	}

	// Atomic: a valid change next to an invalid one is not saved either.
	_, y := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	setCell(t, y, colTotal, rowOf(t, y, f.anaNIP), 15)
	setCell(t, y, colReason, rowOf(t, y, f.anaNIP), "valid")
	setCell(t, y, colTotal, rowOf(t, y, f.budiNIP), 11)
	atomic := fileBytes(t, y) // budi without reason
	pre = h.importStep(t, f, "preview", nil, atomic)
	if m := issueMessages(pre, "errors"); !strings.Contains(m, "Alasan perubahan wajib diisi @"+itoa(rowOf(t, y, f.budiNIP))) {
		t.Fatalf("errors = %s", m)
	}
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": pre.data()["fingerprint"].(string)}, atomic), 400, "File masih punya error — perbaiki lalu unggah ulang")
	if got := h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != "12.00" {
		t.Fatalf("valid row saved despite the error: %s", got)
	}

	// Carry-over below what was used (only the all-branches template reaches dodi).
	_, z := h.saldoTemplate(&f.hr, "?year=2026")
	zr := rowOf(t, z, f.dodiNIP)
	setCell(t, z, colCarry, zr, 1)
	setCell(t, z, colReason, zr, "uji")
	_, c := h.saldoUpload(t, &f.hr, "saldo.xlsx", map[string]string{"year": "2026", "step": "preview"}, fileBytes(t, z))
	if m := issueMessages(c, "errors"); !strings.Contains(m, "Sisa bawaan (1) tidak boleh kurang dari bawaan yang sudah terpakai (2) @"+itoa(zr)) {
		t.Fatalf("errors = %s", m)
	}

	// Error file: the uploaded sheet plus a "Keterangan Error" column.
	rec, _ := h.saldoUpload(t, &f.hr, "saldo-cuti.xlsx", map[string]string{"year": "2026", "branch_id": f.branch, "step": "error_file"}, fileBytes(t, x))
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "spreadsheetml") {
		t.Fatalf("error file: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	ef, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := ef.GetRows(saldoSheet)
	found := false
	for _, r := range rows {
		if len(r) > 0 && r[len(r)-1] == "Keterangan Error" {
			found = true
		}
		if len(r) > 0 && r[0] == f.anaNIP && !strings.Contains(r[len(r)-1], "Jatah harus kelipatan 0,5") {
			t.Fatalf("ana error cell = %v", r)
		}
	}
	if !found {
		t.Fatalf("no Keterangan Error column: %v", rows)
	}
}

func TestLeaveBalanceImportWarnings(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	ar, br := rowOf(t, x, f.anaNIP), rowOf(t, x, f.budiNIP)
	setCell(t, x, colTotal, ar, 30)
	setCell(t, x, colReason, ar, "Besar")
	setCell(t, x, colCarry, br, 2) // carry without an expiry date
	setCell(t, x, colReason, br, "Bawaan")
	file := fileBytes(t, x)
	pre := h.importStep(t, f, "preview", nil, file)
	expect(t, pre, 200, "")
	w := issueMessages(pre, "warnings")
	for _, want := range []string{"jatah 30 hari lebih dari 24", "sisa bawaan tanpa tanggal kedaluwarsa dianggap tidak berlaku"} {
		if !strings.Contains(w, want) {
			t.Errorf("missing warning %q in:\n%s", want, w)
		}
	}
	fp := pre.data()["fingerprint"].(string)
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": fp}, file), 400, `Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`)
	if got := h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != "12.00" {
		t.Fatalf("saved without acknowledgement: %s", got)
	}
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": fp, "ack_warnings": "true"}, file), 200, "")
	if got := h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != "30.00" {
		t.Fatalf("ana = %s", got)
	}

	// An expiry date in the past warns too.
	_, y := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	yr := rowOf(t, y, f.anaNIP)
	setCell(t, y, colCarry, yr, 1)
	setCell(t, y, colExpires, yr, "30/09/2026")
	setCell(t, y, colReason, yr, "Lewat")
	past := h.importStep(t, f, "preview", nil, fileBytes(t, y))
	if m := issueMessages(past, "warnings"); !strings.Contains(m, "tanggal kedaluwarsa sudah lewat") {
		t.Fatalf("warnings = %s", m)
	}
}

func TestLeaveBalanceImportFingerprintMismatch(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	ar := rowOf(t, x, f.anaNIP)
	setCell(t, x, colTotal, ar, 13)
	setCell(t, x, colReason, ar, "uji")
	file := fileBytes(t, x)
	pre := h.importStep(t, f, "preview", nil, file)
	fp := pre.data()["fingerprint"].(string)
	// Someone else edits the row (a concurrent PUT) between preview and commit.
	h.exec(`UPDATE hris.leave_balances SET annual_leave_adjustment = 1 WHERE employee_id = $1 AND year = 2026`, f.ana)
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": fp}, file), 409, "Saldo cuti sudah diubah orang lain sejak preview — unggah ulang file")
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": ""}, file), 409, "")
	if got := h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != "12.00" {
		t.Fatalf("ana = %s", got)
	}
}

func TestLeaveBalanceImportRejectsWrongFiles(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	file := fileBytes(t, x)
	noImport, _ := h.saldoStaff("pos", false, "read", "export")

	_, c := h.saldoUpload(t, &noImport, "saldo.xlsx", map[string]string{"year": "2026", "step": "preview"}, file)
	expect(t, c, 403, "")
	_, c = h.saldoUpload(t, nil, "saldo.xlsx", map[string]string{"year": "2026", "step": "preview"}, file)
	expect(t, c, 401, "")
	_, c = h.saldoUpload(t, &f.hr, "saldo.csv", map[string]string{"year": "2026", "branch_id": f.branch, "step": "preview"}, file)
	expect(t, c, 400, "Unggah file .xlsx dari template Saldo Cuti")
	_, c = h.saldoUpload(t, &f.hr, "saldo.xlsx", map[string]string{"year": "2026", "branch_id": f.branch, "step": "preview"}, []byte("bukan xlsx"))
	expect(t, c, 400, "File tidak bisa dibaca — pastikan formatnya .xlsx")

	// A template for another scope or year is not this import's template.
	for name, fields := range map[string]map[string]string{
		"semua cabang":  {"year": "2026", "step": "preview"},
		"cabang lain":   {"year": "2026", "branch_id": f.other, "step": "preview"},
		"tahun berbeda": {"year": "2027", "branch_id": f.branch, "step": "preview"},
	} {
		_, c := h.saldoUpload(t, &f.hr, "saldo.xlsx", fields, file)
		if c.status != 200 || !strings.Contains(issueMessages(c, "errors"), "File ini bukan template saldo cuti") {
			t.Errorf("%s: %d %s", name, c.status, c.raw)
		}
		if len(c.data()["changes"].([]any)) != 0 {
			t.Errorf("%s: changes %v", name, c.data()["changes"])
		}
	}
	// Only this year and next year can be imported.
	_, old := h.saldoTemplate(&f.hr, "?year=2025&branch_id="+f.branch)
	_, c = h.saldoUpload(t, &f.hr, "saldo.xlsx", map[string]string{"year": "2025", "branch_id": f.branch, "step": "preview"}, fileBytes(t, old))
	if m := issueMessages(c, "errors"); !strings.Contains(m, "Tahun saldo hanya boleh 2026 (tahun berjalan) atau 2027 (tahun depan)") {
		t.Fatalf("errors = %s", m)
	}
	// An xlsx without the Info sheet (a hand-made file).
	plain := excelize.NewFile()
	_ = plain.SetSheetName("Sheet1", saldoSheet)
	for i, v := range []string{"NIP", "Jatah", "Penyesuaian", "Sisa Bawaan", "Kedaluwarsa", "Alasan"} {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = plain.SetCellStr(saldoSheet, cell, v)
	}
	_, c = h.saldoUpload(t, &f.hr, "saldo.xlsx", map[string]string{"year": "2026", "branch_id": f.branch, "step": "preview"}, fileBytes(t, plain))
	if m := issueMessages(c, "errors"); !strings.Contains(m, "Sheet Info tidak ditemukan") {
		t.Fatalf("errors = %s", m)
	}
	// A year outside 2000..2100 is a bad request.
	_, c = h.saldoUpload(t, &f.hr, "saldo.xlsx", map[string]string{"year": "1999", "step": "preview"}, file)
	expect(t, c, 400, "Tahun tidak valid")
}

// Final review 5: an approval raising "terpakai" between preview and commit
// is not a conflict (the commit re-checks the rules under the row lock).
func TestLeaveBalanceImportIgnoresUsedChange(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	ar := rowOf(t, x, f.anaNIP)
	setCell(t, x, colTotal, ar, 13)
	setCell(t, x, colReason, ar, "uji")
	file := fileBytes(t, x)
	fp := h.importStep(t, f, "preview", nil, file).data()["fingerprint"].(string)
	h.exec(`UPDATE hris.leave_balances SET annual_leave_used = 6 WHERE employee_id = $1 AND year = 2026`, f.ana)
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": fp}, file), 200, "")
	if got := h.scalar(`SELECT concat_ws('|', annual_leave_total, annual_leave_used) FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != "13.00|6.00" {
		t.Fatalf("ana = %s", got)
	}
	// But a used figure that no longer fits the new total is a 400, not a 409.
	_, y := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	yr := rowOf(t, y, f.anaNIP)
	setCell(t, y, colTotal, yr, 7)
	setCell(t, y, colReason, yr, "turun")
	file2 := fileBytes(t, y)
	fp2 := h.importStep(t, f, "preview", nil, file2).data()["fingerprint"].(string)
	h.exec(`UPDATE hris.leave_balances SET annual_leave_used = 8 WHERE employee_id = $1 AND year = 2026`, f.ana)
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": fp2}, file2), 400, "File masih punya error — perbaiki lalu unggah ulang")
}

// Final review 4: a commit must carry the fingerprint of the very file that
// was previewed; another file with the same fingerprint is refused.
func TestLeaveBalanceImportCommitMustMatchPreviewedFile(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	build := func(total int) []byte {
		_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
		ar := rowOf(t, x, f.anaNIP)
		setCell(t, x, colTotal, ar, total)
		setCell(t, x, colReason, ar, "uji")
		return fileBytes(t, x)
	}
	previewed, other := build(13), build(20)
	fp := h.importStep(t, f, "preview", nil, previewed).data()["fingerprint"].(string)
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": fp}, other), 409, "")
	if got := h.scalar(`SELECT annual_leave_total::text FROM hris.leave_balances WHERE employee_id = $1 AND year = 2026`, f.ana); got != "12.00" {
		t.Fatalf("ana = %s: an unpreviewed file was committed", got)
	}
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": fp}, previewed), 200, "")
}

// Final review 6: employees are locked in a fixed order (by id), whatever the
// order of the rows in the file.
func TestLeaveBalanceImportLocksInIDOrder(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	ar, br := rowOf(t, x, f.anaNIP), rowOf(t, x, f.budiNIP)
	for _, r := range []int{ar, br} {
		setCell(t, x, colTotal, r, 15)
		setCell(t, x, colReason, r, "urutan")
	}
	// Put the employee with the larger id first in the file.
	firstNIP, secondNIP := f.anaNIP, f.budiNIP
	if f.ana < f.budi {
		firstNIP, secondNIP = secondNIP, firstNIP
	}
	setCell(t, x, "A", min(ar, br), firstNIP)
	setCell(t, x, "A", max(ar, br), secondNIP)
	file := fileBytes(t, x)
	fp := h.importStep(t, f, "preview", nil, file).data()["fingerprint"].(string)
	expect(t, h.importStep(t, f, "commit", map[string]string{"fingerprint": fp}, file), 200, "")
	order := h.scalar(`SELECT string_agg(entity_id::text, ',' ORDER BY ctid) FROM audit.audit_log
		WHERE action = 'leave_balance.import' AND entity_id IN ($1, $2)`, f.ana, f.budi)
	if want := min(f.ana, f.budi) + "," + max(f.ana, f.budi); order != want {
		t.Fatalf("lock/audit order %s, want %s", order, want)
	}
}

// Final review 7 + 8: an unknown step and an oversized upload are 400s.
func TestLeaveBalanceImportStepAndSize(t *testing.T) {
	h := newHarness(t)
	f := h.saldoImportFixture()
	_, x := h.saldoTemplate(&f.hr, "?year=2026&branch_id="+f.branch)
	file := fileBytes(t, x)
	expect(t, h.importStep(t, f, "hapus", nil, file), 400, "Langkah import tidak dikenal")
	expect(t, h.importStep(t, f, "Commit", nil, file), 400, "Langkah import tidak dikenal")
	big := make([]byte, 17<<20+1)
	rec, c := h.saldoUpload(t, &f.hr, "saldo-cuti.xlsx", map[string]string{"year": "2026", "branch_id": f.branch, "step": "preview"}, big)
	if rec.Code != 400 || !strings.Contains(c.raw, "terlalu besar") {
		t.Fatalf("oversized upload: %d %s", rec.Code, c.raw)
	}
}
