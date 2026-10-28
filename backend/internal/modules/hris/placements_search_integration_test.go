package hris

import (
	"net/url"
	"slices"
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

// fixedNow = Senin 2026-10-05: jendela "ada jadwal" (R5) = 05..11 Okt.

type placementSearchFixture struct {
	hr                                testutil.Staff
	tokoA, tokoB, tokoC, dept1, dept2 string
	kasir                             string
	ani, budi, cici, dodi, euis, fani string
}

// placementSearchData: 6 karyawan aktif berawalan "Cr70" (+1 nonaktif).
//   - Ani: Shift, tanpa penempatan, tanpa jadwal (dept1).
//   - Budi: Shift, toko A (utama) + B, baris jadwal 07 & 20 Okt (dept1).
//   - Cici: Non-Shift, toko A, tanpa jadwal (dept2).
//   - Dodi: Shift, toko B, baris jadwal 07 Okt, jabatan Kasir (dept2).
//   - Euis: Shift, toko C, pola mingguan berjalan (dept2).
//   - Fani: Shift, toko C + toko A yang sudah berakhir; baris jadwal 12 Okt
//     (di luar jendela) dan pola mingguan yang berakhir 04 Okt (dept2).
func placementSearchData(h *harness) placementSearchFixture {
	h.t.Helper()
	var f placementSearchFixture
	f.hr, _ = h.staff("hrd", []string{"hris.master.placements"}, false)
	f.tokoA, f.tokoB, f.tokoC = h.branch("Cr70 Toko A"), h.branch("Cr70 Toko B"), h.branch("Cr70 Toko C")
	f.dept1, f.dept2 = h.department("Cr70 Dept Satu"), h.department("Cr70 Dept Dua")
	f.kasir = h.scalar(`INSERT INTO hris.positions (title) VALUES ('Cr70 Kasir') RETURNING id::text`)
	f.ani = h.employee(emp{name: "Cr70 Ani Unplaced", dept: &f.dept1})
	f.budi = h.employee(emp{name: "Cr70 Budi Multi", dept: &f.dept1})
	f.cici = h.employee(emp{name: "Cr70 Cici NonShift", dept: &f.dept2})
	f.dodi = h.employee(emp{name: "Cr70 Dodi Hari", dept: &f.dept2})
	f.euis = h.employee(emp{name: "Cr70 Euis Pola", dept: &f.dept2})
	f.fani = h.employee(emp{name: "Cr70 Fani Kosong", dept: &f.dept2})
	h.employee(emp{name: "Cr70 Gugun Nonaktif", inactive: true})
	h.exec(`UPDATE hris.employees SET schedule_type = 'non_shift' WHERE id = $1`, f.cici)
	h.exec(`UPDATE hris.employees SET nip = 'NIP70-DODI', job_title_id = $2 WHERE id = $1`, f.dodi, f.kasir)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, end_date) VALUES
		($1, $5, true, '2026-01-01', NULL), ($1, $6, false, '2026-02-01', NULL),
		($2, $5, true, '2026-01-01', NULL),
		($3, $6, true, '2026-01-01', NULL),
		($4, $7, true, '2026-01-01', NULL),
		($8, $7, true, '2026-01-01', NULL), ($8, $5, false, '2026-01-01', '2026-09-30')`,
		f.budi, f.cici, f.dodi, f.euis, f.tokoA, f.tokoB, f.tokoC, f.fani)
	shift := h.scalar(`INSERT INTO hris.shifts (name, start_time, end_time, late_tolerance_minutes)
		VALUES ('Cr70 Pagi', '08:00', '16:00', 10) RETURNING id::text`)
	h.exec(`INSERT INTO hris.employee_shift_days (employee_id, work_date, shift_id, batch_id) VALUES
		($1, '2026-10-01', $4, gen_random_uuid()), ($1, '2026-10-07', $4, gen_random_uuid()),
		($1, '2026-10-20', $4, gen_random_uuid()),
		($2, '2026-10-07', $4, gen_random_uuid()),
		($3, '2026-10-12', $4, gen_random_uuid())`, f.budi, f.dodi, f.fani, shift)
	h.exec(`INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from, effective_to) VALUES
		($1, 1, $3, '2026-01-01', NULL),
		($2, 1, $3, '2026-01-01', '2026-10-04')`, f.euis, f.fani, shift)
	return f
}

func searchPath(base string, params ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(params); i += 2 {
		v.Set(params[i], params[i+1])
	}
	return base + "?" + v.Encode()
}

func searchRows(t *testing.T, c call) []map[string]any {
	t.Helper()
	expect(t, c, 200, "")
	var out []map[string]any
	for _, r := range c.data()["rows"].([]any) {
		out = append(out, r.(map[string]any))
	}
	return out
}

func rowIDs(rows []map[string]any) []string {
	var ids []string
	for _, r := range rows {
		ids = append(ids, r["employee_id"].(string))
	}
	return ids
}

const placementSearchURL = "/api/hris/employee-placements/search"

func TestPlacementSearch(t *testing.T) {
	h := newHarness(t)
	f := placementSearchData(h)
	hr := f.hr
	search := func(params ...string) call {
		return h.do(&hr, "GET", searchPath(placementSearchURL, append([]string{"q", "Cr70"}, params...)...), nil)
	}

	// All: sorted by name, counts over the same q filter.
	all := search()
	rows := searchRows(t, all)
	if got, want := rowIDs(rows), []string{f.ani, f.budi, f.cici, f.dodi, f.euis, f.fani}; !slices.Equal(got, want) {
		t.Fatalf("rows = %v, want %v (%s)", got, want, all.raw)
	}
	d := all.data()
	if d["total"] != 6.0 || d["page"] != 1.0 || d["page_size"] != 25.0 {
		t.Fatalf("paging = %s", all.raw)
	}
	wantCounts := map[string]any{"all": 6.0, "unplaced": 1.0, "no_schedule": 2.0, "multi": 1.0}
	for k, v := range wantCounts {
		if d["counts"].(map[string]any)[k] != v {
			t.Fatalf("counts[%s] = %v, want %v (%s)", k, d["counts"].(map[string]any)[k], v, all.raw)
		}
	}

	// Row shape and schedule status (R5).
	byID := map[string]map[string]any{}
	for _, r := range rows {
		byID[r["employee_id"].(string)] = r
	}
	for _, k := range []string{"employee_id", "full_name", "nip", "photo_url", "department_name", "position_title", "placements",
		"department_id", "position_id", "schedule_type", "has_schedule", "schedule_until"} {
		if _, ok := byID[f.dodi][k]; !ok {
			t.Fatalf("row lacks %q: %v", k, byID[f.dodi])
		}
	}
	if r := byID[f.dodi]; r["department_id"] != f.dept2 || r["position_id"] != f.kasir || r["position_title"] != "Cr70 Kasir" ||
		r["schedule_type"] != "shift" || r["nip"] != "NIP70-DODI" {
		t.Fatalf("dodi = %v", r)
	}
	schedule := map[string][2]any{
		f.ani:  {false, nil},
		f.budi: {true, "2026-10-20"},
		f.dodi: {true, "2026-10-07"},
		f.euis: {true, nil},
		f.fani: {false, "2026-10-12"},
	}
	for id, want := range schedule {
		if r := byID[id]; r["has_schedule"] != want[0] || r["schedule_until"] != want[1] {
			t.Fatalf("%s has_schedule/schedule_until = %v/%v, want %v", r["full_name"], r["has_schedule"], r["schedule_until"], want)
		}
	}
	if byID[f.cici]["schedule_type"] != "non_shift" {
		t.Fatalf("cici = %v", byID[f.cici])
	}
	if n := len(byID[f.budi]["placements"].([]any)); n != 2 {
		t.Fatalf("budi placements = %d", n)
	}
	if n := len(byID[f.fani]["placements"].([]any)); n != 1 {
		t.Fatalf("fani running placements = %d", n)
	}
	ended := searchRows(t, search("include_ended", "true", "status", "multi"))
	if len(ended) != 1 || ended[0]["employee_id"] != f.budi {
		t.Fatalf("include_ended keeps the running-placement status filter: %v", rowIDs(ended))
	}
	withEnded := searchRows(t, search("include_ended", "true"))
	for _, r := range withEnded {
		if r["employee_id"] == f.fani && len(r["placements"].([]any)) != 2 {
			t.Fatalf("fani with ended = %v", r["placements"])
		}
	}

	// Status chips.
	statuses := map[string][]string{
		"unplaced":    {f.ani},
		"no_schedule": {f.ani, f.fani},
		"multi":       {f.budi},
		"placed":      {f.budi, f.cici, f.dodi, f.euis, f.fani},
	}
	for status, want := range statuses {
		c := search("status", status)
		if got := rowIDs(searchRows(t, c)); !slices.Equal(got, want) {
			t.Fatalf("status=%s rows = %v, want %v", status, got, want)
		}
		if c.data()["total"] != float64(len(want)) || c.data()["counts"].(map[string]any)["all"] != 6.0 {
			t.Fatalf("status=%s total/counts = %s", status, c.raw)
		}
	}

	// Filters narrow both rows and counts.
	dept := search("department_id", f.dept1)
	if got := rowIDs(searchRows(t, dept)); !slices.Equal(got, []string{f.ani, f.budi}) {
		t.Fatalf("department rows = %v", got)
	}
	if c := dept.data()["counts"].(map[string]any); c["all"] != 2.0 || c["unplaced"] != 1.0 || c["no_schedule"] != 1.0 || c["multi"] != 1.0 {
		t.Fatalf("department counts = %v", c)
	}
	if got := rowIDs(searchRows(t, search("position_id", f.kasir))); !slices.Equal(got, []string{f.dodi}) {
		t.Fatalf("position rows = %v", got)
	}
	if got := rowIDs(searchRows(t, search("branch_id", f.tokoA))); !slices.Equal(got, []string{f.budi, f.cici}) {
		t.Fatalf("branch rows = %v (Fani's ended placement excluded)", got)
	}
	byNIP := h.do(&hr, "GET", searchPath(placementSearchURL, "q", "nip70-dodi"), nil)
	if got := rowIDs(searchRows(t, byNIP)); !slices.Equal(got, []string{f.dodi}) {
		t.Fatalf("q by NIP = %v", got)
	}

	// Pagination.
	second := search("page_size", "25", "page", "2")
	if rows := searchRows(t, second); len(rows) != 0 || second.data()["total"] != 6.0 || second.data()["page"] != 2.0 {
		t.Fatalf("page 2 = %s", second.raw)
	}
	if c := search("page_size", "50"); c.data()["page_size"] != 50.0 {
		t.Fatalf("page_size 50 = %s", c.raw)
	}
	expect(t, search("page_size", "30"), 400, "Jumlah per halaman harus 25, 50, atau 100")
	expect(t, search("page", "0"), 400, "Halaman tidak valid")
	expect(t, search("page", "9223372036854775807"), 400, "Halaman tidak valid")
	expect(t, search("status", "aneh"), 400, "Status tidak valid")
	expect(t, search("department_id", "abc"), 400, "ID departemen tidak valid")
	expect(t, search("position_id", "abc"), 400, "ID jabatan tidak valid")
	expect(t, search("branch_id", "abc"), 400, "ID toko tidak valid")

	// The old overview (R6) still answers with every placement listed.
	old := h.do(&hr, "GET", "/api/hris/employee-placements?include_ended=true&q=Cr70", nil)
	expect(t, old, 200, "")
	if len(old.list()) != 6 {
		t.Fatalf("old overview = %s", old.raw)
	}

	// Non-HR.
	staff, _ := h.staff("pos", nil, true, emp{name: "Cr70 Zaki Crew"})
	expect(t, h.do(&staff, "GET", placementSearchURL, nil), 403, "")
}

func TestPlacementIDs(t *testing.T) {
	h := newHarness(t)
	f := placementSearchData(h)
	hr := f.hr
	const base = "/api/hris/employee-placements/ids"

	all := h.do(&hr, "GET", searchPath(base, "q", "Cr70", "page", "2", "page_size", "25"), nil)
	expect(t, all, 200, "")
	d := all.data()
	var ids []string
	for _, id := range d["ids"].([]any) {
		ids = append(ids, id.(string))
	}
	if want := []string{f.ani, f.budi, f.cici, f.dodi, f.euis, f.fani}; !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	if d["total"] != 6.0 || d["truncated"] != false {
		t.Fatalf("ids = %s", all.raw)
	}

	noSchedule := h.do(&hr, "GET", searchPath(base, "q", "Cr70", "status", "no_schedule", "department_id", f.dept2), nil)
	expect(t, noSchedule, 200, "")
	if got := noSchedule.data()["ids"].([]any); len(got) != 1 || got[0] != f.fani || noSchedule.data()["total"] != 1.0 {
		t.Fatalf("no_schedule ids = %s", noSchedule.raw)
	}
	none := h.do(&hr, "GET", searchPath(base, "q", "tidak-ada-yang-cocok"), nil)
	expect(t, none, 200, "")
	if got, ok := none.data()["ids"].([]any); !ok || len(got) != 0 || none.data()["total"] != 0.0 {
		t.Fatalf("empty ids = %s", none.raw)
	}
	expect(t, h.do(&hr, "GET", searchPath(base, "status", "aneh"), nil), 400, "Status tidak valid")

	staff, _ := h.staff("pos", nil, true, emp{name: "Cr70 Zaki Crew"})
	expect(t, h.do(&staff, "GET", base, nil), 403, "")
}
