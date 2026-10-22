package hris

import (
	"context"
	"errors"
	"testing"
)

type fakePlaces struct{}

func (fakePlaces) Search(_ context.Context, q string) ([]Place, error) {
	if q == "putus" {
		return nil, errors.New("upstream down")
	}
	return []Place{{Name: "Paddy Dago, Bandung", Lat: -6.8889, Lng: 107.6131}}, nil
}

func TestPlaceSearch(t *testing.T) {
	h := newHarness(t)
	hr, _ := h.staff("hrd", []string{"hris.master.placements"}, true)
	staff, _ := h.staff("pos", nil, true)

	expect(t, h.do(&staff, "GET", "/api/hris/places?q=paddy+dago", nil), 403, "")
	expect(t, h.do(&hr, "GET", "/api/hris/places?q=pd", nil), 400, "Ketik minimal 3 huruf untuk mencari lokasi")
	expect(t, h.do(&hr, "GET", "/api/hris/places?q=putus", nil), 503,
		"Pencarian lokasi sedang tidak tersedia. Klik peta, pakai lokasi Anda, atau tempel koordinat/link Google Maps.")

	got := h.do(&hr, "GET", "/api/hris/places?q=paddy+dago", nil)
	expect(t, got, 200, "")
	if l := got.list(); len(l) != 1 {
		t.Fatalf("places = %s", got.raw)
	} else if p := l[0].(map[string]any); p["name"] != "Paddy Dago, Bandung" || p["lat"] != -6.8889 || p["lng"] != 107.6131 {
		t.Fatalf("place = %s", got.raw)
	}
}
