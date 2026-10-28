package hris

import (
	"slices"
	"testing"
)

// fixedNow = Senin 2026-10-05 WIB: "kemarin" = 2026-10-04.

// bulkResult is the result row of employee id in a bulk response.
func bulkResult(t *testing.T, c call, id string) map[string]any {
	t.Helper()
	for _, r := range c.data()["results"].([]any) {
		if m := r.(map[string]any); m["employee_id"] == id {
			return m
		}
	}
	t.Fatalf("no result for %s in %s", id, c.raw)
	return nil
}

// expectBulk checks one employee's outcome, reason and ended stores
// ("branch_name@end_date").
func expectBulk(t *testing.T, c call, id, outcome string, reason any, ended ...string) {
	t.Helper()
	r := bulkResult(t, c, id)
	got := []string{}
	for _, e := range r["ended"].([]any) {
		m := e.(map[string]any)
		got = append(got, m["branch_name"].(string)+"@"+m["end_date"].(string))
	}
	if r["outcome"] != outcome || r["reason"] != reason || !slices.Equal(got, append([]string{}, ended...)) {
		t.Fatalf("result %s = %v, want %s %v %v", id, r, outcome, reason, ended)
	}
}

func TestBulkPlacementMovePreviewCommit(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, false)
	x, y := h.branch("Bm70 Toko X"), h.branch("Bm70 Toko Y")
	a := h.employee(emp{name: "Bm70 Ani"})
	b := h.employee(emp{name: "Bm70 Budi"})
	c := h.employee(emp{name: "Bm70 Cici"})
	d := h.employee(emp{name: "Bm70 Dodi"})
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES
		($1, $4, true, '2026-01-01'), ($2, $5, true, '2026-01-01'), ($3, $4, true, '2026-11-01')`, a, c, d, x, y)
	ids := []string{a, b, c, d}
	snapshot := func() string {
		return h.scalar(`SELECT string_agg(employee_id::text || branch_id::text || is_primary::text || start_date::text ||
			COALESCE(end_date::text, '-'), ',' ORDER BY employee_id, start_date) FROM hris.employee_placements
			WHERE employee_id = ANY($1::text[]::uuid[])`, ids)
	}
	before := snapshot()
	audits := func(action string) string {
		return h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = $1 AND entity_id = ANY($2::text[])`, action, ids)
	}
	body := map[string]any{"branch_id": y, "employee_ids": ids, "mode": "move", "step": "preview"}

	expect(t, h.do(&hr, "POST", "/api/hris/employee-placements/bulk", map[string]any{
		"branch_id": y, "employee_ids": ids, "mode": "pindah"}), 400, "")
	expect(t, h.do(&hr, "POST", "/api/hris/employee-placements/bulk", map[string]any{
		"branch_id": y, "employee_ids": []string{}, "mode": "move"}), 400, "Pilih minimal satu karyawan")

	check := func(res call, step string) {
		t.Helper()
		expect(t, res, 200, "")
		if dt := res.data(); dt["step"] != step || dt["moved"] != 1.0 || dt["created"] != 1.0 || dt["skipped"] != 2.0 ||
			res.body["message"] != "1 dipindahkan, 1 ditambahkan, 2 dilewati" {
			t.Fatalf("%s = %s", step, res.raw)
		}
		expectBulk(t, res, a, "moved", nil, "Bm70 Toko X@2026-10-04")
		expectBulk(t, res, b, "created", nil)
		expectBulk(t, res, c, "skipped", "Sudah di toko ini")
		expectBulk(t, res, d, "skipped", "Punya penempatan mulai 01/11/2026 — ubah lewat Kelola")
		if r := bulkResult(t, res, a); r["name"] != "Bm70 Ani" || r["nip"] == nil {
			t.Fatalf("employee fields = %v", r)
		}
	}

	// Preview: the same plan, nothing written.
	check(h.do(&hr, "POST", "/api/hris/employee-placements/bulk", body), "preview")
	if after := snapshot(); after != before {
		t.Fatalf("preview wrote placements:\n%s\n%s", before, after)
	}
	if n := audits("placement.bulk_move"); n != "0" {
		t.Fatalf("preview audit rows = %s", n)
	}

	// Commit.
	body["step"] = "commit"
	check(h.do(&hr, "POST", "/api/hris/employee-placements/bulk", body), "commit")
	if got := h.scalar(`SELECT end_date::text FROM hris.employee_placements WHERE employee_id = $1 AND branch_id = $2`, a, x); got != "2026-10-04" {
		t.Fatalf("A's store X end = %q", got)
	}
	if got := h.scalar(`SELECT string_agg(branch_id::text || '|' || start_date::text, ',') FROM hris.employee_placements
		WHERE employee_id = $1 AND is_primary AND (end_date IS NULL OR end_date >= '2026-10-05')`, a); got != y+"|2026-10-05" {
		t.Fatalf("A's running primary = %q", got)
	}
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_placements WHERE employee_id = $1 AND branch_id = $2 AND is_primary`, b, y); got != "1" {
		t.Fatalf("B's primary at Y = %s", got)
	}
	if n := audits("placement.bulk_move"); n != "2" {
		t.Fatalf("move audit rows = %s", n)
	}
	if got := h.scalar(`SELECT before::text || ' ' || after::text FROM audit.audit_log WHERE action = 'placement.bulk_move' AND entity_id = $1`, a); got !=
		`{"branches": ["Bm70 Toko X"]} {"branch": "Bm70 Toko Y", "is_primary": true, "start_date": "2026-10-05"}` {
		t.Fatalf("A's audit = %s", got)
	}

	// Add: A already at Y (skipped), C gets a second, non-primary store.
	add := h.do(&hr, "POST", "/api/hris/employee-placements/bulk", map[string]any{
		"branch_id": x, "employee_ids": []string{a, c, c}, "mode": "add", "step": "commit", "start_date": "2026-10-04"})
	expect(t, add, 200, "")
	if dt := add.data(); dt["created"] != 1.0 || dt["skipped"] != 1.0 || dt["moved"] != 0.0 || add.body["message"] != "1 ditambahkan, 1 dilewati" {
		t.Fatalf("add = %s", add.raw)
	}
	expectBulk(t, add, a, "skipped", "Sudah ditempatkan di toko ini")
	expectBulk(t, add, c, "created", nil)
	if got := h.scalar(`SELECT string_agg(branch_id::text || '|' || is_primary::text, ',' ORDER BY is_primary) FROM hris.employee_placements
		WHERE employee_id = $1`, c); got != x+"|false,"+y+"|true" {
		t.Fatalf("C's placements = %q", got)
	}
	if n := audits("placement.bulk_add"); n != "1" {
		t.Fatalf("add audit rows = %s", n)
	}
	expect(t, h.do(&hr, "POST", "/api/hris/employee-placements/bulk", map[string]any{
		"branch_id": x, "employee_ids": []string{a}, "mode": "add", "start_date": "2026-10-05", "end_date": "2026-10-01"}), 400,
		"Tanggal selesai tidak boleh sebelum tanggal mulai")
}

// R6: without mode the endpoint answers as before.
func TestBulkPlacementLegacyUnchanged(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, false)
	x := h.branch("Bl70 Toko X")
	a, b := h.employee(emp{name: "Bl70 Ani"}), h.employee(emp{name: "Bl70 Budi"})
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES ($1, $2, true, '2026-01-01')`, a, x)
	res := h.do(&hr, "POST", "/api/hris/employee-placements/bulk", map[string]any{
		"branch_id": x, "employee_ids": []string{a, b}, "start_date": "2026-04-01"})
	expect(t, res, 200, "")
	if res.body["message"] != "1 penempatan ditambahkan, 1 dilewati (sudah ditempatkan / tidak valid)" {
		t.Fatalf("message = %s", res.raw)
	}
	if got := keys(t, res.raw, "data"); !slices.Equal(got, []string{"created", "skipped"}) || res.data()["created"] != 1.0 ||
		res.data()["skipped"] != 1.0 {
		t.Fatalf("legacy data = %s", res.raw)
	}
}

