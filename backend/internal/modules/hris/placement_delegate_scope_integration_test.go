package hris

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// EPIC-071 R3/R6: Penempatan Karyawan for a store delegate with the
// placement right. fixedNow = Monday 2026-10-05, so "yesterday" = 10-04.

const (
	placementIDsURL  = "/api/hris/employee-placements/ids"
	placementBulkURL = "/api/hris/employee-placements/bulk"
	placementEndURL  = "/api/hris/employee-placements/bulk-end"
	errForeignStore  = "Punya penempatan di toko lain di luar wilayah Anda — hubungi HR"
	errNoStoreToEnd  = "Tidak ada penempatan di toko Anda"
)

// placementScopeFixture (the TestDelegateOutside shape plus store D):
// delegate P placed at A over stores {A, D}; X placed at A; Z unplaced
// reports to P; Y placed at C reports to Z; W placed at C, not below P;
// U unplaced (Non-Shift) reports to P. C is outside P's area.
type placementScopeFixture struct {
	p                   testutil.Staff
	pID                 string
	x, y, z, w, u       string
	tokoA, tokoC, tokoD string
}

func newPlacementScopeFixture(h *harness, canPlace bool) placementScopeFixture {
	h.t.Helper()
	var f placementScopeFixture
	f.p, f.pID = h.staff("pos", nil, true, emp{name: "Pst71 P Pengatur"})
	f.tokoA, f.tokoC, f.tokoD = h.branch("Pst71 Toko A"), h.branch("Pst71 Toko C"), h.branch("Pst71 Toko D")
	h.place(f.pID, f.tokoA)
	h.delegate(f.pID, canPlace, f.tokoA, f.tokoD)
	f.x = h.employee(emp{name: "Pst71 X Toko A"})
	h.place(f.x, f.tokoA)
	f.z = h.employee(emp{name: "Pst71 Z Bawahan", boss: &f.pID})
	f.y = h.employee(emp{name: "Pst71 Y Cucu", boss: &f.z})
	h.place(f.y, f.tokoC)
	f.w = h.employee(emp{name: "Pst71 W Luar"})
	h.place(f.w, f.tokoC)
	f.u = h.employee(emp{name: "Pst71 U Bawahan", boss: &f.pID})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, f.u)
	return f
}

// scopeRowIDs are the employee ids of a search response, sorted.
func scopeRowIDs(t *testing.T, c call) []string {
	t.Helper()
	ids := rowIDs(searchRows(t, c))
	slices.Sort(ids)
	return ids
}

func sortedIDs(ids ...string) []string {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}

