package hris

import (
	"bytes"
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
