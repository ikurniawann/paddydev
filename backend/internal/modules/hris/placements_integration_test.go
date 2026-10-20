package hris

import (
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// branch inserts a company and an active store, returning the store id.
func (h *harness) branch(name string) string {
	h.t.Helper()
	sfx := testutil.RandomHex(4)
	holding := h.scalar(`INSERT INTO configuration.holdings (name, code) VALUES ($1, $2) RETURNING id::text`,
		"Holding "+sfx, "H-"+sfx)
	company := h.scalar(`INSERT INTO configuration.companies (holding_id, name, code) VALUES ($1, $2, $3) RETURNING id::text`,
		holding, "Co "+sfx, "CO-"+sfx)
	return h.scalar(`INSERT INTO configuration.branches (company_id, name, code, lat, lng)
		VALUES ($1, $2, $3, -6.9, 107.6) RETURNING id::text`, company, name, "BR-"+sfx)
}

func TestPlacements(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, true)
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Multi Toko"})
	other, otherID := h.staff("pos", nil, true, emp{name: "Crew Lain"})
	pvj, kopo := h.branch("Toko PVJ Test"), h.branch("Toko Kopo Test")
	base := "/api/hris/employees/" + own + "/placements"

	// Guards: placements are managed by HR menus; an employee reads only their own.
	expect(t, h.do(&staff, "POST", base, map[string]any{"branch_id": pvj}), 403, "")
	expect(t, h.do(&other, "GET", base, nil), 403, "")
	expect(t, h.do(&staff, "GET", "/api/hris/employee-placements", nil), 403, "")

	expect(t, h.do(&hr, "POST", base, map[string]any{}), 400, "Toko wajib dipilih")
	first := h.do(&hr, "POST", base, map[string]any{"branch_id": pvj, "is_primary": true, "start_date": "2026-01-01"})
	expect(t, first, 200, "")
	if d := first.data(); d["branch_name"] != "Toko PVJ Test" || d["is_primary"] != true || d["end_date"] != nil {
		t.Fatalf("create = %s", first.raw)
	}
	pvjID := first.data()["id"].(string)

	// The same store with an overlapping range is refused by the constraint.
	expect(t, h.do(&hr, "POST", base, map[string]any{"branch_id": pvj, "start_date": "2026-06-01"}), 400,
		"Karyawan sudah ditempatkan di toko ini pada rentang tanggal yang sama")
	expect(t, h.do(&hr, "POST", base, map[string]any{"branch_id": kopo, "start_date": "2026-02-01", "end_date": "2026-01-01"}), 400,
		"Tanggal selesai tidak boleh sebelum tanggal mulai")

	// A second primary takes the flag from the first.
	second := h.do(&hr, "POST", base, map[string]any{"branch_id": kopo, "is_primary": true, "start_date": "2026-03-01"})
	expect(t, second, 200, "")
	kopoID := second.data()["id"].(string)
	if h.scalar(`SELECT is_primary::text FROM hris.employee_placements WHERE id = $1`, pvjID) != "false" {
		t.Fatal("old primary kept its flag")
	}
	// ...and moving it back works through PATCH.
	expect(t, h.do(&hr, "PATCH", "/api/hris/employee-placements/"+pvjID, map[string]any{"is_primary": true}), 200, "")
	if h.scalar(`SELECT is_primary::text FROM hris.employee_placements WHERE id = $1`, kopoID) != "false" {
		t.Fatal("PATCH primary did not clear the other one")
	}

	mine := h.do(&staff, "GET", base, nil)
	expect(t, mine, 200, "")
	if len(mine.list()) != 2 {
		t.Fatalf("own placements = %s", mine.raw)
	}

	// Overview, filtered by store and unplaced.
	over := h.do(&hr, "GET", "/api/hris/employee-placements?branch_id="+kopo, nil)
	expect(t, over, 200, "")
	if len(over.list()) != 1 || over.list()[0].(map[string]any)["employee_id"] != own ||
		len(over.list()[0].(map[string]any)["placements"].([]any)) != 2 {
		t.Fatalf("overview = %s", over.raw)
	}
	unplaced := h.do(&hr, "GET", "/api/hris/employee-placements?unplaced=true&q=Crew%20Lain", nil)
	if len(unplaced.list()) != 1 || unplaced.list()[0].(map[string]any)["employee_id"] != otherID {
		t.Fatalf("unplaced = %s", unplaced.raw)
	}

	// Bulk: one new, one already placed (skipped).
	bulk := h.do(&hr, "POST", "/api/hris/employee-placements/bulk", map[string]any{
		"branch_id": kopo, "employee_ids": []string{otherID, own}, "start_date": "2026-04-01"})
	expect(t, bulk, 200, "")
	if d := bulk.data(); d["created"] != 1.0 || d["skipped"] != 1.0 {
		t.Fatalf("bulk = %s", bulk.raw)
	}

	// Clock-in choices: both stores, primary (PVJ) first and default.
	choice := h.do(&staff, "GET", "/api/hris/me/placements?date=2026-10-05", nil)
	expect(t, choice, 200, "")
	if d := choice.data(); d["default_branch_id"] != pvj || len(d["placements"].([]any)) != 2 {
		t.Fatalf("me/placements = %s", choice.raw)
	}
	if p := choice.data()["placements"].([]any)[0].(map[string]any); p["lat"] != "-6.900000" {
		t.Fatalf("branch coordinates = %v", p)
	}

	// HR attendance correction can set and clear the store.
	att := h.scalar(`INSERT INTO hris.attendance (employee_id, date, clock_in, status) VALUES ($1, '2026-10-01', now(), 'present')
		RETURNING id::text`, own)
	expect(t, h.do(&hr, "PUT", "/api/hris/attendance/"+att, map[string]any{"branch_id": "ffffffff-ffff-ffff-ffff-ffffffffffff",
		"reason": "Koreksi toko"}), 400, "Toko tidak ditemukan")
	expect(t, h.do(&hr, "PUT", "/api/hris/attendance/"+att, map[string]any{"branch_id": kopo, "reason": "Koreksi toko"}), 200, "")
	list := h.do(&hr, "GET", "/api/hris/attendance?employee_id="+own+"&branch_id="+kopo, nil)
	if len(list.list()) != 1 || list.list()[0].(map[string]any)["branch"].(map[string]any)["name"] != "Toko Kopo Test" {
		t.Fatalf("attendance by store = %s", list.raw)
	}
	expect(t, h.do(&hr, "PUT", "/api/hris/attendance/"+att, map[string]any{"branch_id": nil, "reason": "Koreksi toko"}), 200, "")
	if n := len(h.do(&hr, "GET", "/api/hris/attendance?employee_id="+own+"&branch_id=none", nil).list()); n != 1 {
		t.Fatalf("attendance without store = %d", n)
	}

	expect(t, h.do(&hr, "DELETE", "/api/hris/employee-placements/"+kopoID+"?reason=Penempatan%20berakhir", nil), 200, "")
	expect(t, h.do(&hr, "DELETE", "/api/hris/employee-placements/"+kopoID+"?reason=Penempatan%20berakhir", nil), 404, "Penempatan tidak ditemukan")

	branches := h.do(&staff, "GET", "/api/hris/branches", nil)
	expect(t, branches, 200, "")
	if len(branches.list()) < 2 {
		t.Fatalf("branches = %s", branches.raw)
	}
}

