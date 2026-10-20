package hris

import (
	"os"
	"path/filepath"
	"testing"

	"nuhabit/backend/internal/modules/hris/domain"
)

// Toko helper h.branch berkoordinat -6.9, 107.6; -6.903 berjarak ±334 m.
var farFromStore = map[string]any{"latitude": -6.903, "longitude": 107.6, "accuracy": 20}

func clockInBody(photo string, loc map[string]any, notes string) map[string]any {
	body := map[string]any{"action": "clock-in", "photo": photo}
	if loc != nil {
		body["clock_in_location"] = loc
	}
	if notes != "" {
		body["notes"] = notes
	}
	return body
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestClockInOutsideAreaNeedsNote(t *testing.T) {
	h, dir := newFileHarness(t)
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Geofence"})
	store := h.branch("PVJ Geofence")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date)
		VALUES ($1, $2, true, '2026-01-01')`, own, store)
	photo := dataURL("image/png", pngBytes(t))

	// Di luar area tanpa keterangan: 422, belum ada absensi dan selfie.
	out := h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, ""))
	expect(t, out, 422, "Anda di luar area PVJ Geofence (±334 m, batas 100 m). Isi keterangan minimal 5 karakter untuk tetap absen.")
	det := out.body["details"].(map[string]any)
	if det["code"] != "geofence_note_required" || det["distance_m"] != 334.0 || det["radius_m"] != 100.0 ||
		det["branch_name"] != "PVJ Geofence" {
		t.Fatalf("details = %s", out.raw)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.attendance WHERE employee_id = $1`, own); n != "0" {
		t.Fatalf("attendance rows after 422 = %s", n)
	}
	if n := countFiles(t, dir); n != 0 {
		t.Fatalf("selfie files after 422 = %d", n)
	}
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, " abc ")), 422, "")

	// Dengan keterangan: tercatat Invalid + jarak + keterangan.
	in := h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, "Ambil stok di gudang"))
	expect(t, in, 200, "")
	d := in.data()
	if d["clock_in_geofence"] != "invalid" || d["clock_in_distance_m"] != 334.0 || d["notes"] != "Ambil stok di gudang" ||
		d["branch_id"] != store || d["status"] != "present" {
		t.Fatalf("clock-in = %s", in.raw)
	}
	if n := countFiles(t, dir); n != 1 {
		t.Fatalf("selfie files after the resend = %d", n)
	}

	// Clock-out di dalam area: Valid, tanpa keterangan.
	co := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-out", "attendance_id": d["id"],
		"photo": photo, "clock_out_location": map[string]any{"latitude": -6.9005, "longitude": 107.6}})
	expect(t, co, 200, "")
	if c := co.data(); c["clock_out_geofence"] != "valid" || c["clock_out_distance_m"] != 56.0 ||
		c["clock_in_geofence"] != "invalid" {
		t.Fatalf("clock-out = %s", co.raw)
	}
}

