package domain

import (
	"math"
	"testing"
)

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

// metersNorth is a device point exactly d meters south of the test store
// (-6.9, 107.6): a pure latitude offset, so haversine returns d.
func metersNorth(d float64, acc *float64) *GeoPoint {
	return &GeoPoint{Lat: -6.9 - d/(math.Pi/180*6_371_000), Lng: 107.6, Accuracy: acc}
}

func TestGeofenceBoundaries(t *testing.T) {
	in := func(p *GeoPoint) GeofenceInput {
		return GeofenceInput{BranchLat: f64p(-6.9), BranchLng: f64p(107.6), Point: p}
	}
	check := func(name string, got GeofenceResult, status string, dist *int) {
		t.Helper()
		if got.Status != status {
			t.Errorf("%s: status %q, want %q", name, got.Status, status)
		}
		eq(t, got.DistanceM, dist)
	}
	check("100 m tanpa akurasi", Geofence(in(metersNorth(100, nil))), GeofenceValid, intp(100))
	check("101 m tanpa akurasi", Geofence(in(metersNorth(101, nil))), GeofenceInvalid, intp(101))
	check("150 m akurasi 50", Geofence(in(metersNorth(150, f64p(50)))), GeofenceValid, intp(150))
	check("151 m akurasi 50", Geofence(in(metersNorth(151, f64p(50)))), GeofenceInvalid, intp(151))
	check("200 m akurasi 150 (batas 100)", Geofence(in(metersNorth(200, f64p(150)))), GeofenceValid, intp(200))
	check("201 m akurasi 150 (batas 100)", Geofence(in(metersNorth(201, f64p(150)))), GeofenceInvalid, intp(201))
	for name, acc := range map[string]float64{"nol": 0, "negatif": -30, "NaN": math.NaN()} {
		check("101 m akurasi "+name, Geofence(in(metersNorth(101, f64p(acc)))), GeofenceInvalid, intp(101))
	}
	for _, r := range []int{0, -5} {
		g := in(metersNorth(101, nil))
		g.RadiusM = intp(r)
		got := Geofence(g)
		eq(t, got.RadiusM, 100)
		check("radius tidak valid", got, GeofenceInvalid, intp(101))
	}
	pt := metersNorth(334, nil)
	check("toko di 0,0", Geofence(GeofenceInput{BranchLat: f64p(0), BranchLng: f64p(0), Point: pt}), GeofenceUnchecked, nil)
	check("toko hanya lat", Geofence(GeofenceInput{BranchLat: f64p(-6.9), Point: pt}), GeofenceUnchecked, nil)
	check("toko hanya lng", Geofence(GeofenceInput{BranchLng: f64p(107.6), Point: pt}), GeofenceUnchecked, nil)
}

func TestDistanceMetersNearAntipodal(t *testing.T) {
	for _, d := range []float64{
		DistanceMeters(0.0000001, 0, -0.0000001, 180),
		DistanceMeters(90, 0, -90, 0),
		DistanceMeters(-33.683475029077115, 105.53185119730568, 33.683475029077115, -74.46814880269432), // h rounds above 1
		DistanceMeters(41.92029254063311, 5.683331059765379, -41.92029254063311, -174.31666894023462),
	} {
		if math.IsNaN(d) || d < 20_000_000 || d > 20_040_000 {
			t.Fatalf("antipodal distance = %v", d)
		}
	}
}

func TestGeofenceNoteMissing(t *testing.T) {
	eq(t, GeofenceNoteMissing(GeofenceInvalid, nil), true)
	eq(t, GeofenceNoteMissing(GeofenceInvalid, sp("  abcd ")), true)
	eq(t, GeofenceNoteMissing(GeofenceInvalid, sp("Ambil stok")), false)
	eq(t, GeofenceNoteMissing(GeofenceValid, nil), false)
	eq(t, GeofenceNoteMissing(GeofenceUnchecked, nil), false)
	eq(t, GeofenceNoteMissing(GeofenceDinas, nil), false)
}
