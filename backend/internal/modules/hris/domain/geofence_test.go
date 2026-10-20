package domain

import "testing"

func f64p(v float64) *float64 { return &v }
func intp(v int) *int         { return &v }

func TestDistanceMeters(t *testing.T) {
	if d := DistanceMeters(-6.9, 107.6, -6.901, 107.6); d < 111.19 || d > 111.20 {
		t.Fatalf("0,001° lintang = %v m, want ±111,19", d)
	}
	eq(t, DistanceMeters(-6.9, 107.6, -6.9, 107.6), 0.0)
}

func TestGeofence(t *testing.T) {
	store := func(p *GeoPoint) GeofenceInput {
		return GeofenceInput{BranchLat: f64p(-6.9), BranchLng: f64p(107.6), Point: p}
	}
	check := func(name string, in GeofenceInput, status string, dist *int) {
		t.Helper()
		got := Geofence(in)
		if got.Status != status {
			t.Errorf("%s: status %q, want %q", name, got.Status, status)
		}
		eq(t, got.DistanceM, dist)
	}
	check("dalam radius", store(&GeoPoint{Lat: -6.9005, Lng: 107.6}), GeofenceValid, intp(56))
	check("tepat 100 m", store(&GeoPoint{Lat: -6.9009, Lng: 107.6}), GeofenceValid, intp(100))
	check("111 m tanpa akurasi", store(&GeoPoint{Lat: -6.901, Lng: 107.6}), GeofenceInvalid, intp(111))
	check("167 m akurasi 80", store(&GeoPoint{Lat: -6.9015, Lng: 107.6, Accuracy: f64p(80)}), GeofenceValid, intp(167))
	check("334 m akurasi 5000", store(&GeoPoint{Lat: -6.903, Lng: 107.6, Accuracy: f64p(5000)}), GeofenceInvalid, intp(334))
	override := store(&GeoPoint{Lat: -6.903, Lng: 107.6})
	override.RadiusM = intp(350)
	check("override 350 m", override, GeofenceValid, intp(334))
	check("tanpa GPS", store(nil), GeofenceUnchecked, nil)
	check("GPS 0,0", store(&GeoPoint{Lat: 0, Lng: 0}), GeofenceUnchecked, nil)
	check("GPS di luar rentang", store(&GeoPoint{Lat: 95, Lng: 107.6}), GeofenceUnchecked, nil)
	check("toko tanpa koordinat", GeofenceInput{Point: &GeoPoint{Lat: -6.903, Lng: 107.6}}, GeofenceUnchecked, nil)
	trip := store(&GeoPoint{Lat: -6.903, Lng: 107.6})
	trip.OnBusinessTrip = true
	check("perjalanan dinas", trip, GeofenceDinas, intp(334))
	check("dinas tanpa koordinat", GeofenceInput{OnBusinessTrip: true}, GeofenceDinas, nil)
	eq(t, Geofence(store(nil)).RadiusM, 100)
	eq(t, Geofence(override).RadiusM, 350)
}

func TestGeofenceNoteMissing(t *testing.T) {
	eq(t, GeofenceNoteMissing(GeofenceInvalid, nil), true)
	eq(t, GeofenceNoteMissing(GeofenceInvalid, sp("  abcd ")), true)
	eq(t, GeofenceNoteMissing(GeofenceInvalid, sp("Ambil stok")), false)
	eq(t, GeofenceNoteMissing(GeofenceValid, nil), false)
	eq(t, GeofenceNoteMissing(GeofenceUnchecked, nil), false)
	eq(t, GeofenceNoteMissing(GeofenceDinas, nil), false)
}
