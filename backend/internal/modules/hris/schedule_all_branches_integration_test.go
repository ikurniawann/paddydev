package hris

import (
	"net/http"
	"testing"
)

// EPIC-093: a delegate with schedule_all_branches arranges the schedule of
// every active store, while placements and auto-checkout stay in their area.

// scheduleAll ticks "Boleh atur jadwal semua toko" for the delegate.
func (h *harness) scheduleAll(employeeID string) {
	h.t.Helper()
	h.exec(`UPDATE hris.schedule_delegates SET schedule_all_branches = true WHERE employee_id = $1`, employeeID)
}

func gridHas(c call, employeeID string) bool {
	for _, e := range c.data()["employees"].([]any) {
		if e.(map[string]any)["id"] == employeeID {
			return true
		}
	}
	return false
}

func TestAllBranchesDelegateSchedulesOtherStore(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	y := h.employee(emp{name: "Yuni Toko B"})
	h.place(y, f.tokoB)
	nipY := h.scalar(`SELECT nip FROM hris.employees WHERE id = $1`, y)
	h.scheduleAll(f.pID)

	c := h.do(&f.p, http.MethodGet, gridURL(f.tokoB), nil)
	expect(t, c, http.StatusOK, "")
	if !gridHas(c, y) {
		t.Fatalf("Y not in grid B: %s", c.raw)
	}
	expect(t, h.do(&f.p, http.MethodGet, "/api/hris/schedules/template?branch_id="+f.tokoB+scopeWeek, nil), http.StatusOK, "")

	// Saved at once: there is no approval step for schedules.
	_, done := commitCells(t, h, &f.p, cellsBody(f.tokoB, "", anaCell(y, "2026-10-06", "DEF")))
	expect(t, done, http.StatusOK, "")
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_shift_days WHERE employee_id = $1 AND work_date = '2026-10-06'`, y); got != "1" {
		t.Fatalf("shift day rows = %s, want 1", got)
	}

	file := anaWeek(f.tokoB, nipY, []string{"DEF", "DEF", "DEF", "DEF", "DEF", "L", "L"})
	fields := map[string]string{"branch_id": f.tokoB, "from": "2026-10-05", "to": "2026-10-11", "step": "preview"}
	pre := upload(t, h, &f.p, fields, file)
	expect(t, pre, http.StatusOK, "")
	fields["step"], fields["fingerprint"], fields["ack_warnings"] = "commit", pre.data()["fingerprint"].(string), "true"
	expect(t, upload(t, h, &f.p, fields, file), http.StatusOK, "")
}

func TestAllBranchesDelegateInactiveStore(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	tokoC := h.branch("Scope Toko C Tutup")
	h.exec(`UPDATE configuration.branches SET is_active = false WHERE id = $1`, tokoC)
	h.scheduleAll(f.pID)
	expect(t, h.do(&f.p, http.MethodGet, gridURL(tokoC), nil), http.StatusForbidden, errNotInArea)
}

func TestAllBranchesWithoutFlagUnchanged(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	expect(t, h.do(&f.p, http.MethodGet, gridURL(f.tokoB), nil), http.StatusForbidden, errNotInArea)
}

func TestAllBranchesPlacementStaysInArea(t *testing.T) {
	h := newHarness(t)
	f := newPlacementScopeFixture(h, true)
	h.scheduleAll(f.pID)
	expect(t, h.do(&f.p, http.MethodPost, placementBulkURL, map[string]any{
		"branch_id": f.tokoC, "employee_ids": []string{f.x}, "mode": "move", "step": "preview"}), http.StatusForbidden, errNotInArea)
	expect(t, h.do(&f.p, http.MethodGet, searchPath(placementSearchURL, "branch_id", f.tokoC), nil), http.StatusForbidden, errNotInArea)
}

func TestAllBranchesAutoCheckoutStaysInArea(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	h.scheduleAll(f.pID)
	c := h.do(&f.p, http.MethodGet, "/api/hris/auto-checkout-rules/options", nil)
	expect(t, c, http.StatusOK, "")
	if c.data()["all_locations"] != false {
		t.Fatalf("all_locations = %v", c.data()["all_locations"])
	}
	var ids []string
	for _, b := range c.data()["branches"].([]any) {
		ids = append(ids, b.(map[string]any)["id"].(string))
	}
	if len(ids) != 1 || ids[0] != f.tokoA {
		t.Fatalf("auto-checkout branches = %v, want [%s]", ids, f.tokoA)
	}
}

func TestScheduleAccessAllBranches(t *testing.T) {
	h := newHarness(t)
	f := newScopeFixture(h)
	closed := h.branch("Scope Toko Tutup")
	h.exec(`UPDATE configuration.branches SET is_active = false WHERE id = $1`, closed)
	h.scheduleAll(f.pID)
	c := h.do(&f.p, http.MethodGet, "/api/hris/me/schedule-access", nil)
	expect(t, c, http.StatusOK, "")
	d := c.data()
	if d["schedule_all_branches"] != true {
		t.Fatalf("schedule_all_branches = %v", d["schedule_all_branches"])
	}
	if got := branchIDsOf(map[string]any{"branches": d["branches"]}); len(got) != 1 || got[0] != f.tokoA {
		t.Fatalf("branches = %v, want [%s]", got, f.tokoA)
	}
	seen := map[string]bool{}
	prev := ""
	for _, b := range d["schedule_branches"].([]any) {
		m := b.(map[string]any)
		seen[m["id"].(string)] = true
		if code := m["code"].(string); code < prev {
			t.Fatalf("schedule_branches not sorted by code: %q after %q", code, prev)
		} else {
			prev = code
		}
	}
	if !seen[f.tokoA] || !seen[f.tokoB] || seen[closed] {
		t.Fatalf("schedule_branches = %v", d["schedule_branches"])
	}
}
