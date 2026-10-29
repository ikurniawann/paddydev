package hris

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xuri/excelize/v2"

	"nuhabit/backend/internal/platform/hrisdel"
	"nuhabit/backend/internal/platform/testutil"
)

const (
	scopeWeek    = "&from=2026-10-05&to=2026-10-11"
	errNotInArea = "Toko di luar wilayah Anda"
	errNoPerm    = "Insufficient permissions"
)

// scopeFixture: delegate P (no menu grants) over store A; X, a shift employee
// placed at A; a second store B. The clock is Monday 2026-10-05.
type scopeFixture struct {
	p        testutil.Staff
	pID      string
	x        string
	tokoA    string
	tokoB    string
	nipX     string
	defShift string
	nsShift  string
}

func newScopeFixture(h *harness) scopeFixture {
	f := scopeFixture{}
	f.p, f.pID = h.staff("pos", nil, true, emp{name: "Pengatur P"})
	f.tokoA, f.tokoB = h.branch("Scope Toko A"), h.branch("Scope Toko B")
	f.x = h.employee(emp{name: "Ana Shift"})
	h.place(f.x, f.tokoA)
	h.delegate(f.pID, false, f.tokoA)
	f.nipX = h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, f.x)
	f.defShift = h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'DEF'`)
	f.nsShift = h.scalar(`SELECT id::text FROM hris.shifts WHERE code = 'NS'`)
	return f
}

// multipartRequest is the import request for fields and file.
func multipartRequest(t *testing.T, fields map[string]string, file []byte) *http.Request {
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
	return r
}

func gridURL(branch string) string {
	return "/api/hris/schedules?branch_id=" + branch + scopeWeek
}

func TestDelegateScheduleGrid(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	c := h.do(&f.p, http.MethodGet, gridURL(f.tokoA), nil)
	expect(t, c, http.StatusOK, "")
	found := false
	for _, e := range c.data()["employees"].([]any) {
		if e.(map[string]any)["id"] == f.x {
			found = true
		}
	}
	if !found {
		t.Fatalf("X not in grid A: %s", c.raw)
	}
	expect(t, h.do(&f.p, http.MethodGet, gridURL(f.tokoB), nil), http.StatusForbidden, errNotInArea)
	expect(t, h.do(&f.p, http.MethodGet, "/api/hris/schedules?from=2026-10-05&to=2026-10-11", nil), http.StatusForbidden, errNotInArea)
}

func TestDelegateScheduleTemplate(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	r := testutil.AsStaff(httptest.NewRequest(http.MethodGet, "/api/hris/schedules/template?branch_id="+f.tokoA+scopeWeek, nil), f.p)
	rec := httptest.NewRecorder()
	h.exec("SAVEPOINT harness_request")
	h.mux.ServeHTTP(rec, r)
	h.exec("RELEASE SAVEPOINT harness_request")
	if rec.Code != http.StatusOK {
		t.Fatalf("template A = %d %s", rec.Code, rec.Body.String())
	}
	if _, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes())); err != nil {
		t.Fatalf("not an xlsx: %v", err)
	}
	expect(t, h.do(&f.p, http.MethodGet, "/api/hris/schedules/template?branch_id="+f.tokoB+scopeWeek, nil), http.StatusForbidden, errNotInArea)
}

func TestDelegateScheduleCells(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)

	body := cellsBody(f.tokoA, "", anaCell(f.x, "2026-10-06", "DEF"))
	_, done := commitCells(t, h, &f.p, body)
	expect(t, done, http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1 AND work_date = '2026-10-06'`, f.x); got != "1" {
		t.Fatalf("shift day rows = %s, want 1", got)
	}

	// Replacing the per-date row needs a reason even without "delete" (R5).
	other := cellsBody(f.tokoA, "", anaCell(f.x, "2026-10-06", "NS"))
	_, done = commitCells(t, h, &f.p, other)
	expect(t, done, http.StatusBadRequest, hrisdel.ErrReason)
	other["reason"] = "Tukar shift dengan rekan"
	expect(t, h.do(&f.p, http.MethodPost, cellsPath, other), http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'hris.schedule.replace' AND reason = 'Tukar shift dengan rekan'`); got != "1" {
		t.Fatalf("replace audit rows = %s, want 1", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'schedule.cells_update' AND entity_id = $1`, f.x); got != "2" {
		t.Fatalf("cells_update audit rows = %s, want 2", got)
	}

	// Outside the area: refused before anything is read.
	out := cellsBody(f.tokoB, "preview", anaCell(f.x, "2026-10-07", "DEF"))
	expect(t, h.do(&f.p, http.MethodPost, cellsPath, out), http.StatusForbidden, errNotInArea)
}