func TestBulkPlacementEnd(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, false)
	x, y := h.branch("Be70 Toko X"), h.branch("Be70 Toko Y")
	a := h.employee(emp{name: "Be70 Ani"})  // X + Y running: only X ends
	b := h.employee(emp{name: "Be70 Budi"}) // X starts after the end date
	c := h.employee(emp{name: "Be70 Cici"}) // not at X
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES
		($1, $4, true, '2026-01-01'), ($1, $5, false, '2026-02-01'), ($2, $4, true, '2026-10-20'), ($3, $5, true, '2026-01-01')`,
		a, b, c, x, y)
	ids := []string{a, b, c}
	path := "/api/hris/employee-placements/bulk-end"

	expect(t, h.do(&hr, "POST", path, map[string]any{"employee_ids": ids, "step": "commit"}), 400, "")
	expect(t, h.do(&hr, "POST", path, map[string]any{"employee_ids": []string{}, "end_date": "2026-10-10", "step": "commit"}), 400,
		"Pilih minimal satu karyawan")

	check := func(res call, step string) {
		t.Helper()
		expect(t, res, 200, "")
		if dt := res.data(); dt["step"] != step || dt["ended"] != 1.0 || dt["skipped"] != 2.0 ||
			res.body["message"] != "1 diakhiri, 2 dilewati" {
			t.Fatalf("%s = %s", step, res.raw)
		}
		expectBulk(t, res, a, "ended", nil, "Be70 Toko X@2026-10-10")
		expectBulk(t, res, b, "skipped", "Penempatan mulai sesudah tanggal akhir")
		expectBulk(t, res, c, "skipped", "Tidak ada penempatan berjalan")
	}
	body := map[string]any{"employee_ids": ids, "end_date": "2026-10-10", "branch_id": x, "step": "preview"}
	check(h.do(&hr, "POST", path, body), "preview")
	if got := h.scalar(`SELECT count(*)::text FROM hris.employee_placements WHERE employee_id = ANY($1::text[]::uuid[]) AND end_date IS NOT NULL`, ids); got != "0" {
		t.Fatalf("preview ended %s placements", got)
	}

	body["step"] = "commit"
	check(h.do(&hr, "POST", path, body), "commit")
	if got := h.scalar(`SELECT string_agg(branch_id::text || '|' || COALESCE(end_date::text, '-'), ',' ORDER BY start_date)
		FROM hris.employee_placements WHERE employee_id = $1`, a); got != x+"|2026-10-10,"+y+"|-" {
		t.Fatalf("A's placements = %q", got)
	}
	if got := h.scalar(`SELECT end_date::text FROM hris.employee_placements WHERE employee_id = $1`, b); got != "" {
		t.Fatalf("B's future placement ended %q", got)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'placement.bulk_end' AND entity_id = ANY($1::text[])`, ids); n != "1" {
		t.Fatalf("end audit rows = %s", n)
	}

	// Without branch_id every running placement ends.
	all := h.do(&hr, "POST", path, map[string]any{"employee_ids": []string{a}, "end_date": "2026-10-08", "step": "commit"})
	expect(t, all, 200, "")
	expectBulk(t, all, a, "ended", nil, "Be70 Toko X@2026-10-08", "Be70 Toko Y@2026-10-08")
}
