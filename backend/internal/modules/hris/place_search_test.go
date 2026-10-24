package hris

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Klien Nominatim: parameter & User-Agent sesuai kebijakan OSM, hasil diurai,
// koordinat rusak dilewati, hasil yang sama di-cache, HTTP gagal jadi error.
func TestNominatimSearch(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		q := r.URL.Query()
		if q.Get("q") == "gagal" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		if q.Get("q") != "paddy dago" || q.Get("format") != "jsonv2" || q.Get("countrycodes") != "id" ||
			q.Get("limit") != "5" || q.Get("accept-language") != "id" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		if ua := r.Header.Get("User-Agent"); ua == "" || ua == "Go-http-client/1.1" {
			t.Errorf("User-Agent = %q", ua)
		}
		_, _ = w.Write([]byte(`[{"display_name":"Paddy, Jalan Dago, Bandung","lat":"-6.8889","lon":"107.6131"},
			{"display_name":"Rusak","lat":"x","lon":"107"},
			{"display_name":"Nol","lat":"0","lon":"0"}]`))
	}))
	defer srv.Close()

	g := newNominatim(srv.URL, srv.Client())
	g.minGap = 0
	got, err := g.Search(context.Background(), "paddy dago")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Paddy, Jalan Dago, Bandung" || got[0].Lat != -6.8889 || got[0].Lng != 107.6131 {
		t.Fatalf("places = %+v", got)
	}
	if _, err := g.Search(context.Background(), "  Paddy   DAGO "); err != nil || hits.Load() != 1 {
		t.Fatalf("cache: err=%v hits=%d, want 1 hit (query dinormalisasi)", err, hits.Load())
	}
	if _, err := g.Search(context.Background(), "gagal"); err == nil {
		t.Fatal("HTTP 429 harus error")
	}
}

// Jeda minimum antar-permintaan ke Nominatim (maks. 1 per detik).
func TestNominatimThrottle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	g := newNominatim(srv.URL, srv.Client())
	g.minGap = 120 * time.Millisecond
	start := time.Now()
	for _, q := range []string{"satu", "dua", "tiga"} {
		if _, err := g.Search(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(start); d < 240*time.Millisecond {
		t.Fatalf("3 permintaan dalam %v, want ≥ 240ms (jeda 120ms)", d)
	}
}