func TestDelegateScheduleImport(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	file := anaWeek(f.tokoA, f.nipX, []string{"DEF", "DEF", "DEF", "DEF", "DEF", "L", "L"})
	fields := map[string]string{"branch_id": f.tokoA, "from": "2026-10-05", "to": "2026-10-11", "step": "preview"}
	pre := upload(t, h, &f.p, fields, file)
	expect(t, pre, http.StatusOK, "")
	fields["step"], fields["fingerprint"], fields["ack_warnings"] = "commit", pre.data()["fingerprint"].(string), "true"
	expect(t, upload(t, h, &f.p, fields, file), http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1`, f.x); got == "0" {
		t.Fatal("import wrote no shift days")
	}
	fields["branch_id"] = f.tokoB
	expect(t, upload(t, h, &f.p, fields, file), http.StatusForbidden, errNotInArea)
}

func TestScheduleScopeNonDelegate(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	s, _ := h.staff("pos", nil, true, emp{name: "Karyawan Biasa"})
	expect(t, h.do(&s, http.MethodGet, gridURL(f.tokoA), nil), http.StatusForbidden, errNoPerm)
	expect(t, h.do(&s, http.MethodGet, "/api/hris/schedules/template?branch_id="+f.tokoA+scopeWeek, nil), http.StatusForbidden, errNoPerm)
	expect(t, h.do(&s, http.MethodPost, cellsPath, cellsBody(f.tokoA, "preview", anaCell(f.x, "2026-10-06", "DEF"))), http.StatusForbidden, errNoPerm)
	fields := map[string]string{"branch_id": f.tokoA, "from": "2026-10-05", "to": "2026-10-11", "step": "preview"}
	expect(t, upload(t, h, &s, fields, anaWeek(f.tokoA, f.nipX, []string{"DEF", "DEF", "DEF", "DEF", "DEF", "L", "L"})), http.StatusForbidden, errNoPerm)
	expect(t, h.do(nil, http.MethodGet, gridURL(f.tokoA), nil), http.StatusUnauthorized, "")
}

func TestScheduleScopeHRAlsoDelegate(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	hr, hrID := h.staff("hrd", scheduleTestMenus, true, emp{name: "HR Merangkap"})
	h.delegate(hrID, false, f.tokoA)
	expect(t, h.do(&hr, http.MethodGet, gridURL(f.tokoA), nil), http.StatusOK, "")
	expect(t, h.do(&hr, http.MethodGet, gridURL(f.tokoB), nil), http.StatusOK, "")
}

// A delegate learns nothing about people outside the store roster (review
// fix 1): the message is generic and carries no name.
func TestDelegateScheduleNoLeakOutsideRoster(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	const secret = "Yani Rahasia Toko B"
	y := h.employee(emp{name: secret})
	h.place(y, f.tokoB)
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, y)
	nipY := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, y)
	noRows := func() {
		t.Helper()
		if got := h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1`, y); got != "0" {
			t.Fatalf("shift day rows for Y = %s, want 0", got)
		}
	}

	// Import.
	file := anaWeek(f.tokoA, nipY, []string{"DEF", "DEF", "DEF", "DEF", "DEF", "L", "L"})
	fields := map[string]string{"branch_id": f.tokoA, "from": "2026-10-05", "to": "2026-10-11", "step": "preview"}
	pre := upload(t, h, &f.p, fields, file)
	expect(t, pre, http.StatusOK, "")
	if bytes.Contains([]byte(pre.raw), []byte(secret)) || bytes.Contains([]byte(pre.raw), []byte("Non-Shift")) {
		t.Fatalf("import preview leaks Y: %s", pre.raw)
	}
	want := "NIP " + nipY + " tidak ditemukan di Scope Toko A"
	if !bytes.Contains([]byte(pre.raw), []byte(want)) {
		t.Fatalf("import preview lacks %q: %s", want, pre.raw)
	}
	fields["step"] = "error_file"
	r := testutil.AsStaff(multipartRequest(t, fields, file), f.p)
	rec := httptest.NewRecorder()
	h.exec("SAVEPOINT harness_request")
	h.mux.ServeHTTP(rec, r)
	h.exec("RELEASE SAVEPOINT harness_request")
	if rec.Code != http.StatusOK {
		t.Fatalf("error file = %d %s", rec.Code, rec.Body.String())
	}
	ef, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	for _, sheet := range ef.GetSheetList() {
		rows, _ := ef.GetRows(sheet)
		for _, row := range rows {
			for _, v := range row {
				if v == secret {
					t.Fatalf("error file leaks Y in sheet %s", sheet)
				}
			}
		}
	}
	fields["step"], fields["fingerprint"], fields["ack_warnings"] = "commit", pre.data()["fingerprint"].(string), "true"
	if c := upload(t, h, &f.p, fields, file); c.status == http.StatusOK {
		t.Fatalf("commit with errors = %d", c.status)
	}
	noRows()

	// Cells.
	body := cellsBody(f.tokoA, "preview", anaCell(y, "2026-10-06", "DEF"))
	c := h.do(&f.p, http.MethodPost, cellsPath, body)
	expect(t, c, http.StatusOK, "")
	if bytes.Contains([]byte(c.raw), []byte(secret)) || bytes.Contains([]byte(c.raw), []byte("Non-Shift")) {
		t.Fatalf("cells preview leaks Y: %s", c.raw)
	}
	if !bytes.Contains([]byte(c.raw), []byte("Karyawan tidak ditemukan di Scope Toko A")) {
		t.Fatalf("cells preview lacks generic message: %s", c.raw)
	}
	body["step"], body["fingerprint"], body["ack_warnings"] = "commit", c.data()["fingerprint"], true
	if c := h.do(&f.p, http.MethodPost, cellsPath, body); c.status == http.StatusOK {
		t.Fatalf("cells commit with errors = %d", c.status)
	}
	noRows()

	// HR keeps the detailed messages.
	hr, _ := h.staff("hrd", scheduleTestMenus, false)
	hc := h.do(&hr, http.MethodPost, cellsPath, cellsBody(f.tokoA, "preview", anaCell(y, "2026-10-06", "DEF")))
	expect(t, hc, http.StatusOK, "")
	if !bytes.Contains([]byte(hc.raw), []byte(secret)) {
		t.Fatalf("HR message changed: %s", hc.raw)
	}
}

// A delegate whose only store is inactive is no delegate any more: the
// wrapper answers 403, not an empty grid.
func TestDelegateScheduleInactiveStore(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	h.exec(`UPDATE configuration.branches SET is_active = false WHERE id = $1`, f.tokoA)
	expect(t, h.do(&f.p, http.MethodGet, gridURL(f.tokoA), nil), http.StatusForbidden, errNoPerm)
}