func TestClockOutOutsideAreaNeedsNote(t *testing.T) {
	h, _ := newFileHarness(t)
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Pulang Jauh"})
	store := h.branch("Kopo Geofence")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date)
		VALUES ($1, $2, true, '2026-01-01')`, own, store)
	photo := dataURL("image/png", pngBytes(t))
	in := h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo,
		map[string]any{"latitude": -6.9001, "longitude": 107.6}, ""))
	expect(t, in, 200, "")
	id := in.data()["id"]
	body := map[string]any{"action": "clock-out", "attendance_id": id, "photo": photo, "clock_out_location": farFromStore}
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", body), 422,
		"Anda di luar area Kopo Geofence (±334 m, batas 100 m). Isi keterangan minimal 5 karakter untuk tetap absen.")
	body["notes"] = "Antar barang ke pelanggan"
	co := h.do(&staff, "POST", "/api/hris/attendance", body)
	expect(t, co, 200, "")
	if c := co.data(); c["clock_out_geofence"] != "invalid" || c["clock_out_distance_m"] != 334.0 ||
		c["notes"] != "Antar barang ke pelanggan" {
		t.Fatalf("clock-out = %s", co.raw)
	}
}

func TestClockGeofenceUnchecked(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", nil, true)
	noCoord, noCoordID := h.staff("pos", nil, true, emp{name: "Toko Tanpa Koordinat"})
	noGPS, noGPSID := h.staff("pos", nil, true, emp{name: "HP Tanpa GPS"})
	lone, _ := h.staff("pos", nil, true, emp{name: "Tanpa Penempatan Geo"})
	_, byHRID := h.staff("pos", nil, true, emp{name: "Diabsenkan HRD"})
	bare, store := h.branch("Toko Kosong"), h.branch("Toko Normal")
	h.exec(`UPDATE configuration.branches SET lat = NULL, lng = NULL WHERE id = $1`, bare)
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES
		($1, $3, true, '2026-01-01'), ($2, $4, true, '2026-01-01')`, noCoordID, noGPSID, bare, store)
	photo := dataURL("image/png", pngBytes(t))

	cases := []struct {
		name string
		call call
	}{
		{"toko tanpa koordinat", h.do(&noCoord, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, ""))},
		{"perangkat tanpa GPS", h.do(&noGPS, "POST", "/api/hris/attendance", clockInBody(photo, nil, ""))},
		{"tanpa penempatan", h.do(&lone, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, ""))},
	}
	for _, c := range cases {
		expect(t, c.call, 200, "")
		if d := c.call.data(); d["clock_in_geofence"] != "unchecked" || d["clock_in_distance_m"] != nil {
			t.Fatalf("%s = %s", c.name, c.call.raw)
		}
	}

	// HRD mencatat untuk karyawan lain dari lokasi HRD: tidak dinilai.
	byHR := h.do(&hr, "POST", "/api/hris/attendance", map[string]any{"action": "clock-in", "photo": photo,
		"employee_id": byHRID, "branch_id": store, "clock_in_location": farFromStore})
	expect(t, byHR, 200, "")
	if d := byHR.data(); d["clock_in_geofence"] != nil || d["clock_in_distance_m"] != nil {
		t.Fatalf("HR on behalf = %s", byHR.raw)
	}
	byHROut := h.do(&hr, "POST", "/api/hris/attendance", map[string]any{"action": "clock-out", "photo": photo,
		"attendance_id": byHR.data()["id"], "clock_out_location": farFromStore})
	expect(t, byHROut, 200, "")
	if d := byHROut.data(); d["clock_out_geofence"] != nil || d["clock_out_distance_m"] != nil {
		t.Fatalf("HR clock-out on behalf = %s", byHROut.raw)
	}
}

// P5: an employee clocking themselves is judged on the server's today; a
// body date can neither move the attendance row nor pick another day for
// the geofence.
func TestSelfClockIgnoresBodyDate(t *testing.T) {
	h, _ := newFileHarness(t)
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Tanggal Palsu"})
	store := h.branch("Dago Geofence")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date)
		VALUES ($1, $2, true, '2026-01-01')`, own, store)
	photo := dataURL("image/png", pngBytes(t))

	body := clockInBody(photo, farFromStore, "")
	body["date"] = "2026-09-17"
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", body), 422, "")

	body["notes"] = "Ambil stok di gudang"
	in := h.do(&staff, "POST", "/api/hris/attendance", body)
	expect(t, in, 200, "")
	if want := domain.TodayWIB(h.svc.now()); in.data()["date"] != want+"T00:00:00.000Z" {
		t.Fatalf("attendance date = %v, want server today %s", in.data()["date"], want)
	}
	if n := h.scalar(`SELECT count(*)::text FROM hris.attendance WHERE employee_id = $1 AND date = '2026-09-17'`, own); n != "0" {
		t.Fatalf("rows on the body date = %s", n)
	}
}

func TestGeofenceInRosterAndList(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", nil, true)
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Rekap Geo"})
	store := h.branch("Toko Rekap Geo")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date)
		VALUES ($1, $2, true, '2026-01-01')`, own, store)
	photo := dataURL("image/png", pngBytes(t))
	expect(t, h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, "Belanja perlengkapan")), 200, "")

	roster := h.do(&hr, "GET", "/api/hris/attendance/daily-roster?date=2026-10-05", nil)
	expect(t, roster, 200, "")
	var att map[string]any
	for _, e := range roster.data()["employees"].([]any) {
		if row := e.(map[string]any); row["employee_id"] == own {
			att, _ = row["attendance"].(map[string]any)
		}
	}
	if att == nil || att["clock_in_geofence"] != "invalid" || att["clock_in_distance_m"] != 334.0 ||
		att["clock_out_geofence"] != nil || att["notes"] != "Belanja perlengkapan" {
		t.Fatalf("roster attendance = %v", att)
	}

	list := h.do(&hr, "GET", "/api/hris/attendance?employee_id="+own, nil)
	expect(t, list, 200, "")
	if row := list.list()[0].(map[string]any); row["clock_in_geofence"] != "invalid" || row["clock_in_distance_m"] != 334.0 {
		t.Fatalf("list = %s", list.raw)
	}
}