func TestClockInStore(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", nil, true)
	staff, own := h.staff("pos", nil, true, emp{name: "Clock Toko"})
	lone, loneID := h.staff("pos", nil, true, emp{name: "Tanpa Penempatan"})
	pvj, kopo, hq := h.branch("PVJ Clock"), h.branch("Kopo Clock"), h.branch("HQ Clock")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES
		($1, $2, true, '2026-01-01'), ($1, $3, false, '2026-01-01')`, own, pvj, kopo)
	photo := dataURL("image/png", pngBytes(t))

	// Not one of the employee's stores.
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "photo": photo, "branch_id": hq}),
		400, "Toko yang dipilih bukan penempatan aktif karyawan ini")
	in := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "photo": photo, "branch_id": kopo})
	expect(t, in, 200, "")
	if in.data()["branch_id"] != kopo {
		t.Fatalf("clock-in store = %s", in.raw)
	}

	// No store sent: the primary placement. Without placements: no store.
	h.exec(`DELETE FROM hris.attendance WHERE employee_id = $1`, own)
	def := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "photo": photo})
	if def.data()["branch_id"] != pvj {
		t.Fatalf("default store = %s", def.raw)
	}
	none := h.do(&lone, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "photo": photo})
	expect(t, none, 200, "")
	if none.data()["branch_id"] != nil {
		t.Fatalf("no placement store = %s", none.raw)
	}

	// HR records a store for an employee without placements.
	h.exec(`DELETE FROM hris.attendance WHERE employee_id = $1`, loneID)
	manual := h.do(&hr, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "photo": photo,
		"employee_id": loneID, "branch_id": hq})
	expect(t, manual, 200, "")
	if manual.data()["branch_id"] != hq || manual.data()["employee_id"] != loneID {
		t.Fatalf("HR manual store = %s", manual.raw)
	}
}
