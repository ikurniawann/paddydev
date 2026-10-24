package hris

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"nuhabit/backend/internal/platform/auth"
	"nuhabit/backend/internal/platform/httpx"
)

// Pencarian alamat/nama tempat untuk peta Lokasi Toko (HRIS → Penempatan
// Karyawan): HR mengetik "Paddy Dago", peta loncat ke hasil yang dipilih.
// Sumbernya OpenStreetMap Nominatim, diproksi lewat server supaya User-Agent,
// batas 1 permintaan/detik, dan cache sesuai kebijakan pemakaiannya.

// Place is one search result.
type Place struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// PlaceSearch finds places by name or address.
type PlaceSearch interface {
	Search(ctx context.Context, q string) ([]Place, error)
}

const (
	// defaultNominatimURL can be overridden by HRIS_GEOCODE_URL.
	defaultNominatimURL = "https://nominatim.openstreetmap.org/search"
	nominatimUserAgent  = "PaddyHRIS/1.0 (Lokasi Toko; +https://paddydev.reddie.id)"
	placeCacheTTL       = 24 * time.Hour
	placeCacheMax       = 500
	maxPlaceBytes       = 256 << 10
)

type placeCacheEntry struct {
	places []Place
	at     time.Time
}

// nominatim is PlaceSearch over the Nominatim HTTP API: one request at a
// time, at least minGap apart, results cached per normalised query.
type nominatim struct {
	base   string
	client *http.Client
	minGap time.Duration

	mu    sync.Mutex
	last  time.Time
	cache map[string]placeCacheEntry
}

func newNominatim(base string, client *http.Client) *nominatim {
	return &nominatim{base: base, client: client, minGap: time.Second, cache: map[string]placeCacheEntry{}}
}

func nominatimFromEnv() *nominatim {
	base := os.Getenv("HRIS_GEOCODE_URL")
	if base == "" {
		base = defaultNominatimURL
	}
	return newNominatim(base, &http.Client{Timeout: 8 * time.Second})
}

func (g *nominatim) Search(ctx context.Context, q string) ([]Place, error) {
	key := strings.ToLower(strings.Join(strings.Fields(q), " "))
	g.mu.Lock()
	defer g.mu.Unlock()
	if e, ok := g.cache[key]; ok && time.Since(e.at) < placeCacheTTL {
		return e.places, nil
	}
	if wait := g.minGap - time.Since(g.last); wait > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	g.last = time.Now()
	places, err := g.fetch(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(g.cache) >= placeCacheMax {
		g.cache = map[string]placeCacheEntry{}
	}
	g.cache[key] = placeCacheEntry{places: places, at: time.Now()}
	return places, nil
}

func (g *nominatim) fetch(ctx context.Context, q string) ([]Place, error) {
	params := url.Values{"q": {q}, "format": {"jsonv2"}, "countrycodes": {"id"}, "limit": {"5"}, "accept-language": {"id"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.base+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", nominatimUserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, placeSearchError("Nominatim membalas HTTP " + strconv.Itoa(res.StatusCode))
	}
	var raw []struct {
		DisplayName string `json:"display_name"`
		Lat         string `json:"lat"`
		Lon         string `json:"lon"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxPlaceBytes)).Decode(&raw); err != nil {
		return nil, err
	}
	places := make([]Place, 0, len(raw))
	for _, r := range raw {
		lat, errLat := strconv.ParseFloat(r.Lat, 64)
		lng, errLng := strconv.ParseFloat(r.Lon, 64)
		if errLat != nil || errLng != nil || (lat == 0 && lng == 0) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
			continue
		}
		places = append(places, Place{Name: r.DisplayName, Lat: lat, Lng: lng})
	}
	return places, nil
}

type placeSearchError string

func (e placeSearchError) Error() string { return string(e) }

// SearchPlaces is the Lokasi Toko search; the upstream failure is logged
// and answered with a hint to the other ways of filling the point.
func (s *Service) SearchPlaces(ctx context.Context, q string) ([]Place, error) {
	q = strings.TrimSpace(q)
	if utf8.RuneCountInString(q) < 3 {
		return nil, httpx.BadRequest("Ketik minimal 3 huruf untuk mencari lokasi")
	}
	if utf8.RuneCountInString(q) > 200 {
		return nil, httpx.BadRequest("Kata kunci lokasi maksimal 200 huruf")
	}
	places, err := s.places.Search(ctx, q)
	if err != nil {
		s.log.Warn("pencarian lokasi gagal", "err", err)
		return nil, httpx.Status(http.StatusServiceUnavailable,
			"Pencarian lokasi sedang tidak tersedia. Klik peta, pakai lokasi Anda, atau tempel koordinat/link Google Maps.")
	}
	return places, nil
}

// GET /api/hris/places?q=
func (h handlers) searchPlaces(w http.ResponseWriter, r *http.Request, _ *auth.User) error {
	places, err := h.svc.SearchPlaces(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		return err
	}
	return ok(w, dataOf(places))
}