func TestDelegatePlacementSearch(t *testing.T) {
	h := newHarness(t)
	f := newPlacementScopeFixture(h, true)

	c := h.do(&f.p, http.MethodGet, searchPath(placementSearchURL, "page_size", "100"), nil)
	if got, want := scopeRowIDs(t, c), sortedIDs(f.pID, f.x, f.y, f.z, f.u); !slices.Equal(got, want) {
		t.Fatalf("search ids = %v, want %v (no W)\n%s", got, want, c.raw)
	}
	if counts := c.data()["counts"].(map[string]any); counts["all"] != 5.0 || counts["unplaced"] != 2.0 || c.data()["total"] != 5.0 {
		t.Fatalf("counts = %s", c.raw)
	}

	// Status filter keeps the scope: unplaced = Z and U only.
	c = h.do(&f.p, http.MethodGet, searchPath(placementSearchURL, "status", "unplaced"), nil)
	if got, want := scopeRowIDs(t, c), sortedIDs(f.z, f.u); !slices.Equal(got, want) {
		t.Fatalf("unplaced = %v, want %v", got, want)
	}

	// Own store filter; a store outside the area is refused.
	c = h.do(&f.p, http.MethodGet, searchPath(placementSearchURL, "branch_id", f.tokoA), nil)
	if got, want := scopeRowIDs(t, c), sortedIDs(f.pID, f.x); !slices.Equal(got, want) {
		t.Fatalf("store A = %v, want %v", got, want)
	}
	expect(t, h.do(&f.p, http.MethodGet, searchPath(placementSearchURL, "branch_id", f.tokoC), nil), http.StatusForbidden, errOutsideScope)
	expect(t, h.do(&f.p, http.MethodGet, searchPath(placementIDsURL, "branch_id", f.tokoC), nil), http.StatusForbidden, errOutsideScope)

	ids := h.do(&f.p, http.MethodGet, placementIDsURL, nil)
	expect(t, ids, http.StatusOK, "")
	got := []string{}
	for _, id := range ids.data()["ids"].([]any) {
		got = append(got, id.(string))
	}
	slices.Sort(got)
	if want := sortedIDs(f.pID, f.x, f.y, f.z, f.u); !slices.Equal(got, want) || ids.data()["total"] != 5.0 || ids.data()["truncated"] != false {
		t.Fatalf("ids = %s, want %v", ids.raw, want)
	}

	// Ended placements in the column do not widen the area: W stays out.
	c = h.do(&f.p, http.MethodGet, searchPath(placementSearchURL, "include_ended", "true", "page_size", "100"), nil)
	if got, want := scopeRowIDs(t, c), sortedIDs(f.pID, f.x, f.y, f.z, f.u); !slices.Equal(got, want) ||
		c.data()["counts"].(map[string]any)["all"] != 5.0 || c.data()["total"] != 5.0 {
		t.Fatalf("include_ended = %v, want %v\n%s", got, want, c.raw)
	}

	// A placement at A that only starts tomorrow is not running (R3): V, not
	// below P, stays out of search, counts and ids.
	v := h.employee(emp{name: "Pst71 V Besok"})
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date)
		VALUES ($1, $2, true, '2026-10-06')`, v, f.tokoA)
	c = h.do(&f.p, http.MethodGet, searchPath(placementSearchURL, "page_size", "100"), nil)
	if got, want := scopeRowIDs(t, c), sortedIDs(f.pID, f.x, f.y, f.z, f.u); !slices.Equal(got, want) || c.data()["counts"].(map[string]any)["all"] != 5.0 {
		t.Fatalf("search with V = %v, want %v\n%s", got, want, c.raw)
	}
	c = h.do(&f.p, http.MethodGet, searchPath(placementSearchURL, "branch_id", f.tokoA), nil)
	if got, want := scopeRowIDs(t, c), sortedIDs(f.pID, f.x); !slices.Equal(got, want) {
		t.Fatalf("store A with V = %v, want %v", got, want)
	}
	ids = h.do(&f.p, http.MethodGet, placementIDsURL, nil)
	expect(t, ids, http.StatusOK, "")
	if ids.data()["total"] != 5.0 || slices.Contains(ids.data()["ids"].([]any), any(v)) {
		t.Fatalf("ids with V = %s", ids.raw)
	}

	// HR still sees everyone, W included.
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, false)
	c = h.do(&hr, http.MethodGet, searchPath(placementSearchURL, "q", "Pst71", "page_size", "100"), nil)
	if got, want := scopeRowIDs(t, c), sortedIDs(f.pID, f.x, f.y, f.z, f.w, f.u, v); !slices.Equal(got, want) {
		t.Fatalf("HR search = %v, want %v", got, want)
	}
}

func TestDelegatePlacementMove(t *testing.T) {
	h := newHarness(t)
	f := newPlacementScopeFixture(h, true)

	// A delegate always picks a mode.
	expect(t, h.do(&f.p, http.MethodPost, placementBulkURL, map[string]any{
		"branch_id": f.tokoD, "employee_ids": []string{f.x}}), http.StatusBadRequest, "Pilih mode pindah atau tambah")

	body := map[string]any{"branch_id": f.tokoD, "employee_ids": []string{f.x, f.u}, "mode": "move", "step": "preview"}
	pre := h.do(&f.p, http.MethodPost, placementBulkURL, body)
	expect(t, pre, http.StatusOK, "")
	expectBulk(t, pre, f.x, "moved", nil, "Pst71 Toko A@2026-10-04")
	expectBulk(t, pre, f.u, "created", nil)

	body["step"] = "commit"
	done := h.do(&f.p, http.MethodPost, placementBulkURL, body)
	expect(t, done, http.StatusOK, "")
	if dt := done.data(); dt["moved"] != 1.0 || dt["created"] != 1.0 || dt["skipped"] != 0.0 {
		t.Fatalf("commit = %s", done.raw)
	}
	if got := h.scalar(`SELECT end_date::text FROM hris.employee_placements WHERE employee_id = $1 AND branch_id = $2`, f.x, f.tokoA); got != "2026-10-04" {
		t.Fatalf("X's store A end = %q", got)
	}
	for _, id := range []string{f.x, f.u} {
		if got := h.scalar(`SELECT count(*)::text FROM hris.employee_placements
			WHERE employee_id = $1 AND branch_id = $2 AND is_primary AND start_date = '2026-10-05' AND end_date IS NULL`, id, f.tokoD); got != "1" {
			t.Fatalf("%s at D = %s", id, got)
		}
		if got := h.auditCount("placement.bulk_move", id); got != "1" {
			t.Fatalf("bulk_move audit of %s = %s", id, got)
		}
	}

	// Target store outside the area: the whole request is refused.
	expect(t, h.do(&f.p, http.MethodPost, placementBulkURL, map[string]any{
		"branch_id": f.tokoC, "employee_ids": []string{f.x}, "mode": "move", "step": "preview"}), http.StatusForbidden, errOutsideScope)

	// An employee outside the area: refused without any per-employee result.
	out := h.do(&f.p, http.MethodPost, placementBulkURL, map[string]any{
		"branch_id": f.tokoD, "employee_ids": []string{f.w, f.x}, "mode": "add", "step": "preview"})
	expect(t, out, http.StatusForbidden, "1 karyawan di luar wilayah Anda")
	if out.data() != nil {
		t.Fatalf("403 leaked data: %s", out.raw)
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_placements WHERE employee_id = $1`, f.w); got != "1" {
		t.Fatalf("W placements = %s", got)
	}
}

