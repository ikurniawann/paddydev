package hris

import "testing"

func TestBranchLocation(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, true)
	staff, _ := h.staff("pos", nil, true)
	store := h.branch("Toko Lokasi")
	path := "/api/hris/branches/" + store + "/location"

	expect(t, h.do(&staff, "PUT", path, map[string]any{"lat": -6.88, "lng": 107.59}), 403, "")
	expect(t, h.do(&hr, "PUT", path, map[string]any{"lat": -6.88}), 400,
		"Latitude dan longitude wajib diisi berdua atau dikosongkan berdua")
	expect(t, h.do(&hr, "PUT", path, map[string]any{"lat": 0, "lng": 0}), 400,
		"Koordinat tidak valid (latitude -90..90, longitude -180..180, bukan 0,0)")
	expect(t, h.do(&hr, "PUT", path, map[string]any{"lat": -6.88, "lng": 107.59, "geofence_radius_m": 10}), 400,
		"Radius absensi 25–1000 meter")
	expect(t, h.do(&hr, "PUT", "/api/hris/branches/ffffffff-ffff-ffff-ffff-ffffffffffff/location",
		map[string]any{"lat": -6.88, "lng": 107.59}), 404, "Toko tidak ditemukan")
	expect(t, h.do(&hr, "PUT", "/api/hris/branches/bukan-uuid/location", map[string]any{}), 400, "ID toko tidak valid")

	saved := h.do(&hr, "PUT", path, map[string]any{"lat": -6.8889, "lng": 107.596, "geofence_radius_m": 250})
	expect(t, saved, 200, "")
	if d := saved.data(); d["lat"] != "-6.888900" || d["lng"] != "107.596000" || d["geofence_radius_m"] != 250.0 ||
		d["name"] != "Toko Lokasi" {
		t.Fatalf("saved = %s", saved.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log
		WHERE action = 'branch_location.update' AND entity_id = $1`, store); n != "1" {
		t.Fatalf("audit rows = %s", n)
	}
	if before := h.scalar(`SELECT before->>'lat' FROM audit.audit_log
		WHERE action = 'branch_location.update' AND entity_id = $1`, store); before != "-6.900000" {
		t.Fatalf("audit before lat = %q", before)
	}

	// Daftar toko untuk pemilih memuat radius.
	list := h.do(&staff, "GET", "/api/hris/branches", nil)
	found := false
	for _, b := range list.list() {
		if row := b.(map[string]any); row["id"] == store {
			found = row["geofence_radius_m"] == 250.0
		}
	}
	if !found {
		t.Fatalf("branches = %s", list.raw)
	}

	// Dikosongkan: absen di toko ini kembali "Belum dicek".
	cleared := h.do(&hr, "PUT", path, map[string]any{"lat": nil, "lng": nil, "geofence_radius_m": nil})
	expect(t, cleared, 200, "")
	if d := cleared.data(); d["lat"] != nil || d["lng"] != nil || d["geofence_radius_m"] != nil {
		t.Fatalf("cleared = %s", cleared.raw)
	}
}