func TestDelegatePlacementMoveForeignPlacement(t *testing.T) {
	h := newHarness(t)
	f := newPlacementScopeFixture(h, true)
	h.placeExtra(f.x, f.tokoC)

	done := h.do(&f.p, http.MethodPost, placementBulkURL, map[string]any{
		"branch_id": f.tokoD, "employee_ids": []string{f.x, f.y}, "mode": "move", "step": "commit"})
	expect(t, done, http.StatusOK, "")
	expectBulk(t, done, f.x, "skipped", errForeignStore)
	expectBulk(t, done, f.y, "moved", nil, "Pst71 Toko C@2026-10-04")
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_placements WHERE employee_id = $1 AND end_date IS NULL`, f.x); got != "2" {
		t.Fatalf("X's open placements = %s, want 2 (untouched)", got)
	}
	if got := h.auditCount("placement.bulk_move", f.x); got != "0" {
		t.Fatalf("X audit = %s", got)
	}

	// Adding a store does not end anything, so X may get D as well.
	add := h.do(&f.p, http.MethodPost, placementBulkURL, map[string]any{
		"branch_id": f.tokoD, "employee_ids": []string{f.x}, "mode": "add", "step": "preview"})
	expect(t, add, http.StatusOK, "")
	expectBulk(t, add, f.x, "created", nil)
}

// Adding a primary store clears the employee's other overlapping primary
// flags; a delegate must not do that to a store outside the area of someone
// not below them (R6), so such an employee is skipped like on a move.
func TestDelegatePlacementAddPrimaryForeign(t *testing.T) {
	h := newHarness(t)
	f := newPlacementScopeFixture(h, true)
	// X2: secondary at A (in the area), primary at C (outside), not below P.
	x2 := h.employee(emp{name: "Pst71 X2 Utama Luar"})
	h.place(x2, f.tokoC)
	h.placeExtra(x2, f.tokoA)
	// Z2: below P, primary at C only.
	z2 := h.employee(emp{name: "Pst71 Z2 Bawahan Luar", boss: &f.pID})
	h.place(z2, f.tokoC)
	primaryAt := func(id, branch string) string {
		return h.scalar(`SELECT string_agg(is_primary::text, ',') FROM hris.employee_placements
			WHERE employee_id = $1 AND branch_id = $2`, id, branch)
	}

	done := h.do(&f.p, http.MethodPost, placementBulkURL, map[string]any{
		"branch_id": f.tokoD, "employee_ids": []string{x2, z2, f.x}, "mode": "add", "is_primary": true, "step": "commit"})
	expect(t, done, http.StatusOK, "")
	expectBulk(t, done, x2, "skipped", errForeignStore)
	expectBulk(t, done, z2, "created", nil)
	expectBulk(t, done, f.x, "created", nil)
	if got := primaryAt(x2, f.tokoC); got != "true" {
		t.Fatalf("X2's primary at C = %q, want true (untouched)", got)
	}
	if got := primaryAt(x2, f.tokoD); got != "" {
		t.Fatalf("X2 at D = %q, want none", got)
	}
	if got := h.auditCount("placement.bulk_add", x2); got != "0" {
		t.Fatalf("X2 audit = %s", got)
	}
	// A subordinate and an in-area-only employee behave as for HR: the new
	// primary at D takes the flag from the overlapping one.
	if primaryAt(z2, f.tokoC) != "false" || primaryAt(z2, f.tokoD) != "true" {
		t.Fatalf("Z2 C/D primary = %s/%s", primaryAt(z2, f.tokoC), primaryAt(z2, f.tokoD))
	}
	if primaryAt(f.x, f.tokoA) != "false" || primaryAt(f.x, f.tokoD) != "true" {
		t.Fatalf("X A/D primary = %s/%s", primaryAt(f.x, f.tokoA), primaryAt(f.x, f.tokoD))
	}

	// Without is_primary nothing foreign is written, so X2 may get D.
	add := h.do(&f.p, http.MethodPost, placementBulkURL, map[string]any{
		"branch_id": f.tokoD, "employee_ids": []string{x2}, "mode": "add", "step": "preview"})
	expect(t, add, http.StatusOK, "")
	expectBulk(t, add, x2, "created", nil)
}

func TestDelegatePlacementEnd(t *testing.T) {
	h := newHarness(t)
	f := newPlacementScopeFixture(h, true)
	h.placeExtra(f.x, f.tokoC)

	done := h.do(&f.p, http.MethodPost, placementEndURL, map[string]any{
		"employee_ids": []string{f.x, f.y, f.z}, "end_date": "2026-10-05", "step": "commit"})
	expect(t, done, http.StatusOK, "")
	expectBulk(t, done, f.x, "ended", nil, "Pst71 Toko A@2026-10-05")
	expectBulk(t, done, f.y, "skipped", errNoStoreToEnd)
	expectBulk(t, done, f.z, "skipped", errNoStoreToEnd)
	if dt := done.data(); dt["ended"] != 1.0 || dt["skipped"] != 2.0 {
		t.Fatalf("end = %s", done.raw)
	}
	if got := h.scalar(`SELECT COALESCE(end_date::text, '-') FROM hris.employee_placements WHERE employee_id = $1 AND branch_id = $2`, f.x, f.tokoC); got != "-" {
		t.Fatalf("X's store C end = %q, want open", got)
	}
	if got := h.scalar(`SELECT COALESCE(end_date::text, '-') FROM hris.employee_placements WHERE employee_id = $1`, f.y); got != "-" {
		t.Fatalf("Y's store C end = %q, want open", got)
	}

	end := func(body map[string]any) call {
		body["end_date"], body["step"] = "2026-10-06", "preview"
		return h.do(&f.p, http.MethodPost, placementEndURL, body)
	}
	expect(t, end(map[string]any{"employee_ids": []string{f.w}}), http.StatusForbidden, "1 karyawan di luar wilayah Anda")
	expect(t, end(map[string]any{"employee_ids": []string{f.y}, "branch_id": f.tokoC}), http.StatusForbidden, errOutsideScope)
	// An own store as branch_id is fine, in any letter case.
	ok := end(map[string]any{"employee_ids": []string{f.pID}, "branch_id": strings.ToUpper(f.tokoA)})
	expect(t, ok, http.StatusOK, "")
	expectBulk(t, ok, f.pID, "ended", nil, "Pst71 Toko A@2026-10-06")
}

func TestDelegatePlacementNoRight(t *testing.T) {
	h := newHarness(t)
	f := newPlacementScopeFixture(h, false)
	// Each request is one the handler itself would answer differently (400
	// on bad input, 200 otherwise), so the 403 comes from the route wrapper.
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, placementSearchURL + "?branch_id=bukan-uuid", nil},
		{http.MethodGet, placementSearchURL, nil},
		{http.MethodGet, placementIDsURL + "?status=bukan-status", nil},
		{http.MethodGet, placementIDsURL, nil},
		{http.MethodPost, placementBulkURL, map[string]any{}},
		{http.MethodPost, placementBulkURL, map[string]any{"branch_id": f.tokoD, "employee_ids": []string{f.x}, "mode": "move", "step": "preview"}},
		{http.MethodPost, placementEndURL, map[string]any{}},
		{http.MethodPost, placementEndURL, map[string]any{"employee_ids": []string{f.x}, "end_date": "2026-10-05", "step": "preview"}},
	} {
		expect(t, h.do(&f.p, c.method, c.path, c.body), http.StatusForbidden, errNoPlaceRight)
	}

	// Someone who is neither HR nor a delegate keeps the usual answer.
	other, _ := h.staff("pos", nil, true)
	expect(t, h.do(&other, http.MethodGet, placementSearchURL, nil), http.StatusForbidden, errNoPerm)
}

func TestDelegatePlacementStale(t *testing.T) {
	h := newHarness(t)
	f := newPlacementScopeFixture(h, true)
	body := map[string]any{"branch_id": f.tokoD, "employee_ids": []string{f.x}, "mode": "move", "step": "preview"}
	expect(t, h.do(&f.p, http.MethodPost, placementBulkURL, body), http.StatusOK, "")

	// HR moves X to C (outside P's area) in the meantime.
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, false)
	expect(t, h.do(&hr, http.MethodPost, placementBulkURL, map[string]any{
		"branch_id": f.tokoC, "employee_ids": []string{f.x}, "mode": "move", "step": "commit"}), http.StatusOK, "")

	body["step"] = "commit"
	expect(t, h.do(&f.p, http.MethodPost, placementBulkURL, body), http.StatusForbidden, "1 karyawan di luar wilayah Anda")
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_placements WHERE employee_id = $1 AND branch_id = $2`, f.x, f.tokoD); got != "0" {
		t.Fatalf("X at D = %s, want 0", got)
	}
}
