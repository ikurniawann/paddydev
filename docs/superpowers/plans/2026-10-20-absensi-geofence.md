# Paket Absensi: Geofence Cabang & Perjalanan Dinas — Implementation Plan (EPIC-062)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Also load the project skill `paddy-hris-acuan` before starting.

**Goal:** Setiap clock-in/clock-out ESS dinilai di server terhadap radius toko (100 m, override per toko), statusnya (Valid / Invalid / Belum dicek / Perjalanan Dinas) dan jaraknya disimpan, absen di luar area wajib keterangan lewat popup ESS, karyawan bisa mengajukan Perjalanan Dinas yang membebaskan geofence, dan HR bisa mengisi koordinat toko lewat peta serta melihat hasilnya di monitoring dan rekap absensi.

**Architecture:**
- Aturan geofence = fungsi murni Go di `backend/internal/modules/hris/domain/geofence.go` (haversine, toleransi akurasi, status). Server adalah sumber kebenaran; frontend tidak menghitung status.
- `ClockIn`/`ClockOut` (`backend/internal/modules/hris/attendance_clock.go`) memanggil `geofenceFor` (file baru `attendance_geofence.go`) **sebelum** selfie disimpan. Di luar area tanpa keterangan → 422 `geofence_note_required`; ESS membuka popup, lalu mengirim ulang payload yang sama plus `notes`.
- Kolom baru `hris.attendance.clock_in_geofence/clock_in_distance_m/clock_out_geofence/clock_out_distance_m` dan `configuration.branches.geofence_radius_m` (override radius, NULL = 100 m).
- Perjalanan Dinas = tabel baru `hris.business_trips` (rentang tanggal, approval atasan langsung atau HRD) + route `/api/hris/business-trips`. Tanggal yang disetujui membuat status geofence `dinas`, termasuk absen yang sudah terekam di rentang itu (periode belum closing).
- HR mengisi koordinat lewat `PUT /api/hris/branches/{id}/location` (menu Penempatan Karyawan). Komponen peta baru `LocationPickerMap` memakai stack OpenLayers + OSM yang sudah ada (`frontend/src/features/site/components/branch-map.tsx`, `frontend/src/features/member-app/components/geo-map.tsx`) dan dipasang di dialog "Lokasi Toko" HRIS **dan** form profil cabang (Pengaturan → Bisnis).
- Next.js: semua `/api/hris/*` dilayani Go (`frontend/src/lib/backend-routes.ts`); route fallback Next (`frontend/src/lib/hris/attendance-repo.ts`) tidak diubah.

**Tech Stack:** Go (pgx), migrasi PostgreSQL, Next.js 16 (client components, React Query, `next/dynamic` `ssr: false`), OpenLayers 10 (`ol`) + OSM tiles, vitest + Testing Library.

**Spec:** `docs/epics/EPIC-062-absensi-geofence.md` (Goal, Acceptance Criteria, Pertanyaan terbuka) dan acuan `docs/hris/acuan-mom-20261019/` — `status-requirement.md` baris AB-01, AB-02, AB-03; `pertanyaan-rekomendasi.md` K-14 dan urutan eksekusi langkah 6 "Paket Absensi"; `matriks-approval.md` (pola approver atasan langsung, Q-APV-3).

**Prasyarat:**
- Branch kerja `feat/epic062-geofence-absensi` dari `main` (rencana ini ditulis di `docs/epic062-geofence-absensi`).
- **Sebelum eksekusi, user mengonfirmasi rekomendasi di bagian "Pertanyaan terbuka" epic** (P-1 s.d. P-7). Ruling di bawah mengikuti rekomendasi itu; bila user memilih lain, ubah ruling dan test terkait sebelum Task 1.
- Worktree tidak punya `node_modules`. Untuk perintah frontend: `ln -s /home/wit/Desktop/paddydev/frontend/node_modules frontend/node_modules` (symlink tidak di-commit; `node_modules` sudah di `.gitignore`).

## Ruling

- **R1 — Radius.** Default global 100 m = konstanta `domain.GeofenceRadiusDefaultM` (nilai MOM AB-01). Override per toko opsional `configuration.branches.geofence_radius_m` (NULL = 100 m; 25–1000 m), diisi HR. Alasan: toko Paddy banyak di dalam mal/area gudang yang luasnya > 100 m; satu kolom nullable lebih murah daripada setting global yang tidak pernah diubah (P-1). Cost if wrong: radius global perlu UI setting, override bisa diabaikan.
- **R2 — Status.** `valid` (Dalam area), `invalid` (Di luar area), `unchecked` (Belum dicek: toko tanpa koordinat, perangkat tanpa GPS, atau koordinat tidak masuk akal termasuk 0,0 — K-14: tidak pernah Invalid karena koordinat kosong), `dinas` (Perjalanan Dinas disetujui). NULL = absen sebelum EPIC-062, absen yang dicatat HRD untuk karyawan lain, atau absen manual/koreksi HRD. Jarak disimpan dalam meter bulat setiap kali bisa diukur (termasuk saat `dinas`).
- **R3 — Akurasi GPS.** Status memakai jarak yang sudah dibulatkan. Toleransi = `min(akurasi, 100 m)`; akurasi kosong/≤ 0 = 0. **Invalid hanya bila jarak − toleransi > radius** (P-2). Batas 100 m mencegah akurasi palsu/buruk (mis. 5 km dari lokasi IP) meloloskan siapa saja.
- **R4 — Di luar area = boleh absen dengan keterangan.** MOM AB-03 menulis "popup + kolom keterangan", jadi absen tidak diblokir (P-3). Status `invalid` tanpa keterangan ≥ 5 karakter → 422 `{"error": "...", "details": {"code": "geofence_note_required", "distance_m", "radius_m", "branch_name"}}`; tidak ada baris absensi dan selfie belum disimpan. Keterangan disimpan di kolom `notes` yang sudah ada (clock-out menambahkan baris baru, perilaku lama). Status absensi (`present`) dan payroll tidak berubah; HR meninjau lewat monitoring/rekap.
- **R5 — Clock-out.** Dinilai dengan aturan yang sama terhadap toko absensi itu (`attendance.branch_id`) dan tanggal absensi; kolom terpisah `clock_out_*`; aturan keterangan sama (P-5).
- **R6 — Hanya absen oleh karyawan sendiri.** Bila HRD meng-clock-in/out karyawan lain (`employee_id` lain), lokasi berasal dari perangkat HRD, jadi kolom geofence NULL. Absen manual dan koreksi HRD (`attendance_adjust.go`) tidak diubah dan tetap NULL.
- **R7 — Perjalanan Dinas.** Tabel `hris.business_trips`: tanggal mulai ≥ hari ini (WIB), selesai ≥ mulai, maksimal 31 hari per pengajuan, tujuan (3–200 karakter) dan keperluan (5–1000 karakter) wajib, tidak boleh beririsan dengan pengajuan `pending`/`approved` milik karyawan yang sama (exclusion constraint). Approver: atasan langsung (`hris.employees.reporting_to`) atau HRD sebagai cadangan & override; tidak boleh memutus milik sendiri (pola Q-APV-3; P-4). HRD yang mencatat untuk karyawan lain → langsung `approved` ("penugasan"). Batal: pemohon saat `pending`; HRD saat `pending` atau `approved`. Setiap keputusan dan penugasan dicatat di `audit.audit_log` (LA-01).
- **R8 — Efek Perjalanan Dinas.** Clock-in/out pada tanggal dinas yang disetujui → status `dinas` tanpa keterangan, toko tetap dipilih dari penempatan aktif (perilaku lama). Saat disetujui, baris absensi yang sudah ada di rentang itu dengan status `invalid`/`unchecked` diubah menjadi `dinas`, kecuali di periode yang sudah closing. Status absensi `remote` manual HR tetap ada dan tidak diubah otomatis.
- **R9 — Koordinat toko diisi HR di HRIS.** Role HR live tidak punya menu `settings.business` (profil cabang), jadi HRIS → Master Data → Penempatan Karyawan mendapat dialog "Lokasi Toko" (menu `hris.master.placements`) yang memanggil `PUT /api/hris/branches/{id}/location`. Endpoint menulis tiga kolom `configuration.branches` (`lat`, `lng`, `geofence_radius_m`) langsung lewat SQL, seperti HRIS sudah membaca tabel itu, dan mencatat audit `branch_location.update`. Koordinat yang sama dipakai peta situs publik (`/locations`).
- **R10 — Peta pemilih titik.** Komponen baru `LocationPickerMap` (OpenLayers + OSM, stack yang sama dengan `BranchMap`/`GeoMap`): klik peta = titik toko, lingkaran radius digambar geodesik, lat/lng tetap bisa diketik atau ditempel dari Google Maps. Dimuat lewat `next/dynamic` dengan `ssr: false` di dalam client component. Dipasang di dialog Lokasi Toko (HRIS) dan form profil cabang (Pengaturan → Bisnis).
- **R11 — Offline.** ESS adalah web app tanpa antrean offline; absen tanpa internet gagal seperti sekarang dan dialihkan ke pengajuan absen manual (AB-04/AB-06, epic terpisah) (P-6).

## Global Constraints

- Uji integrasi hanya di DB test `postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable`; jangan pakai port 55432, jangan menyentuh DB live atau container docker lain.
- Go: `$HOME/.local/go/bin/go`; `gofmt -l` bersih sebelum commit.
- Migrasi baru di `backend/database/migrations/deltas/` bertimestamp `20261023090000` dan `20261023090100` (EPIC-061 memakai `20261022090000`–`20261022090200` di branch lain dan sudah diterapkan di DB test). Terapkan ke DB test: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`.
- Baca `frontend/AGENTS.md` sebelum mengubah kode Next.js. Dokumentasi Next ada di `/home/wit/Desktop/paddydev/frontend/node_modules/next/dist/docs/` (lazy loading: `01-app/02-guides/lazy-loading.md`; `ssr: false` hanya di Client Component).
- Teks UI dan pesan error dalam Bahasa Indonesia, persis seperti di rencana ini (test mengunci string).
- Setting dan data live (koordinat 52 cabang, radius) adalah keputusan user: rencana ini tidak mengubah data live.
- Commit per task dengan trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Jangan commit ke `main` dan jangan push tanpa izin user.

## Review Focus

1. **Lokasi perangkat 0,0 atau di luar rentang** (GPS rusak/emulator): harus `unchecked`, bukan `invalid` 6.000 km. Diuji di Task 1 (`TestGeofence` kasus "GPS 0,0" dan "GPS di luar rentang").
2. **Akurasi GPS sangat buruk atau dipalsukan** (akurasi 5.000 m): tidak boleh otomatis lolos; toleransi dibatasi 100 m. Diuji di Task 1 (kasus "334 m akurasi 5000").
3. **422 tidak boleh meninggalkan jejak**: tanpa baris absensi dan tanpa file selfie, supaya kirim ulang dengan keterangan tidak kena "Already clocked in today". Diuji di Task 2 (`TestClockInOutsideAreaNeedsNote`, cek jumlah baris dan isi direktori upload).
4. **HRD meng-clock-in karyawan lain dari kantor** tidak boleh menghasilkan Invalid/422 untuk karyawan itu. Diuji di Task 2 (`TestClockGeofenceUnchecked`, kasus HR).
5. **Perjalanan Dinas disetujui setelah karyawan sudah absen di luar area pagi itu**: baris hari itu ikut menjadi `dinas`. Diuji di Task 4 (`TestBusinessTripFlow`, relabel).

---

### Task 1: Aturan geofence (domain murni)

**Files:**
- Create: `backend/internal/modules/hris/domain/geofence.go`
- Test: `backend/internal/modules/hris/domain/geofence_test.go`

**Interfaces:**
- Consumes: `JSTrim` (sudah ada di `domain/calendar.go`).
- Produces:
  - `const GeofenceRadiusDefaultM = 100`, `const GeofenceAccuracyCapM = 100.0`, `const GeofenceNoteMinChars = 5`
  - `const GeofenceValid = "valid"`, `GeofenceInvalid = "invalid"`, `GeofenceUnchecked = "unchecked"`, `GeofenceDinas = "dinas"`
  - `type GeoPoint struct { Lat, Lng float64; Accuracy *float64 }`
  - `type GeofenceInput struct { BranchLat, BranchLng *float64; RadiusM *int; Point *GeoPoint; OnBusinessTrip bool }`
  - `type GeofenceResult struct { Status string; DistanceM *int; RadiusM int }`
  - `func DistanceMeters(lat1, lng1, lat2, lng2 float64) float64`
  - `func Geofence(in GeofenceInput) GeofenceResult`
  - `func GeofenceNoteMissing(status string, note *string) bool`
  - `func usableCoord(lat, lng float64) bool` (dipakai Task 3)

- [ ] **Step 1: Tulis test yang gagal**

`backend/internal/modules/hris/domain/geofence_test.go`:

```go
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
```

(`eq` dan `sp` sudah ada di `domain_test.go`.)

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain -run 'TestDistanceMeters|TestGeofence' -v`
Expected: FAIL (build error `undefined: DistanceMeters`).

- [ ] **Step 3: Implementasi**

`backend/internal/modules/hris/domain/geofence.go`:

```go
package domain

import "math"

// Geofence absensi (EPIC-062, MOM AB-01/AB-03, K-14): the distance from a
// clock-in/out position to the chosen store, judged against the store
// radius (100 m or a per-store override) with a capped GPS accuracy
// tolerance.

const (
	// GeofenceRadiusDefaultM is the MOM radius when the store has no override.
	GeofenceRadiusDefaultM = 100
	// GeofenceAccuracyCapM caps the GPS accuracy tolerance (EPIC-062 R3).
	GeofenceAccuracyCapM = 100.0
	// GeofenceNoteMinChars is the shortest out-of-area note (AB-03).
	GeofenceNoteMinChars = 5
)

// Statuses stored in hris.attendance.clock_in_geofence / clock_out_geofence.
const (
	GeofenceValid     = "valid"
	GeofenceInvalid   = "invalid"
	GeofenceUnchecked = "unchecked"
	GeofenceDinas     = "dinas"
)

// GeoPoint is a device position; Accuracy is the reported radius in meters.
type GeoPoint struct {
	Lat, Lng float64
	Accuracy *float64
}

// GeofenceInput is one clock event: the store coordinates and radius
// override, the device position, and whether an approved business trip
// covers the date.
type GeofenceInput struct {
	BranchLat, BranchLng *float64
	RadiusM              *int
	Point                *GeoPoint
	OnBusinessTrip       bool
}

// GeofenceResult is the verdict; DistanceM is nil when it cannot be measured.
type GeofenceResult struct {
	Status    string
	DistanceM *int
	RadiusM   int
}

// DistanceMeters is the haversine distance in meters (the same formula as
// frontend/src/lib/hris/placements.ts distanceMeters).
func DistanceMeters(lat1, lng1, lat2, lng2 float64) float64 {
	rad := func(v float64) float64 { return v * math.Pi / 180 }
	dLat, dLng := rad(lat2-lat1), rad(lng2-lng1)
	h := math.Pow(math.Sin(dLat/2), 2) + math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Pow(math.Sin(dLng/2), 2)
	return 2 * 6_371_000 * math.Asin(math.Sqrt(h))
}

// usableCoord rejects NaN, out-of-range and 0,0 positions (a GPS fix that
// failed, not a place in Indonesia).
func usableCoord(lat, lng float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lng) || math.Abs(lat) > 90 || math.Abs(lng) > 180 {
		return false
	}
	return lat != 0 || lng != 0
}

func accuracyTolerance(acc *float64) float64 {
	if acc == nil || math.IsNaN(*acc) || *acc <= 0 {
		return 0
	}
	return math.Min(*acc, GeofenceAccuracyCapM)
}

// Geofence judges one clock event (EPIC-062 R2/R3): dinas on an approved
// business trip; unchecked without usable store or device coordinates;
// invalid only when the rounded distance minus the capped accuracy exceeds
// the radius; valid otherwise.
func Geofence(in GeofenceInput) GeofenceResult {
	radius := GeofenceRadiusDefaultM
	if in.RadiusM != nil && *in.RadiusM > 0 {
		radius = *in.RadiusM
	}
	out := GeofenceResult{Status: GeofenceUnchecked, RadiusM: radius}
	if in.BranchLat != nil && in.BranchLng != nil && usableCoord(*in.BranchLat, *in.BranchLng) &&
		in.Point != nil && usableCoord(in.Point.Lat, in.Point.Lng) {
		d := int(math.Round(DistanceMeters(*in.BranchLat, *in.BranchLng, in.Point.Lat, in.Point.Lng)))
		out.DistanceM = &d
	}
	switch {
	case in.OnBusinessTrip:
		out.Status = GeofenceDinas
	case out.DistanceM == nil:
		out.Status = GeofenceUnchecked
	case float64(*out.DistanceM)-accuracyTolerance(in.Point.Accuracy) > float64(radius):
		out.Status = GeofenceInvalid
	default:
		out.Status = GeofenceValid
	}
	return out
}

// GeofenceNoteMissing: an out-of-area event needs a note of at least
// GeofenceNoteMinChars characters (AB-03); other statuses need none.
func GeofenceNoteMissing(status string, note *string) bool {
	if status != GeofenceInvalid {
		return false
	}
	return note == nil || len([]rune(JSTrim(*note))) < GeofenceNoteMinChars
}
```

- [ ] **Step 4: Jalankan test, pastikan lulus**

Run: `cd backend && $HOME/.local/go/bin/go test ./internal/modules/hris/domain -count=1`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
gofmt -l backend/internal/modules/hris/domain
git add backend/internal/modules/hris/domain/geofence.go backend/internal/modules/hris/domain/geofence_test.go
git commit -m "feat(hris): aturan geofence absensi radius 100 m, toleransi akurasi GPS, status valid/invalid/belum dicek/dinas, EPIC-062"
```

---

### Task 2: Clock-in/out menyimpan status geofence dan wajib keterangan di luar area

**Files:**
- Create: `backend/database/migrations/deltas/20261023090000_attendance_geofence.sql`
- Create: `backend/internal/modules/hris/attendance_geofence.go`
- Modify: `backend/internal/modules/hris/attendance_clock.go` (`ClockIn`, `ClockOut`)
- Modify: `backend/internal/modules/hris/placements.go` (`PlacementRepo.BranchGeofence` + implementasi store)
- Test: `backend/internal/modules/hris/attendance_geofence_integration_test.go` (baru)

**Interfaces:**
- Consumes: `domain.Geofence`, `domain.GeofenceNoteMissing`, `domain.GeofenceNoteMinChars` (Task 1); `s.clockBranch` (lama); `attendanceDate` (lama, `attendance_adjust.go`).
- Produces:
  - Kolom `hris.attendance.clock_in_geofence text`, `clock_in_distance_m integer`, `clock_out_geofence text`, `clock_out_distance_m integer`; kolom `configuration.branches.geofence_radius_m integer`.
  - `PlacementRepo.BranchGeofence(ctx context.Context, id string) (*Row, error)` → kolom `name, lat, lng, geofence_radius_m`; nil bila tidak ada.
  - `type clockGeofence struct { domain.GeofenceResult; BranchName string }`
  - `func (s *Service) geofenceFor(ctx context.Context, a *Actor, employeeID, date string, branchID *string, loc *Row) (*clockGeofence, error)` (Task 4 menambah cek Perjalanan Dinas di sini).
  - `func requireGeofenceNote(g *clockGeofence, note *string) error`
  - `func numPtr(r *Row, key string) *float64`
  - Respons 422: `{"success":false,"error":"Anda di luar area <toko> (±<jarak> m, batas <radius> m). Isi keterangan minimal 5 karakter untuk tetap absen.","details":{"code":"geofence_note_required","distance_m":<int>,"radius_m":<int>,"branch_name":<string|null>}}`

- [ ] **Step 1: Tulis migrasi**

`backend/database/migrations/deltas/20261023090000_attendance_geofence.sql`:

```sql
-- =============================================================================
-- Geofence absensi (EPIC-062, MOM AB-01/AB-03, K-14).
--   hris.attendance.clock_in_geofence / clock_out_geofence:
--     valid | invalid | unchecked (Belum dicek) | dinas (Perjalanan Dinas).
--     NULL = absen sebelum EPIC-062, dicatat HRD untuk karyawan lain, atau
--     absen manual/koreksi HRD.
--   *_distance_m: jarak ke toko absensi (meter, dibulatkan), NULL bila tidak
--     bisa diukur.
--   configuration.branches.geofence_radius_m: override radius per toko
--     (NULL = 100 m, nilai MOM).
-- =============================================================================

ALTER TABLE hris.attendance
  ADD COLUMN IF NOT EXISTS clock_in_geofence    text,
  ADD COLUMN IF NOT EXISTS clock_in_distance_m  integer,
  ADD COLUMN IF NOT EXISTS clock_out_geofence   text,
  ADD COLUMN IF NOT EXISTS clock_out_distance_m integer;

DO $$ BEGIN
  ALTER TABLE hris.attendance ADD CONSTRAINT attendance_clock_in_geofence_check
    CHECK (clock_in_geofence IN ('valid', 'invalid', 'unchecked', 'dinas'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE hris.attendance ADD CONSTRAINT attendance_clock_out_geofence_check
    CHECK (clock_out_geofence IN ('valid', 'invalid', 'unchecked', 'dinas'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE hris.attendance ADD CONSTRAINT attendance_geofence_distance_check
    CHECK ((clock_in_distance_m IS NULL OR clock_in_distance_m >= 0)
       AND (clock_out_distance_m IS NULL OR clock_out_distance_m >= 0));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS idx_attendance_geofence_invalid ON hris.attendance (date)
  WHERE clock_in_geofence = 'invalid' OR clock_out_geofence = 'invalid';

ALTER TABLE configuration.branches
  ADD COLUMN IF NOT EXISTS geofence_radius_m integer;

DO $$ BEGIN
  ALTER TABLE configuration.branches ADD CONSTRAINT branches_geofence_radius_check
    CHECK (geofence_radius_m IS NULL OR geofence_radius_m BETWEEN 25 AND 1000);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

COMMENT ON COLUMN configuration.branches.geofence_radius_m IS
  'Radius absensi toko dalam meter (EPIC-062); NULL = 100 m sesuai MOM AB-01.';
```

Run: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`
Expected: `20261023090000_attendance_geofence.sql` diterapkan, tanpa error.

- [ ] **Step 2: Tulis test yang gagal**

`backend/internal/modules/hris/attendance_geofence_integration_test.go`:

```go
package hris

import (
	"os"
	"path/filepath"
	"testing"
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
}
```

- [ ] **Step 3: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run 'TestClockInOutsideAreaNeedsNote|TestClockOutOutsideAreaNeedsNote|TestClockGeofenceUnchecked' -count=1 -v`
Expected: FAIL — `status 200, want 422` pada clock-in di luar area tanpa keterangan.

- [ ] **Step 4: Implementasi**

`backend/internal/modules/hris/placements.go` — tambahkan ke `PlacementRepo`:

```go
	// BranchGeofence is the store's name, coordinates and radius override
	// (EPIC-062); nil when the store does not exist.
	BranchGeofence(ctx context.Context, id string) (*Row, error)
```

dan implementasinya di bawah `BranchExists`:

```go
func (s *store) BranchGeofence(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT name, lat, lng, geofence_radius_m FROM configuration.branches WHERE id::text = $1`, id)
	return row, ignoreBadInput(err)
}
```

`backend/internal/modules/hris/attendance_geofence.go`:

```go
package hris

import (
	"context"
	"fmt"
	"math"
	"net/http"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/httpx"
)

// Geofence absensi (EPIC-062): the status and distance of every clock-in
// and clock-out against the chosen store. Only an employee clocking
// themselves is judged; HR recording for someone else leaves the columns
// NULL (R6).

// clockGeofence is the verdict plus the store name for the 422 message.
type clockGeofence struct {
	domain.GeofenceResult
	BranchName string
}

// numPtr is a nullable numeric column (numeric arrives as text); nil for
// NULL or an unparsable value.
func numPtr(r *Row, key string) *float64 {
	if r == nil || r.Get(key) == nil {
		return nil
	}
	v := r.Num(key)
	if math.IsNaN(v) {
		return nil
	}
	return &v
}

// clockPoint is the device position of a clock location object; nil when
// the device sent none.
func clockPoint(loc *Row) *domain.GeoPoint {
	if loc == nil || !loc.Has("latitude") || !loc.Has("longitude") {
		return nil
	}
	return &domain.GeoPoint{Lat: loc.Num("latitude"), Lng: loc.Num("longitude"), Accuracy: numPtr(loc, "accuracy")}
}

// geofenceFor judges one clock event of employeeID on date at branchID;
// nil when the actor records for another employee.
func (s *Service) geofenceFor(ctx context.Context, a *Actor, employeeID, date string, branchID *string, loc *Row) (*clockGeofence, error) {
	if a.EmployeeID == nil || *a.EmployeeID != employeeID {
		return nil, nil
	}
	in := domain.GeofenceInput{Point: clockPoint(loc)}
	name := ""
	if branchID != nil && *branchID != "" {
		b, err := s.repo.BranchGeofence(ctx, *branchID)
		if err != nil {
			return nil, err
		}
		if b != nil {
			in.BranchLat, in.BranchLng = numPtr(b, "lat"), numPtr(b, "lng")
			if r := numPtr(b, "geofence_radius_m"); r != nil {
				v := int(*r)
				in.RadiusM = &v
			}
			name = b.Str("name")
		}
	}
	return &clockGeofence{GeofenceResult: domain.Geofence(in), BranchName: name}, nil
}

// requireGeofenceNote is the AB-03 popup contract: an out-of-area event
// without a note answers 422 geofence_note_required before anything is
// stored, so the client resends the same payload with notes.
func requireGeofenceNote(g *clockGeofence, note *string) error {
	if g == nil || !domain.GeofenceNoteMissing(g.Status, note) {
		return nil
	}
	place := g.BranchName
	var branchName any
	if place == "" {
		place = "toko"
	} else {
		branchName = place
	}
	distance := *g.DistanceM // invalid always has a distance
	return &httpx.Error{
		Status: http.StatusUnprocessableEntity,
		Message: fmt.Sprintf("Anda di luar area %s (±%d m, batas %d m). Isi keterangan minimal %d karakter untuk tetap absen.",
			place, distance, g.RadiusM, domain.GeofenceNoteMinChars),
		Details: obj("code", "geofence_note_required", "distance_m", distance, "radius_m", g.RadiusM, "branch_name", branchName),
	}
}

// status and distance are the column values; NULL when not judged.
func (g *clockGeofence) status() any {
	if g == nil {
		return nil
	}
	return g.Status
}

func (g *clockGeofence) distance() any {
	if g == nil || g.DistanceM == nil {
		return nil
	}
	return *g.DistanceM
}
```

`backend/internal/modules/hris/attendance_clock.go`, di `ClockIn` ganti blok dari `branch, err := s.clockBranch(...)` sampai `photo, err := s.saveSelfie(...)` menjadi:

```go
	branch, err := s.clockBranch(ctx, a, *employeeID, date, in.BranchID)
	if err != nil {
		return nil, nil, err
	}
	geo, err := s.geofenceFor(ctx, a, *employeeID, date, branch, in.Location)
	if err != nil {
		return nil, nil, err
	}
	if err := requireGeofenceNote(geo, in.Notes); err != nil {
		return nil, nil, err
	}
	photo, err := s.saveSelfie(in.Photo, *employeeID)
```

dan setelah `f.add("branch_id", branch)`:

```go
	f.add("clock_in_geofence", geo.status())
	f.add("clock_in_distance_m", geo.distance())
```

Di `ClockOut`, setelah blok `ensureAttendanceOpen` dan sebelum `saveSelfie`:

```go
	geo, err := s.geofenceFor(ctx, a, employeeID, attendanceDate(att), att.StrPtr("branch_id"), in.Location)
	if err != nil {
		return nil, err
	}
	if err := requireGeofenceNote(geo, in.Notes); err != nil {
		return nil, err
	}
```

dan setelah `f.add("notes", notes)`:

```go
	f.add("clock_out_geofence", geo.status())
	f.add("clock_out_distance_m", geo.distance())
```

Perbarui komentar paket di atas `ClockIn`: tambahkan kalimat "Both directions are judged against the store geofence (EPIC-062); out of area needs a note."

- [ ] **Step 5: Jalankan test, pastikan lulus, lalu paket hris**

Run: perintah Step 3. Expected: PASS.

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -count=1`
Expected: `ok`, atau hanya kegagalan yang sama dengan baseline `main` (jalankan perintah yang sama di `main` lebih dulu dan simpan daftar `--- FAIL`). `TestClockInStore` (penempatan) tetap lulus karena tidak mengirim lokasi (status `unchecked`).

- [ ] **Step 6: Commit**

```bash
gofmt -l backend/internal/modules/hris
git add backend/database/migrations/deltas/20261023090000_attendance_geofence.sql backend/internal/modules/hris/attendance_geofence.go backend/internal/modules/hris/attendance_clock.go backend/internal/modules/hris/placements.go backend/internal/modules/hris/attendance_geofence_integration_test.go
git commit -m "feat(hris): clock-in/out menyimpan status & jarak geofence, di luar area wajib keterangan (422), EPIC-062"
```

---

### Task 3: API lokasi toko untuk HR (koordinat + radius)

**Files:**
- Modify: `backend/internal/modules/hris/domain/geofence.go` (`ValidateBranchLocation`)
- Modify: `backend/internal/modules/hris/domain/geofence_test.go`
- Modify: `backend/internal/modules/hris/placements.go` (`PlacementRepo.SaveBranchLocation`, `ActiveBranches` + `ActivePlacements` memuat `geofence_radius_m`, service `SaveBranchLocation`, handler `putBranchLocation`)
- Modify: `backend/internal/modules/hris/routes.go`
- Test: `backend/internal/modules/hris/branch_location_integration_test.go` (baru)

**Interfaces:**
- Consumes: `usableCoord` (Task 1); `PlacementRepo.BranchGeofence` (Task 2); `audit.Write`, `ptr` (lama, `attendance_periods.go`).
- Produces:
  - `func ValidateBranchLocation(lat, lng *float64, radius *int) string` (domain)
  - `PlacementRepo.SaveBranchLocation(ctx context.Context, id string, lat, lng *float64, radius *int) (*Row, error)` → `id, code, name, lat, lng, geofence_radius_m`
  - `func (s *Service) SaveBranchLocation(ctx context.Context, u *auth.User, id string, lat, lng *float64, radius *int, meta audit.Entry) (*Row, error)`
  - Route `PUT /api/hris/branches/{id}/location` body `{ lat: number|null, lng: number|null, geofence_radius_m: number|null }` → `{ data: { id, code, name, lat, lng, geofence_radius_m } }`
  - `GET /api/hris/branches` dan `GET /api/hris/me/placements` kini memuat `geofence_radius_m`.

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan ke `domain/geofence_test.go`:

```go
func TestValidateBranchLocation(t *testing.T) {
	eq(t, ValidateBranchLocation(nil, nil, nil), "")
	eq(t, ValidateBranchLocation(f64p(-6.8889), f64p(107.596), intp(250)), "")
	eq(t, ValidateBranchLocation(f64p(-6.8889), nil, nil), "Latitude dan longitude wajib diisi berdua atau dikosongkan berdua")
	eq(t, ValidateBranchLocation(f64p(0), f64p(0), nil), "Koordinat tidak valid (latitude -90..90, longitude -180..180, bukan 0,0)")
	eq(t, ValidateBranchLocation(f64p(-91), f64p(107.6), nil), "Koordinat tidak valid (latitude -90..90, longitude -180..180, bukan 0,0)")
	eq(t, ValidateBranchLocation(f64p(-6.9), f64p(107.6), intp(10)), "Radius absensi 25–1000 meter")
	eq(t, ValidateBranchLocation(nil, nil, intp(1001)), "Radius absensi 25–1000 meter")
}
```

`backend/internal/modules/hris/branch_location_integration_test.go`:

```go
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
```

- [ ] **Step 2: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -run 'TestValidateBranchLocation|TestBranchLocation' -count=1 -v`
Expected: FAIL (build error `undefined: ValidateBranchLocation`).

- [ ] **Step 3: Implementasi**

`domain/geofence.go`, tambahkan:

```go
// ValidateBranchLocation checks the HR store location form (EPIC-062 R9):
// both coordinates or neither, a usable position, and a radius override of
// 25–1000 m; "" when valid.
func ValidateBranchLocation(lat, lng *float64, radius *int) string {
	if (lat == nil) != (lng == nil) {
		return "Latitude dan longitude wajib diisi berdua atau dikosongkan berdua"
	}
	if lat != nil && !usableCoord(*lat, *lng) {
		return "Koordinat tidak valid (latitude -90..90, longitude -180..180, bukan 0,0)"
	}
	if radius != nil && (*radius < 25 || *radius > 1000) {
		return "Radius absensi 25–1000 meter"
	}
	return ""
}
```

`placements.go`:
- Di `ActivePlacements`, ubah kolom `ep.is_primary, b.lat, b.lng` menjadi `ep.is_primary, b.lat, b.lng, b.geofence_radius_m`.
- Di `ActiveBranches`, ubah `b.lat, b.lng` menjadi `b.lat, b.lng, b.geofence_radius_m`.
- Tambahkan ke `PlacementRepo`:

```go
	// SaveBranchLocation writes the store coordinates and radius override
	// (EPIC-062 R9), returning the saved location; nil when not found.
	SaveBranchLocation(ctx context.Context, id string, lat, lng *float64, radius *int) (*Row, error)
```

- Implementasi store, service, dan handler (letakkan service + handler setelah `listBranches`; tambahkan import `"nuhabit/backend/internal/platform/audit"`):

```go
func (s *store) SaveBranchLocation(ctx context.Context, id string, lat, lng *float64, radius *int) (*Row, error) {
	return queryRow(ctx, s.db, `UPDATE configuration.branches
		SET lat = $2, lng = $3, geofence_radius_m = $4, updated_at = now()
		WHERE id = $1 RETURNING id, code, name, lat, lng, geofence_radius_m`, id, lat, lng, radius)
}

// SaveBranchLocation is the HR store location edit (EPIC-062 R9): HRIS
// writes the three geofence columns of configuration.branches (lat/lng are
// shared with the public site map) and records the change in the audit log.
func (s *Service) SaveBranchLocation(ctx context.Context, u *auth.User, id string, lat, lng *float64, radius *int, meta audit.Entry) (*Row, error) {
	if msg := domain.ValidateBranchLocation(lat, lng, radius); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	var row *Row
	err := s.repo.InTx(ctx, func(r Repository) error {
		before, err := r.BranchGeofence(ctx, id)
		if err != nil {
			return err
		}
		if before == nil {
			return httpx.NotFound("Toko tidak ditemukan")
		}
		if row, err = r.SaveBranchLocation(ctx, id, lat, lng, radius); err != nil {
			return err
		}
		meta.ActorID, meta.ActorName = u.ID, ptr(u.FullName)
		meta.Action, meta.Entity, meta.EntityID = "branch_location.update", "configuration.branches", ptr(id)
		meta.EntityLabel = ptr(before.Str("name"))
		meta.Before = map[string]any{"lat": before.Get("lat"), "lng": before.Get("lng"), "geofence_radius_m": before.Get("geofence_radius_m")}
		meta.After = map[string]any{"lat": row.Get("lat"), "lng": row.Get("lng"), "geofence_radius_m": row.Get("geofence_radius_m")}
		return audit.Write(ctx, r.querier(), meta)
	})
	return row, err
}

// PUT /api/hris/branches/{id}/location { lat, lng, geofence_radius_m }
func (h handlers) putBranchLocation(w http.ResponseWriter, r *http.Request, u *auth.User) error {
	id, err := requireUUID(r.PathValue("id"), "ID toko tidak valid")
	if err != nil {
		return err
	}
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	lat := f.Num("lat", nullish, validate.NumOpts{})
	lng := f.Num("lng", nullish, validate.NumOpts{})
	radius := f.Int("geofence_radius_m", nullish, validate.NumOpts{})
	if err := formErr(f, message); err != nil {
		return err
	}
	row, err := h.svc.SaveBranchLocation(r.Context(), u, id, lat, lng, radius, audit.Entry{}.WithRequest(r))
	if err != nil {
		return err
	}
	return ok(w, dataOf(row))
}
```

`routes.go`, di blok placements setelah `GET /api/hris/branches`:

```go
		r("PUT /api/hris/branches/{id}/location", h.withMenu(placementManagers, h.putBranchLocation)),
```

Bila `requireUUID` mengembalikan pesan lain untuk `bukan-uuid`, cek `transport.go:169`; pesan yang diteruskan adalah argumen kedua, jadi `ID toko tidak valid` benar.

- [ ] **Step 4: Jalankan test, pastikan lulus, lalu paket hris**

Run: perintah Step 2. Expected: PASS.
Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -count=1`
Expected: sama dengan baseline (`TestPlacements` tetap lulus; kolom tambahan tidak dicek urutannya).

- [ ] **Step 5: Commit**

```bash
gofmt -l backend/internal/modules/hris
git add backend/internal/modules/hris/domain/geofence.go backend/internal/modules/hris/domain/geofence_test.go backend/internal/modules/hris/placements.go backend/internal/modules/hris/routes.go backend/internal/modules/hris/branch_location_integration_test.go
git commit -m "feat(hris): API lokasi toko (koordinat + radius absensi) untuk HR dengan audit, EPIC-062"
```

---

### Task 4: Perjalanan Dinas (backend) dan pengecualian geofence

**Files:**
- Create: `backend/database/migrations/deltas/20261023090100_business_trips.sql`
- Create: `backend/internal/modules/hris/domain/business_trip.go`
- Create: `backend/internal/modules/hris/domain/business_trip_test.go`
- Create: `backend/internal/modules/hris/business_trips.go`
- Modify: `backend/internal/modules/hris/service.go` (`Repository` memuat `TripRepo`)
- Modify: `backend/internal/modules/hris/routes.go`
- Modify: `backend/internal/modules/hris/attendance_geofence.go` (`geofenceFor` cek Perjalanan Dinas)
- Test: `backend/internal/modules/hris/business_trips_integration_test.go` (baru)

**Interfaces:**
- Consumes: `geofenceFor` (Task 2), `s.repo.SubordinateIDs`, `s.repo.OvertimeTarget` (lama, `overtime.go`), `actorName`, `ptr`, `regexOf`, `minLen`, `errNoRow`, `database.PgCode`.
- Produces:
  - Tabel `hris.business_trips` (kolom: `id, employee_id, start_date, end_date, destination, purpose, status, requested_by, decided_by, decided_by_name, decided_at, rejection_reason, created_at, updated_at`); menu ESS `ess.business-trip` (`/dashboard/me/perjalanan-dinas`).
  - Domain: `const BusinessTripMaxDays = 31`, `func ValidateBusinessTripDates(start, end, today string) string`, `type TripActor struct { EmployeeID *string; IsHR bool }`, `type TripRef struct { EmployeeID string; ReportingTo *string; Status string }`, `func CanDecideBusinessTrip(a TripActor, t TripRef, action string) (bool, string)`.
  - `TripRepo` interface (lihat kode), `ApprovedTripOn(ctx, employeeID, date string) (bool, error)`.
  - Routes:
    - `GET /api/hris/business-trips?scope=approvals&status=&employee_id=` → `{ data: BusinessTrip[] }`
    - `POST /api/hris/business-trips` `{ employee_id?, start_date, end_date, destination, purpose }` → `{ data, message }`
    - `POST /api/hris/business-trips/{id}/decide` `{ action: approve|reject|cancel, rejection_reason? }` → `{ data, message }`
  - Bentuk `BusinessTrip`: kolom tabel (tanggal sebagai `YYYY-MM-DD`) + `employee: { id, full_name, nip, reporting_to }`.

- [ ] **Step 1: Tulis migrasi**

`backend/database/migrations/deltas/20261023090100_business_trips.sql`:

```sql
-- =============================================================================
-- Perjalanan Dinas (EPIC-062, MOM AB-02): pengajuan karyawan dengan rentang
-- tanggal, diputus atasan langsung (reporting_to) atau HRD. Tanggal yang
-- disetujui membuat status geofence absensi 'dinas'.
-- Aturan dijaga constraint: selesai >= mulai, maksimal 31 hari, dan tidak ada
-- dua pengajuan pending/approved yang beririsan untuk karyawan yang sama.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS hris.business_trips (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id      uuid NOT NULL REFERENCES hris.employees(id) ON DELETE CASCADE,
  start_date       date NOT NULL,
  end_date         date NOT NULL,
  destination      text NOT NULL,
  purpose          text NOT NULL,
  status           text NOT NULL DEFAULT 'pending',
  requested_by     uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  decided_by       uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  decided_by_name  text,
  decided_at       timestamptz,
  rejection_reason text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT business_trips_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
  CONSTRAINT business_trips_dates_check CHECK (end_date >= start_date AND end_date - start_date <= 30),
  CONSTRAINT business_trips_text_check CHECK (length(btrim(destination)) > 0 AND length(btrim(purpose)) > 0),
  CONSTRAINT business_trips_no_overlap EXCLUDE USING gist (
    employee_id WITH =,
    daterange(start_date, end_date, '[]') WITH &&
  ) WHERE (status IN ('pending', 'approved'))
);

CREATE INDEX IF NOT EXISTS idx_business_trips_employee ON hris.business_trips (employee_id, start_date DESC);
CREATE INDEX IF NOT EXISTS idx_business_trips_status ON hris.business_trips (status, start_date);

DROP TRIGGER IF EXISTS business_trips_updated_at ON hris.business_trips;
CREATE TRIGGER business_trips_updated_at BEFORE UPDATE ON hris.business_trips
  FOR EACH ROW EXECUTE FUNCTION public.update_hris_updated_at();

COMMENT ON TABLE hris.business_trips IS
  'Perjalanan Dinas (EPIC-062): rentang tanggal yang disetujui dikecualikan dari geofence absensi.';

-- ── Menu ESS → Perjalanan Dinas ──────────────────────────────────────────────
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context)
VALUES ('ess.business-trip', 'Perjalanan Dinas', '/dashboard/me/perjalanan-dinas', 'briefcase', 'sidebar', 25,
        '{"actions":["read"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path,
  icon = EXCLUDED.icon, menu_type = EXCLUDED.menu_type,
  order_number = EXCLUDED.order_number, is_active = true, is_visible = true,
  deleted_at = NULL, updated_at = now();

UPDATE iam.menus SET module = 'ess', level = 2 WHERE code = 'ess.business-trip';
UPDATE iam.menus child SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code = 'ess.business-trip' AND parent.code = 'ess';

-- Hak akses mengikuti menu ESS Lembur: role yang sama, aksi read.
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT rmp.role_id, m.id, '["read"]'::jsonb
FROM iam.role_menu_permissions rmp
JOIN iam.menus src ON src.id = rmp.menu_id AND src.code = 'ess.overtime'
CROSS JOIN iam.menus m
WHERE m.code = 'ess.business-trip' AND rmp.is_active
ON CONFLICT (role_id, menu_id) DO UPDATE SET
  is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();
```

Run: `cd backend && MIGRATE_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go run ./cmd/migrate -apply`
Expected: `20261023090100_business_trips.sql` diterapkan.
Run: `psql 'postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' -Atc "SELECT count(*) FROM iam.role_menu_permissions rmp JOIN iam.menus m ON m.id = rmp.menu_id WHERE m.code = 'ess.business-trip'"`
Expected: sama dengan jumlah role pemegang `ess.overtime` (10 di DB test per 20-10-2026).

- [ ] **Step 2: Tulis test domain yang gagal**

`backend/internal/modules/hris/domain/business_trip_test.go`:

```go
package domain

import "testing"

func TestValidateBusinessTripDates(t *testing.T) {
	today := "2026-10-05"
	eq(t, ValidateBusinessTripDates("2026-10-05", "2026-10-07", today), "")
	eq(t, ValidateBusinessTripDates("2026-10-05", "2026-11-04", today), "") // 31 hari
	eq(t, ValidateBusinessTripDates("2026-10-05", "2026-11-05", today), "Perjalanan Dinas maksimal 31 hari per pengajuan")
	eq(t, ValidateBusinessTripDates("2026-10-04", "2026-10-05", today), "Tanggal mulai tidak boleh sebelum hari ini")
	eq(t, ValidateBusinessTripDates("2026-10-07", "2026-10-06", today), "Tanggal selesai tidak boleh sebelum tanggal mulai")
	eq(t, ValidateBusinessTripDates("2026-02-31", "2026-03-01", today), "Format tanggal tidak valid")
}

func TestCanDecideBusinessTrip(t *testing.T) {
	emp, mgr, hr, other := "emp-1", "mgr-1", "hr-1", "emp-2"
	ref := func(status string) TripRef { return TripRef{EmployeeID: emp, ReportingTo: &mgr, Status: status} }
	ok := func(a TripActor, r TripRef, action string) bool {
		v, _ := CanDecideBusinessTrip(a, r, action)
		return v
	}
	hrA, mgrA, selfA, otherA := TripActor{&hr, true}, TripActor{&mgr, false}, TripActor{&emp, false}, TripActor{&other, false}
	eq(t, ok(mgrA, ref("pending"), "approve"), true)
	eq(t, ok(hrA, ref("pending"), "reject"), true)
	eq(t, ok(TripActor{nil, true}, ref("pending"), "approve"), true) // HRD tanpa data karyawan
	eq(t, ok(otherA, ref("pending"), "approve"), false)
	allowed, reason := CanDecideBusinessTrip(TripActor{&emp, true}, ref("pending"), "approve")
	eq(t, allowed, false)
	eq(t, reason, "Tidak bisa memutuskan Perjalanan Dinas sendiri")
	eq(t, ok(selfA, ref("pending"), "cancel"), true)
	eq(t, ok(selfA, ref("approved"), "cancel"), false)
	eq(t, ok(hrA, ref("approved"), "cancel"), true)
	eq(t, ok(mgrA, ref("pending"), "cancel"), false)
	eq(t, ok(hrA, ref("rejected"), "cancel"), false)
	eq(t, ok(hrA, ref("pending"), "delete"), false)
}
```

- [ ] **Step 3: Tulis test integrasi yang gagal**

`backend/internal/modules/hris/business_trips_integration_test.go`:

```go
package hris

import (
	"testing"

	"nuhabit/backend/internal/platform/testutil"
)

func TestBusinessTripFlow(t *testing.T) {
	h, _ := newFileHarness(t)
	hr, _ := h.staff("hrd", nil, true)
	boss, bossID := h.staff("pos", nil, true, emp{name: "Atasan Dinas"})
	staff, own := h.staff("pos", nil, true, emp{name: "Crew Dinas", boss: &bossID})
	other, otherID := h.staff("pos", nil, true, emp{name: "Rekan Lain"})
	store := h.branch("Toko Dinas")
	h.exec(`INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date) VALUES
		($1, $3, true, '2026-01-01'), ($2, $3, true, '2026-01-01')`, own, otherID, store)
	photo := dataURL("image/png", pngBytes(t))
	base := "/api/hris/business-trips"
	trip := func(start, end string) map[string]any {
		return map[string]any{"start_date": start, "end_date": end, "destination": "Gudang Cikarang", "purpose": "Stock opname gudang"}
	}
	decide := func(s *testutil.Staff, id, action, reason string) call {
		body := map[string]any{"action": action}
		if reason != "" {
			body["rejection_reason"] = reason
		}
		return h.do(s, "POST", base+"/"+id+"/decide", body)
	}

	// Validasi tanggal (hari ini di harness = 2026-10-05).
	expect(t, h.do(&staff, "POST", base, trip("2026-10-04", "2026-10-05")), 400, "Tanggal mulai tidak boleh sebelum hari ini")
	expect(t, h.do(&staff, "POST", base, trip("2026-10-07", "2026-10-06")), 400, "Tanggal selesai tidak boleh sebelum tanggal mulai")
	expect(t, h.do(&staff, "POST", base, trip("2026-10-05", "2026-11-05")), 400, "Perjalanan Dinas maksimal 31 hari per pengajuan")
	expect(t, h.do(&staff, "POST", base, map[string]any{"start_date": "2026-10-05", "end_date": "2026-10-05",
		"destination": "Gudang", "purpose": "abc"}), 400, "Validation failed")
	expect(t, h.do(&staff, "POST", base, map[string]any{"employee_id": otherID, "start_date": "2026-10-05",
		"end_date": "2026-10-05", "destination": "Gudang", "purpose": "Audit toko"}), 403,
		"Hanya HRD yang bisa mencatat Perjalanan Dinas karyawan lain")

	created := h.do(&staff, "POST", base, trip("2026-10-05", "2026-10-07"))
	expect(t, created, 200, "")
	d := created.data()
	id := d["id"].(string)
	if d["status"] != "pending" || d["start_date"] != "2026-10-05" || d["end_date"] != "2026-10-07" ||
		created.body["message"] != "Pengajuan Perjalanan Dinas terkirim — menunggu persetujuan atasan" {
		t.Fatalf("created = %s", created.raw)
	}
	expect(t, h.do(&staff, "POST", base, trip("2026-10-07", "2026-10-08")), 400,
		"Sudah ada Perjalanan Dinas menunggu/disetujui yang beririsan dengan tanggal ini")

	// Daftar: milik sendiri, antrean atasan, dan rekan lain tidak melihatnya.
	if n := len(h.do(&staff, "GET", base, nil).list()); n != 1 {
		t.Fatalf("own list = %d", n)
	}
	if n := len(h.do(&boss, "GET", base+"?scope=approvals&status=pending", nil).list()); n != 1 {
		t.Fatalf("boss queue = %d", n)
	}
	if n := len(h.do(&other, "GET", base+"?scope=approvals", nil).list()); n != 0 {
		t.Fatalf("other queue = %d", n)
	}
	expect(t, h.do(&hr, "GET", base+"?status=nope", nil), 400, "Status tidak valid")

	// Sebelum disetujui: absen di luar area tetap butuh keterangan, tercatat Invalid.
	in := h.do(&staff, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, "Berangkat ke gudang"))
	expect(t, in, 200, "")
	attID := in.data()["id"].(string)
	if in.data()["clock_in_geofence"] != "invalid" {
		t.Fatalf("before approval = %s", in.raw)
	}

	expect(t, decide(&staff, id, "approve", ""), 403, "Tidak bisa memutuskan Perjalanan Dinas sendiri")
	expect(t, decide(&other, id, "approve", ""), 403, "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini")
	expect(t, decide(&boss, id, "reject", ""), 400, "Alasan penolakan wajib diisi")
	approved := decide(&boss, id, "approve", "")
	expect(t, approved, 200, "")
	if approved.data()["status"] != "approved" || approved.body["message"] != "Perjalanan Dinas disetujui" {
		t.Fatalf("approved = %s", approved.raw)
	}
	expect(t, decide(&hr, id, "approve", ""), 400, "Pengajuan sudah approved")
	// Absen pagi tadi ikut menjadi Perjalanan Dinas (R8).
	if g := h.scalar(`SELECT clock_in_geofence FROM hris.attendance WHERE id = $1`, attID); g != "dinas" {
		t.Fatalf("relabel = %q", g)
	}
	// Clock-out jauh pada tanggal dinas: tanpa keterangan.
	co := h.do(&staff, "POST", "/api/hris/attendance", map[string]any{"action": "clock-out", "attendance_id": attID,
		"photo": photo, "clock_out_location": farFromStore})
	expect(t, co, 200, "")
	if co.data()["clock_out_geofence"] != "dinas" || co.data()["clock_out_distance_m"] != 334.0 {
		t.Fatalf("clock-out on trip = %s", co.raw)
	}

	// HRD mencatat untuk karyawan lain: langsung disetujui, absen jauh tanpa keterangan.
	assigned := h.do(&hr, "POST", base, map[string]any{"employee_id": otherID, "start_date": "2026-10-05",
		"end_date": "2026-10-05", "destination": "Kantor pusat", "purpose": "Rapat bulanan"})
	expect(t, assigned, 200, "")
	if assigned.data()["status"] != "approved" || assigned.body["message"] != "Perjalanan Dinas dicatat dan disetujui" {
		t.Fatalf("assigned = %s", assigned.raw)
	}
	onTrip := h.do(&other, "POST", "/api/hris/attendance", clockInBody(photo, farFromStore, ""))
	expect(t, onTrip, 200, "")
	if onTrip.data()["clock_in_geofence"] != "dinas" {
		t.Fatalf("clock-in on trip = %s", onTrip.raw)
	}

	// Pembatalan: pemohon tidak bisa membatalkan yang sudah disetujui, HRD bisa.
	expect(t, decide(&staff, id, "cancel", ""), 403, "Hanya pemohon (saat menunggu) atau HRD yang bisa membatalkan Perjalanan Dinas")
	expect(t, decide(&hr, id, "cancel", ""), 200, "")

	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE entity = 'hris.business_trips' AND entity_id = $1`, id); n != "2" {
		t.Fatalf("audit rows (approve + cancel) = %s", n)
	}
	if n := h.scalar(`SELECT count(*)::text FROM audit.audit_log WHERE action = 'business_trip.assign'
		AND entity_id = $1`, assigned.data()["id"]); n != "1" {
		t.Fatalf("audit assign = %s", n)
	}
}
```

(`clockInBody` dan `farFromStore` dari Task 2.)

- [ ] **Step 4: Jalankan test, pastikan gagal**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -run 'TestValidateBusinessTripDates|TestCanDecideBusinessTrip|TestBusinessTripFlow' -count=1 -v`
Expected: FAIL (build error `undefined: ValidateBusinessTripDates`).

- [ ] **Step 5: Implementasi domain**

`backend/internal/modules/hris/domain/business_trip.go`:

```go
package domain

import "time"

// Perjalanan Dinas (EPIC-062, MOM AB-02): an employee request over a date
// range, decided by the direct manager or HR; approved dates are exempt
// from the attendance geofence.

// BusinessTripMaxDays is the longest request, inclusive (EPIC-062 R7).
const BusinessTripMaxDays = 31

// ValidateBusinessTripDates: real dates, start not before today (WIB), end
// not before start, at most BusinessTripMaxDays days; "" when valid.
func ValidateBusinessTripDates(start, end, today string) string {
	s, err1 := time.Parse(DateLayout, start)
	e, err2 := time.Parse(DateLayout, end)
	if err1 != nil || err2 != nil {
		return "Format tanggal tidak valid"
	}
	if start < today {
		return "Tanggal mulai tidak boleh sebelum hari ini"
	}
	if e.Before(s) {
		return "Tanggal selesai tidak boleh sebelum tanggal mulai"
	}
	if int(e.Sub(s).Hours()/24)+1 > BusinessTripMaxDays {
		return "Perjalanan Dinas maksimal 31 hari per pengajuan"
	}
	return ""
}

// TripActor is who decides.
type TripActor struct {
	EmployeeID *string
	IsHR       bool
}

// TripRef is the trip being decided.
type TripRef struct {
	EmployeeID  string
	ReportingTo *string
	Status      string
}

// CanDecideBusinessTrip: approve/reject by the direct manager or HR, never
// the traveller (Q-APV-3 pattern); cancel by the traveller while pending,
// or by HR while pending or approved. "" reason means allowed.
func CanDecideBusinessTrip(a TripActor, t TripRef, action string) (bool, string) {
	self := samePerson(a.EmployeeID, &t.EmployeeID)
	switch action {
	case "cancel":
		if (t.Status == "pending" && (self || a.IsHR)) || (t.Status == "approved" && a.IsHR) {
			return true, ""
		}
		return false, "Hanya pemohon (saat menunggu) atau HRD yang bisa membatalkan Perjalanan Dinas"
	case "approve", "reject":
		if self {
			return false, "Tidak bisa memutuskan Perjalanan Dinas sendiri"
		}
		if !a.IsHR && !samePerson(a.EmployeeID, t.ReportingTo) {
			return false, "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini"
		}
		return true, ""
	}
	return false, "Aksi tidak dikenal"
}
```

(`samePerson` sudah ada di `domain/workforce.go`.)

- [ ] **Step 6: Implementasi service, SQL, dan handler**

`backend/internal/modules/hris/business_trips.go`:

```go
package hris

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"nuhabit/backend/internal/modules/hris/domain"
	"nuhabit/backend/internal/platform/audit"
	"nuhabit/backend/internal/platform/database"
	"nuhabit/backend/internal/platform/httpx"
	"nuhabit/backend/internal/platform/validate"
)

// Perjalanan Dinas (EPIC-062 R7/R8, MOM AB-02): hris.business_trips.

// TripRepo is the business trip storage.
type TripRepo interface {
	TripList(ctx context.Context, f tripFilter) ([]*Row, error)
	TripByID(ctx context.Context, id string) (*Row, error)
	InsertTrip(ctx context.Context, f fields) (*Row, error)
	DecideTrip(ctx context.Context, id, fromStatus string, f fields) (*Row, error)
	ApprovedTripOn(ctx context.Context, employeeID, date string) (bool, error)
	RelabelTripAttendance(ctx context.Context, employeeID, start, end string) error
}

type tripFilter struct {
	EmployeeIDs []string
	Status      string
}

type tripInput struct {
	EmployeeID                               *string
	StartDate, EndDate, Destination, Purpose string
}

var (
	tripStatuses = []string{"pending", "approved", "rejected", "cancelled"}
	tripNext     = map[string]string{"approve": "approved", "reject": "rejected", "cancel": "cancelled"}
	tripMessages = map[string]string{"approved": "Perjalanan Dinas disetujui", "rejected": "Perjalanan Dinas ditolak",
		"cancelled": "Perjalanan Dinas dibatalkan"}
)

const tripOverlap = "Sudah ada Perjalanan Dinas menunggu/disetujui yang beririsan dengan tanggal ini"

// ListTrips: scope=approvals → the direct reports' trips; HR → all (or one
// employee, "me" = own); others → own.
func (s *Service) ListTrips(ctx context.Context, a *Actor, scope, status, employeeID string) ([]*Row, error) {
	f := tripFilter{Status: status}
	switch {
	case scope == "approvals":
		if a.EmployeeID == nil {
			return []*Row{}, nil
		}
		ids, err := s.repo.SubordinateIDs(ctx, *a.EmployeeID)
		if err != nil || len(ids) == 0 {
			return []*Row{}, err
		}
		f.EmployeeIDs = ids
	case a.IsHR && employeeID == "me":
		if a.EmployeeID == nil {
			return []*Row{}, nil
		}
		f.EmployeeIDs = []string{*a.EmployeeID}
	case a.IsHR:
		if employeeID != "" {
			f.EmployeeIDs = []string{employeeID}
		}
	default:
		if a.EmployeeID == nil {
			return []*Row{}, nil
		}
		f.EmployeeIDs = []string{*a.EmployeeID}
	}
	return s.repo.TripList(ctx, f)
}

// CreateTrip: a request for oneself (pending) or an HR assignment for
// another employee (approved at once).
func (s *Service) CreateTrip(ctx context.Context, a *Actor, in tripInput, meta audit.Entry) (*Row, error) {
	target := a.EmployeeID
	if in.EmployeeID != nil && *in.EmployeeID != "" && *in.EmployeeID != "me" {
		target = in.EmployeeID
	}
	if target == nil {
		return nil, httpx.BadRequest("Akun ini tidak tertaut ke data karyawan")
	}
	self := a.EmployeeID != nil && *target == *a.EmployeeID
	if !self && !a.IsHR {
		return nil, httpx.Forbidden("Hanya HRD yang bisa mencatat Perjalanan Dinas karyawan lain")
	}
	if msg := domain.ValidateBusinessTripDates(in.StartDate, in.EndDate, domain.TodayWIB(s.now())); msg != "" {
		return nil, httpx.BadRequest(msg)
	}
	emp, err := s.repo.OvertimeTarget(ctx, *target)
	if err != nil {
		return nil, err
	}
	if emp == nil || emp.Get("is_active") == false {
		return nil, httpx.NotFound("Karyawan tidak ditemukan/nonaktif")
	}
	status := "pending"
	if !self {
		status = "approved"
	}
	var f fields
	f.add("employee_id", *target)
	f.add("start_date", in.StartDate)
	f.add("end_date", in.EndDate)
	f.add("destination", in.Destination)
	f.add("purpose", in.Purpose)
	f.add("status", status)
	f.add("requested_by", a.EmployeeID)
	if status == "approved" {
		f.add("decided_by", a.EmployeeID)
		f.add("decided_by_name", actorName(a))
		f.add("decided_at", s.clock())
	}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.InsertTrip(ctx, f); err != nil || status != "approved" {
			return err
		}
		if err := r.RelabelTripAttendance(ctx, *target, in.StartDate, in.EndDate); err != nil {
			return err
		}
		return s.auditTrip(ctx, r, a, "business_trip.assign", nil, row, meta)
	})
	if database.PgCode(err) == "23P01" {
		return nil, httpx.BadRequest(tripOverlap)
	}
	if err != nil {
		return nil, err
	}
	message := "Pengajuan Perjalanan Dinas terkirim — menunggu persetujuan atasan"
	if status == "approved" {
		message = "Perjalanan Dinas dicatat dan disetujui"
	}
	return obj("data", row, "message", message), nil
}

// DecideTrip approves, rejects or cancels (domain.CanDecideBusinessTrip);
// the update is guarded on the status read, and an approval relabels the
// range's existing attendance (R8).
func (s *Service) DecideTrip(ctx context.Context, a *Actor, id, action string, rejection *string, meta audit.Entry) (*Row, error) {
	trip, err := s.repo.TripByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if trip == nil {
		return nil, httpx.NotFound("Perjalanan Dinas tidak ditemukan")
	}
	status := trip.Str("status")
	if action != "cancel" && status != "pending" {
		return nil, httpx.BadRequest("Pengajuan sudah "+status, obj("current_status", status))
	}
	var reportingTo *string
	if emp := trip.Child("employee"); emp != nil {
		reportingTo = emp.StrPtr("reporting_to")
	}
	allowed, reason := domain.CanDecideBusinessTrip(domain.TripActor{EmployeeID: a.EmployeeID, IsHR: a.IsHR},
		domain.TripRef{EmployeeID: trip.Str("employee_id"), ReportingTo: reportingTo, Status: status}, action)
	if !allowed {
		return nil, httpx.Forbidden(reason)
	}
	if action == "reject" && (rejection == nil || domain.JSTrim(*rejection) == "") {
		return nil, httpx.BadRequest("Alasan penolakan wajib diisi")
	}
	next := tripNext[action]
	var f fields
	f.add("status", next)
	f.add("decided_by", a.EmployeeID)
	f.add("decided_by_name", actorName(a))
	f.add("decided_at", s.clock())
	if action == "reject" {
		f.add("rejection_reason", domain.JSTrim(*rejection))
	}
	var row *Row
	err = s.repo.InTx(ctx, func(r Repository) error {
		var err error
		if row, err = r.DecideTrip(ctx, id, status, f); err != nil {
			return err
		}
		if next == "approved" {
			if err := r.RelabelTripAttendance(ctx, trip.Str("employee_id"), trip.Str("start_date"), trip.Str("end_date")); err != nil {
				return err
			}
		}
		return s.auditTrip(ctx, r, a, "business_trip."+action, trip, row, meta)
	})
	if errors.Is(err, errNoRow) {
		return nil, httpx.Conflict("Pengajuan sudah diproses oleh orang lain")
	}
	if err != nil {
		return nil, err
	}
	return obj("data", row, "message", tripMessages[next]), nil
}

// auditTrip records a decision or HR assignment (LA-01).
func (s *Service) auditTrip(ctx context.Context, r Repository, a *Actor, action string, before, after *Row, meta audit.Entry) error {
	meta.ActorID, meta.ActorName = a.UserID, ptr(actorName(a))
	meta.Action, meta.Entity, meta.EntityID = action, "hris.business_trips", ptr(after.Str("id"))
	label := after.Str("start_date") + " s.d. " + after.Str("end_date")
	if emp := after.Child("employee"); emp != nil {
		label = emp.Str("full_name") + " · " + label
	}
	meta.EntityLabel = ptr(label)
	if before != nil {
		meta.Before = map[string]any{"status": before.Str("status")}
	}
	meta.After = map[string]any{"status": after.Str("status"), "rejection_reason": after.Get("rejection_reason")}
	return audit.Write(ctx, r.querier(), meta)
}

/* ── SQL ─────────────────────────────────────────────────────────────── */

var tripColumns = `t.id, t.employee_id, t.start_date::text AS start_date, t.end_date::text AS end_date,
	t.destination, t.purpose, t.status, t.requested_by, t.decided_by, t.decided_by_name, t.decided_at,
	t.rejection_reason, t.created_at, t.updated_at, ` +
	embedOne("employee", "id, full_name, nip, reporting_to", "hris.employees", "id = t.employee_id")

func (s *store) TripList(ctx context.Context, f tripFilter) ([]*Row, error) {
	where := []string{"true"}
	var args []any
	if len(f.EmployeeIDs) > 0 {
		args = append(args, f.EmployeeIDs)
		where = append(where, fmt.Sprintf("t.employee_id = ANY($%d::uuid[])", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where = append(where, fmt.Sprintf("t.status = $%d", len(args)))
	}
	rows, err := queryRows(ctx, s.db, `SELECT `+tripColumns+` FROM hris.business_trips t WHERE `+
		strings.Join(where, " AND ")+` ORDER BY t.start_date DESC, t.created_at DESC LIMIT 200`, args...)
	return rows, ignoreBadInput(err)
}

func (s *store) TripByID(ctx context.Context, id string) (*Row, error) {
	row, err := queryRow(ctx, s.db, `SELECT `+tripColumns+` FROM hris.business_trips t WHERE t.id = $1`, id)
	return row, ignoreBadInput(err)
}

func (s *store) InsertTrip(ctx context.Context, f fields) (*Row, error) {
	row, err := insertRow(ctx, s.db, "hris.business_trips", f, "id")
	if err != nil || row == nil {
		return nil, err
	}
	return s.TripByID(ctx, row.Str("id"))
}

func (s *store) DecideTrip(ctx context.Context, id, fromStatus string, f fields) (*Row, error) {
	if _, err := updateOne(ctx, s.db, "hris.business_trips", f, fields{{"id", id}, {"status", fromStatus}}, "id"); err != nil {
		return nil, err
	}
	return s.TripByID(ctx, id)
}

func (s *store) ApprovedTripOn(ctx context.Context, employeeID, date string) (bool, error) {
	var ok bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM hris.business_trips
		WHERE employee_id = $1 AND status = 'approved' AND $2::date BETWEEN start_date AND end_date)`,
		employeeID, date).Scan(&ok)
	return ok, err
}

// RelabelTripAttendance turns the range's invalid/unchecked geofence
// verdicts into dinas, outside closed attendance periods (EPIC-062 R8).
func (s *store) RelabelTripAttendance(ctx context.Context, employeeID, start, end string) error {
	_, err := s.db.Exec(ctx, `UPDATE hris.attendance a SET
		  clock_in_geofence = CASE WHEN a.clock_in_geofence IN ('invalid', 'unchecked') THEN 'dinas' ELSE a.clock_in_geofence END,
		  clock_out_geofence = CASE WHEN a.clock_out_geofence IN ('invalid', 'unchecked') THEN 'dinas' ELSE a.clock_out_geofence END
		WHERE a.employee_id = $1 AND a.date BETWEEN $2::date AND $3::date
		  AND (a.clock_in_geofence IN ('invalid', 'unchecked') OR a.clock_out_geofence IN ('invalid', 'unchecked'))
		  AND NOT EXISTS (SELECT 1 FROM hris.attendance_periods p
		                  WHERE p.status = 'closed' AND a.date BETWEEN p.start_date AND p.end_date)`,
		employeeID, start, end)
	return err
}

/* ── HTTP ────────────────────────────────────────────────────────────── */

// GET /api/hris/business-trips?scope=approvals&status&employee_id
func (h handlers) listTrips(w http.ResponseWriter, r *http.Request, a *Actor) error {
	q := r.URL.Query()
	status := q.Get("status")
	if status != "" && !slices.Contains(tripStatuses, status) {
		return httpx.BadRequest("Status tidak valid")
	}
	employeeID := q.Get("employee_id")
	if employeeID != "" && employeeID != "me" && !domain.IsUUID(employeeID) {
		return httpx.BadRequest("ID karyawan tidak valid")
	}
	rows, err := h.svc.ListTrips(r.Context(), a, q.Get("scope"), status, employeeID)
	if err != nil {
		return err
	}
	return ok(w, dataOf(rows))
}

// POST /api/hris/business-trips { employee_id?, start_date, end_date, destination, purpose }
func (h handlers) createTrip(w http.ResponseWriter, r *http.Request, a *Actor) error {
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	var in tripInput
	in.EmployeeID = f.Str("employee_id", optional, validate.StrOpts{Check: func(s string) (string, string, bool) {
		return "invalid_union", "Invalid input", s == "me" || domain.UUIDRe.MatchString(s)
	}})
	in.StartDate = strOr(f.Str("start_date", validate.Rule{}, validate.StrOpts{Check: regexOf(domain.DateRe)}))
	in.EndDate = strOr(f.Str("end_date", validate.Rule{}, validate.StrOpts{Check: regexOf(domain.DateRe)}))
	in.Destination = strOr(f.Str("destination", validate.Rule{}, validate.StrOpts{Trim: true, Max: 200,
		Check: minLen(3, "Tujuan minimal 3 karakter")}))
	in.Purpose = strOr(f.Str("purpose", validate.Rule{}, validate.StrOpts{Trim: true, Max: 1000,
		Check: minLen(5, "Keperluan minimal 5 karakter")}))
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.CreateTrip(r.Context(), a, in, audit.Entry{}.WithRequest(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}

// POST /api/hris/business-trips/{id}/decide { action, rejection_reason? }
func (h handlers) decideTrip(w http.ResponseWriter, r *http.Request, a *Actor) error {
	id, err := requireUUID(r.PathValue("id"), "ID Perjalanan Dinas tidak valid")
	if err != nil {
		return err
	}
	const message = "Validation failed"
	f, err := readForm(r, message)
	if err != nil {
		return err
	}
	action := f.Enum("action", validate.Rule{}, []string{"approve", "reject", "cancel"})
	rejection := f.Str("rejection_reason", optional, validate.StrOpts{Max: 500})
	if err := formErr(f, message); err != nil {
		return err
	}
	out, err := h.svc.DecideTrip(r.Context(), a, id, *action, rejection, audit.Entry{}.WithRequest(r))
	if err != nil {
		return err
	}
	return ok(w, out)
}
```

Catatan: cek signature `minLen` (`input.go:62`) dan bahwa `StrOpts.Trim` dijalankan sebelum `Check`; bila urutannya terbalik, ganti `Check` dengan pemeriksaan panjang setelah `strings.TrimSpace` di service dengan pesan yang sama.

`service.go`: tambahkan `TripRepo` ke `Repository` (setelah `PlacementRepo`).

`routes.go`, setelah blok overtime:

```go
		// business trips (Perjalanan Dinas, EPIC-062)
		r("GET /api/hris/business-trips", h.withActor(h.listTrips)),
		r("POST /api/hris/business-trips", h.withActor(h.createTrip)),
		r("POST /api/hris/business-trips/{id}/decide", h.withActor(h.decideTrip)),
```

`attendance_geofence.go`, di `geofenceFor` tepat sebelum `return &clockGeofence{...}`:

```go
	onTrip, err := s.repo.ApprovedTripOn(ctx, employeeID, date)
	if err != nil {
		return nil, err
	}
	in.OnBusinessTrip = onTrip
```

- [ ] **Step 7: Jalankan test, pastikan lulus, lalu paket hris**

Run: perintah Step 4. Expected: PASS.
Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris/... -count=1`
Expected: sama dengan baseline.

- [ ] **Step 8: Commit**

```bash
gofmt -l backend/internal/modules/hris
git add backend/database/migrations/deltas/20261023090100_business_trips.sql backend/internal/modules/hris/domain/business_trip.go backend/internal/modules/hris/domain/business_trip_test.go backend/internal/modules/hris/business_trips.go backend/internal/modules/hris/business_trips_integration_test.go backend/internal/modules/hris/service.go backend/internal/modules/hris/routes.go backend/internal/modules/hris/attendance_geofence.go
git commit -m "feat(hris): Perjalanan Dinas (pengajuan, approval atasan/HRD, audit) membebaskan geofence absensi, EPIC-062"
```

---

### Task 5: Geofence tampil di Monitoring dan Rekap absensi HR

**Files:**
- Modify: `backend/internal/modules/hris/postgres_attendance.go` (`attendanceListColumns`, `RosterRows`)
- Modify: `backend/internal/modules/hris/attendance.go` (`buildRoster`)
- Modify: `backend/internal/modules/hris/attendance_geofence_integration_test.go` (test baru `TestGeofenceInRosterAndList`)
- Create: `frontend/src/lib/hris/geofence.ts`
- Create: `frontend/src/lib/hris/geofence.test.ts`
- Create: `frontend/src/features/hris/attendance/components/geofence-badge.tsx`
- Modify: `frontend/src/features/hris/attendance/types.ts` (`RosterAttendance`, `AttendanceListRow`)
- Modify: `frontend/src/features/hris/attendance/components/roster-row.tsx`
- Modify: `frontend/src/features/hris/attendance/components/attendance-recap-tab.tsx`

**Interfaces:**
- Consumes: kolom geofence (Task 2).
- Produces:
  - API: baris `GET /api/hris/attendance` memuat `clock_in_geofence, clock_in_distance_m, clock_out_geofence, clock_out_distance_m`; `daily-roster` `employees[].attendance` memuat kolom yang sama plus `notes`.
  - TS `frontend/src/lib/hris/geofence.ts`: `type GeofenceStatus = "valid" | "invalid" | "unchecked" | "dinas"`, `GEOFENCE_META: Record<GeofenceStatus, { label: string; className: string }>`, `isGeofenceStatus(v: unknown): v is GeofenceStatus`, `geofenceLabel(status: string | null | undefined, distanceM: number | null | undefined): string | null`.
  - Komponen `GeofenceBadge({ status, distanceM, prefix })`.

- [ ] **Step 1: Tulis test Go yang gagal**

Tambahkan ke `attendance_geofence_integration_test.go`:

```go
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
```

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./internal/modules/hris -run TestGeofenceInRosterAndList -count=1 -v`
Expected: FAIL (`roster attendance = map[...]` tanpa `clock_in_geofence`).

- [ ] **Step 2: Implementasi Go**

`postgres_attendance.go`, `attendanceListColumns`: ubah baris `is_overtime, overtime_hours, validated_at, is_manual, adjusted_at, adjusted_by_name, adjustment_count, ` menjadi:

```go
  is_overtime, overtime_hours, validated_at, is_manual, adjusted_at, adjusted_by_name, adjustment_count,
  clock_in_geofence, clock_in_distance_m, clock_out_geofence, clock_out_distance_m, ` +
```

`RosterRows`: setelah `a.shift_id AS attendance_shift_id,` tambahkan:

```sql
		       a.clock_in_geofence, a.clock_in_distance_m, a.clock_out_geofence, a.clock_out_distance_m,
		       a.notes AS attendance_notes,
```

`attendance.go` `buildRoster`, ubah pembentukan `attendance`:

```go
			attendance = obj("id", row.Get("attendance_id"), "clock_in", row.Get("clock_in"), "clock_out", row.Get("clock_out"),
				"work_hours", hours, "is_late", row.Bool("is_late"), "late_minutes", int64(row.Num("late_minutes")),
				"clock_in_photo_url", row.Get("clock_in_photo_url"), "clock_out_photo_url", row.Get("clock_out_photo_url"),
				"shift_id", row.Get("attendance_shift_id"),
				"clock_in_geofence", row.Get("clock_in_geofence"), "clock_in_distance_m", row.Get("clock_in_distance_m"),
				"clock_out_geofence", row.Get("clock_out_geofence"), "clock_out_distance_m", row.Get("clock_out_distance_m"),
				"notes", row.Get("attendance_notes"))
```

Run: perintah Step 1 lalu `go test ./internal/modules/hris/... -count=1` (dengan `TEST_DATABASE_URL`).
Expected: PASS; paket sama dengan baseline (`TestWorkforce` hanya mengunci urutan `summary`).

- [ ] **Step 3: Tulis test frontend yang gagal**

`frontend/src/lib/hris/geofence.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { GEOFENCE_META, geofenceLabel, isGeofenceStatus } from "./geofence";

describe("geofenceLabel", () => {
  it("menulis status dan jarak", () => {
    expect(geofenceLabel("valid", 56)).toBe("Dalam area · 56 m");
    expect(geofenceLabel("invalid", 334)).toBe("Di luar area · 334 m");
    expect(geofenceLabel("unchecked", null)).toBe("Belum dicek");
    expect(geofenceLabel("dinas", 334)).toBe("Perjalanan Dinas");
  });
  it("kosong untuk absen tanpa penilaian (sebelum EPIC-062 / dicatat HRD)", () => {
    expect(geofenceLabel(null, null)).toBeNull();
    expect(geofenceLabel("lainnya", 10)).toBeNull();
  });
  it("mengenali status yang valid saja", () => {
    expect(isGeofenceStatus("dinas")).toBe(true);
    expect(isGeofenceStatus("remote")).toBe(false);
    expect(GEOFENCE_META.invalid.label).toBe("Di luar area");
  });
});
```

Run: `cd frontend && npx vitest run src/lib/hris/geofence.test.ts`
Expected: FAIL (`Failed to resolve import "./geofence"`).

- [ ] **Step 4: Implementasi frontend**

`frontend/src/lib/hris/geofence.ts`:

```ts
/**
 * Geofence absensi (EPIC-062, MOM AB-01/AB-03): label status hasil penilaian
 * server untuk ESS dan halaman Absensi HRD. Status dihitung di server; file
 * ini tidak menghitung jarak.
 */

export type GeofenceStatus = "valid" | "invalid" | "unchecked" | "dinas";

export const GEOFENCE_META: Record<GeofenceStatus, { label: string; className: string }> = {
  valid: { label: "Dalam area", className: "bg-green-100 text-green-700" },
  invalid: { label: "Di luar area", className: "bg-red-100 text-red-700" },
  unchecked: { label: "Belum dicek", className: "bg-gray-100 text-gray-600" },
  dinas: { label: "Perjalanan Dinas", className: "bg-sky-100 text-sky-700" },
};

export function isGeofenceStatus(v: unknown): v is GeofenceStatus {
  return typeof v === "string" && v in GEOFENCE_META;
}

/** "Di luar area · 334 m"; null bila absen tidak dinilai (sebelum EPIC-062 atau dicatat HRD). */
export function geofenceLabel(
  status: string | null | undefined,
  distanceM: number | null | undefined
): string | null {
  if (!isGeofenceStatus(status)) return null;
  const label = GEOFENCE_META[status].label;
  if ((status === "valid" || status === "invalid") && distanceM != null) return `${label} · ${distanceM} m`;
  return label;
}
```

`frontend/src/features/hris/attendance/components/geofence-badge.tsx`:

```tsx
import { MapPin } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { GEOFENCE_META, geofenceLabel, isGeofenceStatus } from "@/lib/hris/geofence";

/** Badge status geofence satu clock-in/out; tidak dirender bila absen tidak dinilai. */
export function GeofenceBadge({
  status,
  distanceM,
  prefix,
}: {
  status: string | null | undefined;
  distanceM?: number | null;
  prefix?: string;
}) {
  if (!isGeofenceStatus(status)) return null;
  return (
    <Badge className={`gap-1 ${GEOFENCE_META[status].className}`}>
      <MapPin className="h-3 w-3" />
      {prefix ? `${prefix}: ` : ""}
      {geofenceLabel(status, distanceM)}
    </Badge>
  );
}
```

`frontend/src/features/hris/attendance/types.ts`:
- Tambahkan di atas file: `import type { GeofenceStatus } from "@/lib/hris/geofence";`
- Di `RosterAttendance` setelah `shift_id: string | null;`:

```ts
  /** Geofence clock-in/out (EPIC-062); null = tidak dinilai */
  clock_in_geofence: GeofenceStatus | null;
  clock_in_distance_m: number | null;
  clock_out_geofence: GeofenceStatus | null;
  clock_out_distance_m: number | null;
  /** Keterangan karyawan (wajib bila di luar area) */
  notes: string | null;
```

- Di `AttendanceListRow` setelah `adjustment_count?: number;`:

```ts
  clock_in_geofence?: GeofenceStatus | null;
  clock_in_distance_m?: number | null;
  clock_out_geofence?: GeofenceStatus | null;
  clock_out_distance_m?: number | null;
```

`roster-row.tsx`: impor `import { GeofenceBadge } from "./geofence-badge";`, lalu di blok `emp.attendance ? (...)`, setelah `<div className="mt-0.5 flex justify-end gap-2">…</div>` (selfie), tambahkan:

```tsx
          <div className="mt-1 flex flex-wrap justify-end gap-1">
            <GeofenceBadge status={emp.attendance.clock_in_geofence} distanceM={emp.attendance.clock_in_distance_m} prefix="Masuk" />
            <GeofenceBadge status={emp.attendance.clock_out_geofence} distanceM={emp.attendance.clock_out_distance_m} prefix="Pulang" />
          </div>
          {emp.attendance.notes &&
            (emp.attendance.clock_in_geofence === "invalid" || emp.attendance.clock_out_geofence === "invalid") && (
              <p className="mt-0.5 max-w-xs whitespace-pre-line text-right text-[11px] text-gray-500">“{emp.attendance.notes}”</p>
            )}
```

`attendance-recap-tab.tsx`: impor `import { GeofenceBadge } from "./geofence-badge";`. Di `<thead>` tambahkan `<th className="p-3">Lokasi</th>` setelah `<th className="p-3">Status</th>`. Di `<tbody>`, setelah `<td>` kolom Status tambahkan:

```tsx
                      <td className="p-3">
                        <div className="flex flex-col items-start gap-1">
                          <GeofenceBadge status={row.clock_in_geofence} distanceM={row.clock_in_distance_m} prefix="Masuk" />
                          <GeofenceBadge status={row.clock_out_geofence} distanceM={row.clock_out_distance_m} prefix="Pulang" />
                          {!row.clock_in_geofence && !row.clock_out_geofence && (
                            <span className="text-xs text-gray-400">—</span>
                          )}
                          {row.notes && (row.clock_in_geofence === "invalid" || row.clock_out_geofence === "invalid") && (
                            <p className="max-w-[16rem] whitespace-pre-line text-xs text-gray-500">“{row.notes}”</p>
                          )}
                        </div>
                      </td>
```

Bila tabel memakai `colSpan` untuk baris kosong/loading, tambah 1.

- [ ] **Step 5: Jalankan test frontend, tsc, eslint**

Run: `cd frontend && npx vitest run src/lib/hris/geofence.test.ts src/features/hris/attendance`
Expected: PASS.
Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/lib/hris/geofence.ts src/features/hris/attendance`
Expected: bersih.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/modules/hris/postgres_attendance.go backend/internal/modules/hris/attendance.go backend/internal/modules/hris/attendance_geofence_integration_test.go frontend/src/lib/hris/geofence.ts frontend/src/lib/hris/geofence.test.ts frontend/src/features/hris/attendance
git commit -m "feat(hris): status, jarak, dan keterangan geofence tampil di Monitoring dan Rekap absensi HRD, EPIC-062"
```

---

### Task 6: Popup ESS di luar area dengan keterangan wajib

**Files:**
- Modify: `frontend/src/lib/hris/geofence.ts` (`GEOFENCE_NOTE_MIN`, `GeofenceNoteInfo`, `geofenceNoteRequired`)
- Modify: `frontend/src/lib/hris/geofence.test.ts`
- Modify: `frontend/src/features/hris/ess/types.ts` (`EssClockPayload.notes`, `EssTodayAttendance` geofence, `EssClockResponse`)
- Modify: `frontend/src/features/hris/ess/api.ts` (`ClockGeofenceError`, `submitClock`)
- Create: `frontend/src/features/hris/ess/components/geofence-note-dialog.tsx`
- Create: `frontend/src/features/hris/ess/components/geofence-note-dialog.test.tsx`
- Modify: `frontend/src/features/hris/ess/components/ess-clock-panel.tsx`

**Interfaces:**
- Consumes: respons 422 `geofence_note_required` (Task 2); `GeofenceBadge` (Task 5); `geofenceLabel` (Task 5).
- Produces:
  - `const GEOFENCE_NOTE_MIN = 5`
  - `interface GeofenceNoteInfo { message: string; distanceM: number | null; radiusM: number; branchName: string | null }`
  - `geofenceNoteRequired(status: number, body: unknown): GeofenceNoteInfo | null`
  - `class ClockGeofenceError extends Error { readonly info: GeofenceNoteInfo }`
  - `submitClock(payload: EssClockPayload): Promise<EssClockResponse>`
  - `GeofenceNoteDialog({ info, pending, onSubmit, onCancel })`

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan ke `frontend/src/lib/hris/geofence.test.ts`:

```ts
import { geofenceNoteRequired } from "./geofence";

describe("geofenceNoteRequired", () => {
  const body = {
    success: false,
    error: "Anda di luar area PVJ (±334 m, batas 100 m). Isi keterangan minimal 5 karakter untuk tetap absen.",
    details: { code: "geofence_note_required", distance_m: 334, radius_m: 100, branch_name: "PVJ" },
  };
  it("membaca respons 422 di luar area", () => {
    expect(geofenceNoteRequired(422, body)).toEqual({
      message: body.error,
      distanceM: 334,
      radiusM: 100,
      branchName: "PVJ",
    });
  });
  it("mengabaikan error lain", () => {
    expect(geofenceNoteRequired(400, body)).toBeNull();
    expect(geofenceNoteRequired(422, { error: "Validation failed", details: [] })).toBeNull();
    expect(geofenceNoteRequired(422, null)).toBeNull();
  });
});
```

(Gabungkan import dengan import yang sudah ada di atas file.)

`frontend/src/features/hris/ess/components/geofence-note-dialog.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { GeofenceNoteDialog } from "./geofence-note-dialog";

const info = {
  message: "Anda di luar area PVJ (±334 m, batas 100 m). Isi keterangan minimal 5 karakter untuk tetap absen.",
  distanceM: 334,
  radiusM: 100,
  branchName: "PVJ",
};

afterEach(cleanup);

describe("GeofenceNoteDialog", () => {
  it("wajib keterangan minimal 5 karakter sebelum tetap absen", () => {
    const onSubmit = vi.fn();
    render(<GeofenceNoteDialog info={info} pending={false} onSubmit={onSubmit} onCancel={() => {}} />);
    expect(screen.getByText(/±334 m dari PVJ/)).toBeTruthy();
    const submit = screen.getByRole("button", { name: "Tetap absen" }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: " abc " } });
    expect(submit.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "  Ambil stok di gudang " } });
    expect(submit.disabled).toBe(false);
    fireEvent.click(submit);
    expect(onSubmit).toHaveBeenCalledWith("Ambil stok di gudang");
  });

  it("tidak tampil tanpa info", () => {
    render(<GeofenceNoteDialog info={null} pending={false} onSubmit={() => {}} onCancel={() => {}} />);
    expect(screen.queryByRole("button", { name: "Tetap absen" })).toBeNull();
  });
});
```

Run: `cd frontend && npx vitest run src/lib/hris/geofence.test.ts src/features/hris/ess/components/geofence-note-dialog.test.tsx`
Expected: FAIL (`geofenceNoteRequired` bukan fungsi; `./geofence-note-dialog` tidak ada).

- [ ] **Step 2: Implementasi**

`frontend/src/lib/hris/geofence.ts`, tambahkan:

```ts
/** Panjang minimal keterangan absen di luar area (MOM AB-03). */
export const GEOFENCE_NOTE_MIN = 5;

export interface GeofenceNoteInfo {
  message: string;
  distanceM: number | null;
  radiusM: number;
  branchName: string | null;
}

/**
 * Respons 422 `geofence_note_required` dari POST /api/hris/attendance: absen
 * di luar area belum tercatat dan perlu dikirim ulang dengan keterangan.
 * null untuk respons lain.
 */
export function geofenceNoteRequired(status: number, body: unknown): GeofenceNoteInfo | null {
  if (status !== 422 || !body || typeof body !== "object") return null;
  const { error, details } = body as { error?: unknown; details?: unknown };
  if (!details || typeof details !== "object") return null;
  const d = details as { code?: unknown; distance_m?: unknown; radius_m?: unknown; branch_name?: unknown };
  if (d.code !== "geofence_note_required") return null;
  return {
    message: typeof error === "string" ? error : "Anda di luar area toko. Isi keterangan untuk tetap absen.",
    distanceM: typeof d.distance_m === "number" ? d.distance_m : null,
    radiusM: typeof d.radius_m === "number" ? d.radius_m : 100,
    branchName: typeof d.branch_name === "string" ? d.branch_name : null,
  };
}
```

`frontend/src/features/hris/ess/types.ts`:
- Impor `import type { GeofenceStatus } from "@/lib/hris/geofence";`
- `EssTodayAttendance`, setelah `branch?: …`:

```ts
  clock_in_geofence?: GeofenceStatus | null;
  clock_in_distance_m?: number | null;
  clock_out_geofence?: GeofenceStatus | null;
  clock_out_distance_m?: number | null;
```

- `EssClockPayload`, setelah `clock_out_location?`:

```ts
  /** Keterangan; wajib (≥ 5 karakter) bila server menilai di luar area */
  notes?: string;
```

- Tambahkan:

```ts
export interface EssClockResponse {
  message?: string;
  data?: {
    is_late?: boolean;
    late_minutes?: number;
    clock_in_geofence?: GeofenceStatus | null;
    clock_in_distance_m?: number | null;
    clock_out_geofence?: GeofenceStatus | null;
    clock_out_distance_m?: number | null;
  };
}
```

`frontend/src/features/hris/ess/api.ts`: impor `EssClockResponse` dari `./types` dan `import { geofenceNoteRequired, type GeofenceNoteInfo } from "@/lib/hris/geofence";`, lalu ganti `submitClock`:

```ts
/** Absen di luar area tanpa keterangan: popup lalu kirim ulang payload yang sama + notes. */
export class ClockGeofenceError extends Error {
  readonly info: GeofenceNoteInfo;
  constructor(info: GeofenceNoteInfo) {
    super(info.message);
    this.name = "ClockGeofenceError";
    this.info = info;
  }
}

export async function submitClock(payload: EssClockPayload): Promise<EssClockResponse> {
  const res = await fetch("/api/hris/attendance", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const json: unknown = await res.json().catch(() => ({}));
  const note = geofenceNoteRequired(res.status, json);
  if (note) throw new ClockGeofenceError(note);
  if (!res.ok) {
    const error = (json as { error?: unknown } | null)?.error;
    throw new Error(typeof error === "string" && error.trim() ? error : `Request failed (${res.status})`);
  }
  return json as EssClockResponse;
}
```

`frontend/src/features/hris/ess/components/geofence-note-dialog.tsx`:

```tsx
"use client";

import { useState } from "react";
import { MapPin } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { GEOFENCE_NOTE_MIN, type GeofenceNoteInfo } from "@/lib/hris/geofence";

/**
 * Popup ESS saat server menilai absen di luar area toko (MOM AB-03): absen
 * tetap boleh, tercatat "Di luar area", dan wajib keterangan untuk HRD.
 */
export function GeofenceNoteDialog({
  info,
  pending,
  onSubmit,
  onCancel,
}: {
  info: GeofenceNoteInfo | null;
  pending: boolean;
  onSubmit: (note: string) => void;
  onCancel: () => void;
}) {
  const [note, setNote] = useState("");
  const trimmed = note.trim();
  const place = info?.branchName ?? "toko";
  return (
    <Dialog
      open={info !== null}
      onOpenChange={(open) => {
        if (!open) {
          setNote("");
          onCancel();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <MapPin className="h-5 w-5 text-red-600" /> Anda di luar area toko
          </DialogTitle>
          <DialogDescription>
            {info?.distanceM != null ? `Posisi Anda ±${info.distanceM} m dari ${place}` : `Posisi Anda di luar area ${place}`}{" "}
            (batas {info?.radiusM ?? 100} m). Absen tetap tercatat dengan status <b>Di luar area</b> dan ditinjau HRD.
          </DialogDescription>
        </DialogHeader>
        <label htmlFor="geofence-note" className="text-sm font-medium text-gray-700">
          Keterangan
        </label>
        <Textarea
          id="geofence-note"
          value={note}
          onChange={(e) => setNote(e.target.value)}
          placeholder="Mis. mengambil stok di gudang, antar barang ke pelanggan"
          rows={3}
        />
        <p className="text-xs text-gray-500">Minimal {GEOFENCE_NOTE_MIN} karakter.</p>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              setNote("");
              onCancel();
            }}
          >
            Batal
          </Button>
          <Button
            disabled={pending || [...trimmed].length < GEOFENCE_NOTE_MIN}
            onClick={() => {
              onSubmit(trimmed);
              setNote("");
            }}
          >
            Tetap absen
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

`frontend/src/features/hris/ess/components/ess-clock-panel.tsx`:
- Impor tambahan:

```tsx
import { GeofenceBadge } from "@/features/hris/attendance/components/geofence-badge";
import { geofenceLabel, type GeofenceNoteInfo } from "@/lib/hris/geofence";
import { ClockGeofenceError } from "../api";
import { GeofenceNoteDialog } from "./geofence-note-dialog";
```

- State baru di dalam komponen: `const [noteFor, setNoteFor] = useState<{ payload: EssClockPayload; info: GeofenceNoteInfo } | null>(null);`
- Ganti bagian `clock.mutate(payload, {...})` di `submit` dengan `send(payload);` dan tambahkan fungsi:

```tsx
  function send(payload: EssClockPayload) {
    clock.mutate(payload, {
      onSuccess: (json) => {
        const data = json.data;
        const late = data?.is_late ? ` — terlambat ${data.late_minutes} menit` : "";
        const geo =
          payload.action === "clock-in"
            ? geofenceLabel(data?.clock_in_geofence, data?.clock_in_distance_m)
            : geofenceLabel(data?.clock_out_geofence, data?.clock_out_distance_m);
        toast.success(payload.action === "clock-in" ? "✅ Clock-in berhasil" : "✅ Clock-out berhasil", {
          description: `${formatTime(new Date())}${late}${geo ? ` · 📍 ${geo}` : ""}`,
        });
      },
      onError: (error) => {
        if (error instanceof ClockGeofenceError) {
          setNoteFor({ payload, info: error.info });
          return;
        }
        toast.error("Gagal", { description: error.message || "Coba lagi" });
      },
    });
  }
```

- Di blok ringkasan `today && clockedIn`, setelah badge tepat waktu/terlambat:

```tsx
          <GeofenceBadge status={today.clock_in_geofence} distanceM={today.clock_in_distance_m} prefix="Masuk" />
          <GeofenceBadge status={today.clock_out_geofence} distanceM={today.clock_out_distance_m} prefix="Pulang" />
```

- Setelah `<CameraCapture …/>`:

```tsx
      <GeofenceNoteDialog
        info={noteFor?.info ?? null}
        pending={clock.isPending}
        onCancel={() => setNoteFor(null)}
        onSubmit={(note) => {
          const pending = noteFor;
          setNoteFor(null);
          if (pending) send({ ...pending.payload, notes: note });
        }}
      />
```

- Ubah teks bantuan `Foto selfie & lokasi GPS wajib disertakan` menjadi `Foto selfie wajib; aktifkan GPS agar lokasi bisa dicek (radius toko 100 m)`.
- Perbarui JSDoc komponen: "Di luar area: server menjawab 422, popup meminta keterangan, payload (termasuk selfie) dikirim ulang."

- [ ] **Step 3: Jalankan test, tsc, eslint**

Run: `cd frontend && npx vitest run src/lib/hris/geofence.test.ts src/features/hris/ess`
Expected: PASS.
Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/lib/hris/geofence.ts src/features/hris/ess`
Expected: bersih.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/hris/geofence.ts frontend/src/lib/hris/geofence.test.ts frontend/src/features/hris/ess
git commit -m "feat(hris-ui): popup ESS absen di luar area dengan keterangan wajib, badge geofence absen hari ini, EPIC-062"
```

---

### Task 7: Peta pemilih titik (OpenLayers + OSM) di form profil cabang

**Files:**
- Create: `frontend/src/lib/maps/coords.ts`
- Create: `frontend/src/lib/maps/coords.test.ts`
- Create: `frontend/src/components/maps/location-picker-map.tsx`
- Create: `frontend/src/components/maps/location-picker-field.tsx`
- Modify: `frontend/src/features/configuration/business/components/branch-profile-dialog.tsx`

**Interfaces:**
- Consumes: paket `ol` (^10.10.0, sudah di `package.json`); pola setup `BranchMap` (`frontend/src/features/site/components/branch-map.tsx`).
- Produces:
  - `interface LatLngPoint { lat: number; lng: number }`
  - `roundCoord(v: number): number` (6 desimal, sesuai `numeric(9,6)`)
  - `isUsableCoord(lat: number, lng: number): boolean`
  - `coordsFromInputs(lat: string, lng: string): LatLngPoint | null`
  - `parseLatLng(text: string): LatLngPoint | null` (format salinan Google Maps `-6.888900, 107.596000`)
  - `LocationPickerMap({ value, radiusM, onPick, height })` (client-only, impor `ol`)
  - `LocationPickerField({ lat, lng, radiusM, onPick, height })` (membungkus peta dengan `next/dynamic` `ssr: false`)

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/lib/maps/coords.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { coordsFromInputs, isUsableCoord, parseLatLng, roundCoord } from "./coords";

describe("coords", () => {
  it("membulatkan ke 6 desimal (numeric(9,6))", () => {
    expect(roundCoord(-6.88891234)).toBe(-6.888912);
    expect(roundCoord(107.5960004)).toBe(107.596);
  });
  it("menolak 0,0 dan koordinat di luar rentang", () => {
    expect(isUsableCoord(-6.9, 107.6)).toBe(true);
    expect(isUsableCoord(0, 0)).toBe(false);
    expect(isUsableCoord(-91, 107.6)).toBe(false);
    expect(isUsableCoord(-6.9, 181)).toBe(false);
  });
  it("membaca input form lat/lng", () => {
    expect(coordsFromInputs("-6.9", "107.6")).toEqual({ lat: -6.9, lng: 107.6 });
    expect(coordsFromInputs("", "107.6")).toBeNull();
    expect(coordsFromInputs("abc", "107.6")).toBeNull();
  });
  it("membaca salinan koordinat Google Maps", () => {
    expect(parseLatLng("-6.888900, 107.596000")).toEqual({ lat: -6.8889, lng: 107.596 });
    expect(parseLatLng(" -6.8889 107.596 ")).toEqual({ lat: -6.8889, lng: 107.596 });
    expect(parseLatLng("Jl. Sukajadi 131")).toBeNull();
    expect(parseLatLng("0, 0")).toBeNull();
  });
});
```

Run: `cd frontend && npx vitest run src/lib/maps/coords.test.ts`
Expected: FAIL (modul `./coords` tidak ada).

- [ ] **Step 2: Implementasi helper**

`frontend/src/lib/maps/coords.ts`:

```ts
/** Koordinat titik peta (derajat desimal WGS84). */
export interface LatLngPoint {
  lat: number;
  lng: number;
}

/** 6 desimal (±11 cm), presisi kolom configuration.branches.lat/lng numeric(9,6). */
export const roundCoord = (v: number): number => Math.round(v * 1e6) / 1e6;

/** Koordinat yang masuk akal: dalam rentang dan bukan 0,0 (GPS gagal). */
export function isUsableCoord(lat: number, lng: number): boolean {
  if (!Number.isFinite(lat) || !Number.isFinite(lng)) return false;
  if (Math.abs(lat) > 90 || Math.abs(lng) > 180) return false;
  return lat !== 0 || lng !== 0;
}

/** Titik dari dua input form; null bila kosong atau tidak valid. */
export function coordsFromInputs(lat: string, lng: string): LatLngPoint | null {
  if (lat.trim() === "" || lng.trim() === "") return null;
  const point = { lat: Number(lat), lng: Number(lng) };
  return isUsableCoord(point.lat, point.lng) ? point : null;
}

/** "-6.888900, 107.596000" (salin dari Google Maps) atau dipisah spasi; null bila bukan koordinat. */
export function parseLatLng(text: string): LatLngPoint | null {
  const m = text.trim().match(/^(-?\d+(?:\.\d+)?)\s*[,\s]\s*(-?\d+(?:\.\d+)?)$/);
  if (!m) return null;
  const point = { lat: roundCoord(Number(m[1])), lng: roundCoord(Number(m[2])) };
  return isUsableCoord(point.lat, point.lng) ? point : null;
}
```

Run: perintah Step 1. Expected: PASS.

- [ ] **Step 3: Komponen peta**

`frontend/src/components/maps/location-picker-map.tsx`:

```tsx
"use client";

import Feature from "ol/Feature";
import Map from "ol/Map";
import View from "ol/View";
import Point from "ol/geom/Point";
import { circular } from "ol/geom/Polygon";
import TileLayer from "ol/layer/Tile";
import VectorLayer from "ol/layer/Vector";
import "ol/ol.css";
import { fromLonLat, toLonLat } from "ol/proj";
import OSM from "ol/source/OSM";
import VectorSource from "ol/source/Vector";
import CircleStyle from "ol/style/Circle";
import Fill from "ol/style/Fill";
import Stroke from "ol/style/Stroke";
import Style from "ol/style/Style";
import { useEffect, useRef } from "react";
import { roundCoord, type LatLngPoint } from "@/lib/maps/coords";

const PIN = new Style({
  image: new CircleStyle({ radius: 8, fill: new Fill({ color: "#dc2626" }), stroke: new Stroke({ color: "#fff", width: 2.5 }) }),
});
const AREA = new Style({
  fill: new Fill({ color: "rgba(37, 99, 235, 0.12)" }),
  stroke: new Stroke({ color: "rgba(37, 99, 235, 0.6)", width: 1.5 }),
});
const BANDUNG: [number, number] = [107.6, -6.9];
const key = (p: LatLngPoint) => `${p.lat},${p.lng}`;

/**
 * Peta OpenLayers + OSM (stack yang sama dengan BranchMap situs dan GeoMap
 * member app) untuk memilih satu titik: klik peta memindahkan pin. radiusM
 * menggambar area absensi (lingkaran geodesik). Titik dari luar (input
 * diketik) membuat peta terbang ke titik itu; klik di peta tidak.
 */
export function LocationPickerMap({
  value,
  radiusM,
  onPick,
  height = 280,
}: {
  value: LatLngPoint | null;
  radiusM?: number | null;
  onPick: (point: LatLngPoint) => void;
  height?: number;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<Map | null>(null);
  const markRef = useRef<VectorSource | null>(null);
  const pickedRef = useRef<string | null>(null);
  const onPickRef = useRef(onPick);
  useEffect(() => {
    onPickRef.current = onPick;
  });

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const mark = new VectorSource();
    markRef.current = mark;
    const map = new Map({
      target: container,
      layers: [new TileLayer({ source: new OSM() }), new VectorLayer({ source: mark })],
      view: new View({ center: fromLonLat(BANDUNG), zoom: 11 }),
    });
    map.on("click", (evt) => {
      const [lng, lat] = toLonLat(evt.coordinate);
      const point = { lat: roundCoord(lat), lng: roundCoord(lng) };
      pickedRef.current = key(point);
      onPickRef.current(point);
    });
    mapRef.current = map;
    return () => {
      map.setTarget(undefined);
      mapRef.current = null;
      markRef.current = null;
    };
  }, []);

  const lat = value?.lat;
  const lng = value?.lng;
  useEffect(() => {
    const mark = markRef.current;
    const map = mapRef.current;
    if (!mark || !map) return;
    mark.clear();
    if (lat === undefined || lng === undefined) return;
    if (radiusM && radiusM > 0) {
      const area = new Feature(circular([lng, lat], radiusM, 64).transform("EPSG:4326", "EPSG:3857"));
      area.setStyle(AREA);
      mark.addFeature(area);
    }
    const pin = new Feature(new Point(fromLonLat([lng, lat])));
    pin.setStyle(PIN);
    mark.addFeature(pin);
    if (pickedRef.current !== key({ lat, lng })) {
      const view = map.getView();
      view.animate({ center: fromLonLat([lng, lat]), zoom: Math.max(view.getZoom() ?? 11, 17), duration: 300 });
    }
    pickedRef.current = null;
  }, [lat, lng, radiusM]);

  return (
    <div className="overflow-hidden rounded-lg border border-gray-200" style={{ height }}>
      <div ref={containerRef} className="h-full w-full cursor-crosshair" role="application" aria-label="Peta pilih lokasi toko" />
    </div>
  );
}
```

`frontend/src/components/maps/location-picker-field.tsx`:

```tsx
"use client";

import dynamic from "next/dynamic";
import { coordsFromInputs, type LatLngPoint } from "@/lib/maps/coords";

// OpenLayers butuh window: muat hanya di browser, di bundle terpisah.
const LocationPickerMap = dynamic(() => import("./location-picker-map").then((m) => m.LocationPickerMap), {
  ssr: false,
  loading: () => <div className="h-[280px] animate-pulse rounded-lg bg-gray-100" />,
});

/**
 * Peta pemilih titik untuk form yang menyimpan lat/lng sebagai teks: klik peta
 * mengisi input; mengetik di input memindahkan pin.
 */
export function LocationPickerField({
  lat,
  lng,
  radiusM,
  onPick,
  height,
}: {
  lat: string;
  lng: string;
  radiusM?: number | null;
  onPick: (point: LatLngPoint) => void;
  height?: number;
}) {
  return (
    <div className="space-y-1">
      <LocationPickerMap value={coordsFromInputs(lat, lng)} radiusM={radiusM} onPick={onPick} height={height} />
      <p className="text-xs text-gray-500">
        Klik peta untuk menaruh titik toko. Latitude/longitude tetap bisa diketik atau ditempel dari Google Maps.
      </p>
    </div>
  );
}
```

- [ ] **Step 4: Pasang di form profil cabang**

`branch-profile-dialog.tsx`: impor `import { LocationPickerField } from "@/components/maps/location-picker-field";`. Tepat setelah `</div>` penutup grid yang memuat input Latitude/Longitude (sebelum `<LabeledField label="Gambar hero">`), tambahkan:

```tsx
        <LabeledField label="Titik di peta" hint="Dipakai peta situs dan geofence absensi (radius 100 m)">
          <LocationPickerField
            lat={form.lat}
            lng={form.lng}
            onPick={(p) => {
              set("lat", String(p.lat));
              set("lng", String(p.lng));
            }}
          />
        </LabeledField>
```

Bila `LabeledField` tidak menerima `hint`, cek props-nya di `frontend/src/features/site/admin/fields.tsx` (Slug memakai `hint`, jadi seharusnya ada).

- [ ] **Step 5: tsc, eslint, test**

Run: `cd frontend && npx vitest run src/lib/maps && npx tsc --noEmit -p . && npx eslint src/lib/maps src/components/maps src/features/configuration/business/components/branch-profile-dialog.tsx`
Expected: test PASS, tsc dan eslint bersih. (Peta sendiri diperiksa manual di Task 10, karena OpenLayers tidak berjalan di jsdom.)

- [ ] **Step 6: Commit**

```bash
git add frontend/src/lib/maps frontend/src/components/maps frontend/src/features/configuration/business/components/branch-profile-dialog.tsx
git commit -m "feat(ui): peta OpenLayers/OSM pemilih titik toko di form profil cabang, EPIC-062"
```

---

### Task 8: Dialog "Lokasi Toko" untuk HR di Penempatan Karyawan

**Files:**
- Modify: `frontend/src/lib/hris/geofence.ts` (`GEOFENCE_DEFAULT_RADIUS_M`, `BranchLocationForm`, `validateBranchLocation`, `toBranchLocationPayload`)
- Modify: `frontend/src/lib/hris/geofence.test.ts`
- Modify: `frontend/src/features/hris/placements/types.ts`, `api.ts`, `mutations.ts`
- Create: `frontend/src/features/hris/placements/components/branch-location-dialog.tsx`
- Create: `frontend/src/features/hris/placements/components/branch-location-dialog.test.tsx`
- Modify: `frontend/src/features/hris/placements/components/placements-page.tsx`

**Interfaces:**
- Consumes: `PUT /api/hris/branches/{id}/location` dan `geofence_radius_m` di `GET /api/hris/branches` (Task 3); `LocationPickerField`, `parseLatLng` (Task 7).
- Produces:
  - `const GEOFENCE_DEFAULT_RADIUS_M = 100`
  - `interface BranchLocationForm { lat: string; lng: string; radius: string }`
  - `validateBranchLocation(form: BranchLocationForm): string` ("" bila valid; pesan sama dengan server)
  - `toBranchLocationPayload(form: BranchLocationForm): BranchLocationPayload`
  - `interface BranchLocationPayload { lat: number | null; lng: number | null; geofence_radius_m: number | null }`
  - `saveBranchLocation(id: string, payload: BranchLocationPayload)`, `useSaveBranchLocation()`
  - `BranchLocationDialog({ open, onOpenChange, branches })`

- [ ] **Step 1: Tulis test yang gagal**

Tambahkan ke `frontend/src/lib/hris/geofence.test.ts`:

```ts
import { toBranchLocationPayload, validateBranchLocation } from "./geofence";

describe("validateBranchLocation", () => {
  it("sama dengan aturan server", () => {
    expect(validateBranchLocation({ lat: "", lng: "", radius: "" })).toBe("");
    expect(validateBranchLocation({ lat: "-6.8889", lng: "107.596", radius: "250" })).toBe("");
    expect(validateBranchLocation({ lat: "-6.8889", lng: "", radius: "" })).toBe(
      "Latitude dan longitude wajib diisi berdua atau dikosongkan berdua"
    );
    expect(validateBranchLocation({ lat: "0", lng: "0", radius: "" })).toBe(
      "Koordinat tidak valid (latitude -90..90, longitude -180..180, bukan 0,0)"
    );
    expect(validateBranchLocation({ lat: "-6.9", lng: "107.6", radius: "10" })).toBe("Radius absensi 25–1000 meter");
    expect(validateBranchLocation({ lat: "-6.9", lng: "107.6", radius: "120.5" })).toBe("Radius absensi 25–1000 meter");
  });
  it("membentuk payload API", () => {
    expect(toBranchLocationPayload({ lat: "-6.8889", lng: "107.596", radius: "" })).toEqual({
      lat: -6.8889,
      lng: 107.596,
      geofence_radius_m: null,
    });
    expect(toBranchLocationPayload({ lat: "", lng: "", radius: "250" })).toEqual({
      lat: null,
      lng: null,
      geofence_radius_m: 250,
    });
  });
});
```

(Gabungkan import dengan import yang sudah ada di atas file.)

`frontend/src/features/hris/placements/components/branch-location-dialog.test.tsx`:

```tsx
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StoreBranch } from "../types";
import { BranchLocationDialog } from "./branch-location-dialog";

const mutate = vi.fn();
vi.mock("../mutations", () => ({ useSaveBranchLocation: () => ({ mutate, isPending: false }) }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/components/maps/location-picker-field", () => ({
  LocationPickerField: ({ onPick }: { onPick: (p: { lat: number; lng: number }) => void }) => (
    <button type="button" onClick={() => onPick({ lat: -6.8889, lng: 107.596 })}>
      pilih titik
    </button>
  ),
}));

const branches: StoreBranch[] = [
  { id: "b1", code: "PVJ", name: "Paddy PVJ", company_id: "c1", company_name: "Paddy", lat: null, lng: null, geofence_radius_m: null },
  { id: "b2", code: "KPO", name: "Paddy Kopo", company_id: "c1", company_name: "Paddy", lat: "-6.946000", lng: "107.592000", geofence_radius_m: 200 },
];

afterEach(() => {
  cleanup();
  mutate.mockReset();
});

describe("BranchLocationDialog", () => {
  it("menghitung toko tanpa koordinat dan menyimpan titik dari peta", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    expect(screen.getByText("1 dari 2 toko belum punya koordinat")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Paddy PVJ/ }));
    fireEvent.click(screen.getByRole("button", { name: "pilih titik" }));
    expect((screen.getByLabelText("Latitude") as HTMLInputElement).value).toBe("-6.8889");
    fireEvent.click(screen.getByRole("button", { name: "Simpan lokasi" }));
    expect(mutate).toHaveBeenCalledWith(
      { id: "b1", payload: { lat: -6.8889, lng: 107.596, geofence_radius_m: null } },
      expect.anything()
    );
  });

  it("menempel koordinat Google Maps dan menolak radius di luar batas", () => {
    render(<BranchLocationDialog open onOpenChange={() => {}} branches={branches} />);
    fireEvent.click(screen.getByRole("button", { name: /Paddy Kopo/ }));
    expect((screen.getByLabelText("Radius (meter)") as HTMLInputElement).value).toBe("200");
    fireEvent.change(screen.getByLabelText("Tempel dari Google Maps"), { target: { value: "-6.9, 107.61" } });
    expect((screen.getByLabelText("Longitude") as HTMLInputElement).value).toBe("107.61");
    fireEvent.change(screen.getByLabelText("Radius (meter)"), { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan lokasi" }));
    expect(mutate).not.toHaveBeenCalled();
  });
});
```

Run: `cd frontend && npx vitest run src/lib/hris/geofence.test.ts src/features/hris/placements/components/branch-location-dialog.test.tsx`
Expected: FAIL (`validateBranchLocation` dan `./branch-location-dialog` tidak ada).

- [ ] **Step 2: Implementasi**

`frontend/src/lib/hris/geofence.ts`, tambahkan (baris `import` diletakkan di atas file):

```ts
import { isUsableCoord } from "@/lib/maps/coords";

/** Radius MOM AB-01 bila toko tidak punya override. */
export const GEOFENCE_DEFAULT_RADIUS_M = 100;

export interface BranchLocationForm {
  lat: string;
  lng: string;
  /** Override radius; kosong = 100 m */
  radius: string;
}

/** Validasi form Lokasi Toko; pesan sama dengan server (EPIC-062 R9). "" bila valid. */
export function validateBranchLocation(form: BranchLocationForm): string {
  const hasLat = form.lat.trim() !== "";
  const hasLng = form.lng.trim() !== "";
  if (hasLat !== hasLng) return "Latitude dan longitude wajib diisi berdua atau dikosongkan berdua";
  if (hasLat && !isUsableCoord(Number(form.lat), Number(form.lng))) {
    return "Koordinat tidak valid (latitude -90..90, longitude -180..180, bukan 0,0)";
  }
  if (form.radius.trim() !== "") {
    const r = Number(form.radius);
    if (!Number.isInteger(r) || r < 25 || r > 1000) return "Radius absensi 25–1000 meter";
  }
  return "";
}

export function toBranchLocationPayload(form: BranchLocationForm) {
  const num = (v: string) => (v.trim() === "" ? null : Number(v));
  return { lat: num(form.lat), lng: num(form.lng), geofence_radius_m: num(form.radius) };
}
```

`placements/types.ts`: di `StoreBranch` setelah `lng`:

```ts
  /** Override radius absensi (meter); null = 100 m */
  geofence_radius_m: number | null;
```

dan tambahkan:

```ts
/** PUT /api/hris/branches/{id}/location (EPIC-062). */
export interface BranchLocationPayload {
  lat: number | null;
  lng: number | null;
  geofence_radius_m: number | null;
}
```

Di `ClockPlacement` tambahkan `geofence_radius_m?: number | null;`.

`placements/api.ts`: impor `apiPut` dan `BranchLocationPayload`, tambahkan:

```ts
export const saveBranchLocation = (id: string, payload: BranchLocationPayload) =>
  apiPut<{ data: StoreBranch }>(`/api/hris/branches/${id}/location`, payload);
```

`placements/mutations.ts`:

```ts
export function useSaveBranchLocation() {
  const onSuccess = useInvalidatePlacements();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: BranchLocationPayload }) => saveBranchLocation(id, payload),
    onSuccess,
  });
}
```

(impor `saveBranchLocation` dan tipe `BranchLocationPayload`.)

`frontend/src/features/hris/placements/components/branch-location-dialog.tsx`:

```tsx
"use client";

import { useState } from "react";
import { toast } from "sonner";
import { MapPin } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { LocationPickerField } from "@/components/maps/location-picker-field";
import { parseLatLng } from "@/lib/maps/coords";
import {
  GEOFENCE_DEFAULT_RADIUS_M,
  toBranchLocationPayload,
  validateBranchLocation,
  type BranchLocationForm,
} from "@/lib/hris/geofence";
import { useSaveBranchLocation } from "../mutations";
import type { StoreBranch } from "../types";

const EMPTY: BranchLocationForm = { lat: "", lng: "", radius: "" };
const toForm = (b: StoreBranch): BranchLocationForm => ({
  lat: b.lat === null ? "" : String(Number(b.lat)),
  lng: b.lng === null ? "" : String(Number(b.lng)),
  radius: b.geofence_radius_m == null ? "" : String(b.geofence_radius_m),
});

/**
 * HRIS → Penempatan Karyawan → Lokasi Toko (EPIC-062, K-14): HR mengisi
 * titik toko lewat peta (OpenLayers + OSM) atau tempel koordinat, plus
 * override radius absensi. Toko tanpa koordinat membuat absen "Belum dicek".
 */
export function BranchLocationDialog({
  open,
  onOpenChange,
  branches,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  branches: StoreBranch[];
}) {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [form, setForm] = useState<BranchLocationForm>(EMPTY);
  const [paste, setPaste] = useState("");
  const save = useSaveBranchLocation();
  const missing = branches.filter((b) => b.lat === null || b.lng === null).length;
  const selected = branches.find((b) => b.id === selectedId) ?? null;
  const radius = Number(form.radius) || GEOFENCE_DEFAULT_RADIUS_M;

  function choose(b: StoreBranch) {
    setSelectedId(b.id);
    setForm(toForm(b));
    setPaste("");
  }

  function applyPaste(text: string) {
    setPaste(text);
    const point = parseLatLng(text);
    if (point) setForm((f) => ({ ...f, lat: String(point.lat), lng: String(point.lng) }));
  }

  function submit() {
    if (!selected) return;
    const error = validateBranchLocation(form);
    if (error) {
      toast.error(error);
      return;
    }
    save.mutate(
      { id: selected.id, payload: toBranchLocationPayload(form) },
      {
        onSuccess: () => toast.success(`Lokasi ${selected.name} disimpan`),
        onError: (e) => toast.error(e.message || "Gagal menyimpan lokasi"),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>Lokasi Toko</DialogTitle>
          <DialogDescription>
            {missing} dari {branches.length} toko belum punya koordinat
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 md:grid-cols-[14rem_1fr]">
          <ul className="max-h-[420px] space-y-1 overflow-y-auto pr-1">
            {branches.map((b) => (
              <li key={b.id}>
                <button
                  type="button"
                  onClick={() => choose(b)}
                  className={`flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-sm ${
                    b.id === selectedId ? "bg-blue-50 text-blue-700" : "hover:bg-gray-50"
                  }`}
                >
                  <span className="truncate">{b.name}</span>
                  {b.lat === null || b.lng === null ? (
                    <span className="ml-2 shrink-0 text-xs text-amber-600">kosong</span>
                  ) : (
                    <MapPin className="ml-2 h-3.5 w-3.5 shrink-0 text-green-600" />
                  )}
                </button>
              </li>
            ))}
          </ul>
          {selected ? (
            <div className="space-y-3">
              <LocationPickerField
                lat={form.lat}
                lng={form.lng}
                radiusM={radius}
                onPick={(p) => setForm((f) => ({ ...f, lat: String(p.lat), lng: String(p.lng) }))}
              />
              <div className="grid gap-3 sm:grid-cols-3">
                <label className="text-xs font-medium text-gray-600">
                  Latitude
                  <Input type="number" step="any" value={form.lat} onChange={(e) => setForm((f) => ({ ...f, lat: e.target.value }))} />
                </label>
                <label className="text-xs font-medium text-gray-600">
                  Longitude
                  <Input type="number" step="any" value={form.lng} onChange={(e) => setForm((f) => ({ ...f, lng: e.target.value }))} />
                </label>
                <label className="text-xs font-medium text-gray-600">
                  Radius (meter)
                  <Input
                    type="number"
                    min={25}
                    max={1000}
                    placeholder={String(GEOFENCE_DEFAULT_RADIUS_M)}
                    value={form.radius}
                    onChange={(e) => setForm((f) => ({ ...f, radius: e.target.value }))}
                  />
                </label>
              </div>
              <label className="block text-xs font-medium text-gray-600">
                Tempel dari Google Maps
                <Input value={paste} placeholder="-6.888900, 107.596000" onChange={(e) => applyPaste(e.target.value)} />
              </label>
              <p className="text-xs text-gray-500">
                Radius kosong = {GEOFENCE_DEFAULT_RADIUS_M} m. Koordinat yang sama dipakai peta lokasi di situs publik.
              </p>
            </div>
          ) : (
            <p className="self-center text-sm text-gray-500">Pilih toko di kiri untuk mengisi lokasinya.</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Tutup
          </Button>
          <Button disabled={!selected || save.isPending} onClick={submit}>
            Simpan lokasi
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

`placements-page.tsx`:
- Impor `MapPin` dari `lucide-react` dan `BranchLocationDialog` dari `./branch-location-dialog`.
- State `const [locationOpen, setLocationOpen] = useState(false);` dan `const missingCoords = (branches.data ?? []).filter((b) => b.lat === null || b.lng === null).length;`
- Ganti prop `actions` `PageHeader`:

```tsx
        actions={
          canManage ? (
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => setLocationOpen(true)}>
                <MapPin className="mr-1.5 h-4 w-4" /> Lokasi Toko
                {missingCoords > 0 ? ` (${missingCoords} belum diisi)` : ""}
              </Button>
              {selected.size > 0 && (
                <Button onClick={() => setBulkOpen(true)}>
                  <Store className="mr-1.5 h-4 w-4" /> Tempatkan {selected.size} karyawan…
                </Button>
              )}
            </div>
          ) : null
        }
```

- Sebelum penutup `</div>` terluar, tambahkan `<BranchLocationDialog open={locationOpen} onOpenChange={setLocationOpen} branches={branches.data ?? []} />`.

- [ ] **Step 3: Jalankan test, tsc, eslint**

Run: `cd frontend && npx vitest run src/lib/hris/geofence.test.ts src/features/hris/placements`
Expected: PASS.
Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/lib/hris/geofence.ts src/features/hris/placements`
Expected: bersih. Perbaiki fixture `StoreBranch` di test lama yang belum memuat `geofence_radius_m` (tambahkan `geofence_radius_m: null`).

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/hris/geofence.ts frontend/src/lib/hris/geofence.test.ts frontend/src/features/hris/placements
git commit -m "feat(hris-ui): dialog Lokasi Toko di Penempatan Karyawan (peta, tempel koordinat, radius), EPIC-062"
```

---

### Task 9: UI Perjalanan Dinas (ESS + tab HR)

**Files:**
- Create: `frontend/src/lib/hris/business-trips.ts`
- Create: `frontend/src/lib/hris/business-trips.test.ts`
- Create: `frontend/src/features/hris/business-trips/types.ts`, `api.ts`, `query-keys.ts`, `queries.ts`, `mutations.ts`, `index.ts`
- Create: `frontend/src/features/hris/business-trips/components/trip-request-dialog.tsx`
- Create: `frontend/src/features/hris/business-trips/components/trip-status-badge.tsx`
- Create: `frontend/src/features/hris/business-trips/components/perjalanan-dinas-page.tsx`
- Create: `frontend/src/features/hris/business-trips/components/business-trips-tab.tsx`
- Create: `frontend/src/app/dashboard/(dashboard)/me/perjalanan-dinas/page.tsx`
- Modify: `frontend/src/features/hris/attendance/components/attendance-page.tsx`

**Interfaces:**
- Consumes: route `/api/hris/business-trips` (Task 4); `useActiveEmployees` (lama, `features/hris/attendance/queries.ts`); `shortDateId` (lama, `lib/hris/placements.ts`); `todayWib` (lama, `lib/dates.ts`).
- Produces:
  - `type TripStatus`, `TRIP_STATUS_META`, `interface TripForm`, `EMPTY_TRIP_FORM`, `tripDays(start, end): number`, `validateTripForm(form, today): string`, `tripRangeLabel(start, end): string`.
  - Hooks `useBusinessTrips(filter)`, `useCreateBusinessTrip()`, `useDecideBusinessTrip()`.
  - Halaman ESS `/dashboard/me/perjalanan-dinas` (menu `ess.business-trip` dari Task 4) dan tab HR "Perjalanan Dinas" di halaman Absensi.

- [ ] **Step 1: Tulis test yang gagal**

`frontend/src/lib/hris/business-trips.test.ts`:

```ts
import { describe, expect, it } from "vitest";
import { EMPTY_TRIP_FORM, TRIP_STATUS_META, tripDays, tripRangeLabel, validateTripForm } from "./business-trips";

const form = { start_date: "2026-10-05", end_date: "2026-10-07", destination: "Gudang Cikarang", purpose: "Stock opname" };

describe("Perjalanan Dinas", () => {
  it("menghitung hari inklusif", () => {
    expect(tripDays("2026-10-05", "2026-10-07")).toBe(3);
    expect(tripDays("2026-10-05", "2026-10-05")).toBe(1);
    expect(tripDays("2026-10-07", "2026-10-05")).toBe(0);
    expect(tripDays("", "2026-10-05")).toBe(0);
  });
  it("memvalidasi seperti server", () => {
    const today = "2026-10-05";
    expect(validateTripForm(form, today)).toBe("");
    expect(validateTripForm(EMPTY_TRIP_FORM, today)).toBe("Tanggal mulai dan selesai wajib diisi");
    expect(validateTripForm({ ...form, start_date: "2026-10-04" }, today)).toBe("Tanggal mulai tidak boleh sebelum hari ini");
    expect(validateTripForm({ ...form, end_date: "2026-10-04", start_date: "2026-10-06" }, today)).toBe(
      "Tanggal selesai tidak boleh sebelum tanggal mulai"
    );
    expect(validateTripForm({ ...form, end_date: "2026-11-05" }, today)).toBe("Perjalanan Dinas maksimal 31 hari per pengajuan");
    expect(validateTripForm({ ...form, destination: " ab " }, today)).toBe("Tujuan minimal 3 karakter");
    expect(validateTripForm({ ...form, purpose: "abcd" }, today)).toBe("Keperluan minimal 5 karakter");
  });
  it("label rentang dan status", () => {
    expect(tripRangeLabel("2026-10-05", "2026-10-07")).toBe("5 Okt 2026 – 7 Okt 2026 (3 hari)");
    expect(tripRangeLabel("2026-10-05", "2026-10-05")).toBe("5 Okt 2026 (1 hari)");
    expect(TRIP_STATUS_META.approved.label).toBe("Disetujui");
  });
});
```

Run: `cd frontend && npx vitest run src/lib/hris/business-trips.test.ts`
Expected: FAIL (modul tidak ada).

- [ ] **Step 2: Implementasi helper**

`frontend/src/lib/hris/business-trips.ts`:

```ts
import { shortDateId } from "./placements";

/** Perjalanan Dinas (EPIC-062, MOM AB-02): label & validasi form, sama dengan server. */

export type TripStatus = "pending" | "approved" | "rejected" | "cancelled";

export const TRIP_STATUS_META: Record<TripStatus, { label: string; className: string }> = {
  pending: { label: "Menunggu", className: "bg-amber-100 text-amber-700" },
  approved: { label: "Disetujui", className: "bg-green-100 text-green-700" },
  rejected: { label: "Ditolak", className: "bg-red-100 text-red-700" },
  cancelled: { label: "Dibatalkan", className: "bg-gray-100 text-gray-600" },
};

export const TRIP_MAX_DAYS = 31;

export interface TripForm {
  start_date: string;
  end_date: string;
  destination: string;
  purpose: string;
}

export const EMPTY_TRIP_FORM: TripForm = { start_date: "", end_date: "", destination: "", purpose: "" };

/** Jumlah hari inklusif; 0 bila tanggal kosong/tidak valid atau selesai sebelum mulai. */
export function tripDays(start: string, end: string): number {
  const s = Date.parse(`${start}T00:00:00Z`);
  const e = Date.parse(`${end}T00:00:00Z`);
  if (Number.isNaN(s) || Number.isNaN(e) || e < s) return 0;
  return Math.round((e - s) / 86_400_000) + 1;
}

/** "" bila valid; pesan sama dengan server (EPIC-062 R7). */
export function validateTripForm(form: TripForm, today: string): string {
  if (!form.start_date || !form.end_date) return "Tanggal mulai dan selesai wajib diisi";
  if (form.start_date < today) return "Tanggal mulai tidak boleh sebelum hari ini";
  if (form.end_date < form.start_date) return "Tanggal selesai tidak boleh sebelum tanggal mulai";
  if (tripDays(form.start_date, form.end_date) > TRIP_MAX_DAYS) return "Perjalanan Dinas maksimal 31 hari per pengajuan";
  if (form.destination.trim().length < 3) return "Tujuan minimal 3 karakter";
  if (form.purpose.trim().length < 5) return "Keperluan minimal 5 karakter";
  return "";
}

/** "5 Okt 2026 – 7 Okt 2026 (3 hari)". */
export function tripRangeLabel(start: string, end: string): string {
  const days = tripDays(start, end);
  const range = start === end ? shortDateId(start) : `${shortDateId(start)} – ${shortDateId(end)}`;
  return `${range} (${days} hari)`;
}
```

Run: perintah Step 1. Expected: PASS.

- [ ] **Step 3: Feature `business-trips`**

`frontend/src/features/hris/business-trips/types.ts`:

```ts
import type { TripForm, TripStatus } from "@/lib/hris/business-trips";

/** Baris hris.business_trips (GET /api/hris/business-trips). */
export interface BusinessTrip {
  id: string;
  employee_id: string;
  start_date: string; // YYYY-MM-DD
  end_date: string;
  destination: string;
  purpose: string;
  status: TripStatus;
  requested_by: string | null;
  decided_by: string | null;
  decided_by_name: string | null;
  decided_at: string | null;
  rejection_reason: string | null;
  created_at: string;
  employee: { id: string; full_name: string; nip: string | null; reporting_to: string | null } | null;
}

export type TripAction = "approve" | "reject" | "cancel";

export interface TripPayload extends TripForm {
  /** HRD mencatat untuk karyawan lain (langsung disetujui) */
  employee_id?: string;
}

export interface TripListFilter {
  scope?: "approvals";
  status?: TripStatus;
  employee_id?: string;
}
```

`api.ts`:

```ts
import { apiGet, apiPost, buildListUrl } from "@/lib/api-client";
import type { BusinessTrip, TripAction, TripListFilter, TripPayload } from "./types";

export const fetchBusinessTrips = (filter: TripListFilter = {}) =>
  apiGet<{ data: BusinessTrip[] }>(
    buildListUrl("/api/hris/business-trips", { scope: filter.scope, status: filter.status, employee_id: filter.employee_id })
  ).then((res) => res.data ?? []);

export const createBusinessTrip = (payload: TripPayload) =>
  apiPost<{ data: BusinessTrip; message?: string }>("/api/hris/business-trips", payload);

export const decideBusinessTrip = (id: string, action: TripAction, rejectionReason?: string) =>
  apiPost<{ data: BusinessTrip; message?: string }>(`/api/hris/business-trips/${id}/decide`, {
    action,
    rejection_reason: rejectionReason,
  });
```

`query-keys.ts`:

```ts
import type { TripListFilter } from "./types";

export const businessTripQueryKeys = {
  all: ["hris", "business-trips"] as const,
  list: (filter: TripListFilter) => ["hris", "business-trips", "list", filter] as const,
};
```

`queries.ts`:

```ts
"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchBusinessTrips } from "./api";
import { businessTripQueryKeys } from "./query-keys";
import type { TripListFilter } from "./types";

export const useBusinessTrips = (filter: TripListFilter = {}) =>
  useQuery({ queryKey: businessTripQueryKeys.list(filter), queryFn: () => fetchBusinessTrips(filter) });
```

`mutations.ts`:

```ts
"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import { createBusinessTrip, decideBusinessTrip } from "./api";
import { businessTripQueryKeys } from "./query-keys";
import type { TripAction, TripPayload } from "./types";

// Persetujuan bisa mengubah status geofence absensi (R8): segarkan absensi juga.
function useInvalidateTrips() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: businessTripQueryKeys.all });
    void qc.invalidateQueries({ queryKey: attendanceQueryKeys.all });
  };
}

export function useCreateBusinessTrip() {
  const onSuccess = useInvalidateTrips();
  return useMutation({ mutationFn: (payload: TripPayload) => createBusinessTrip(payload), retry: false, onSuccess });
}

export function useDecideBusinessTrip() {
  const onSuccess = useInvalidateTrips();
  return useMutation({
    mutationFn: (vars: { id: string; action: TripAction; rejectionReason?: string }) =>
      decideBusinessTrip(vars.id, vars.action, vars.rejectionReason),
    retry: false,
    onSuccess,
  });
}
```

Cek `attendanceQueryKeys.all` ada di `features/hris/attendance/query-keys.ts`; bila namanya lain, pakai kunci akar yang dipakai `attendanceQueryKeys.calendarAll()` dan `list`.

`index.ts`:

```ts
export * from "./types";
export * from "./api";
export * from "./query-keys";
export * from "./queries";
export * from "./mutations";
export { EssPerjalananDinasPage } from "./components/perjalanan-dinas-page";
export { BusinessTripsTab } from "./components/business-trips-tab";
```

`components/trip-status-badge.tsx`:

```tsx
import { Badge } from "@/components/ui/badge";
import { TRIP_STATUS_META, type TripStatus } from "@/lib/hris/business-trips";

export function TripStatusBadge({ status }: { status: TripStatus }) {
  const meta = TRIP_STATUS_META[status];
  return <Badge className={meta.className}>{meta.label}</Badge>;
}
```

`components/trip-request-dialog.tsx`:

```tsx
"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { todayWib } from "@/lib/dates";
import { EMPTY_TRIP_FORM, tripDays, validateTripForm, type TripForm } from "@/lib/hris/business-trips";
import { useCreateBusinessTrip } from "../mutations";

/**
 * Form Perjalanan Dinas. Tanpa `employees`: pengajuan karyawan sendiri
 * (menunggu atasan). Dengan `employees` (HRD): pilih karyawan, langsung disetujui.
 */
export function TripRequestDialog({
  open,
  onOpenChange,
  employees,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  employees?: { value: string; label: string }[];
}) {
  const [form, setForm] = useState<TripForm>(EMPTY_TRIP_FORM);
  const [employeeId, setEmployeeId] = useState("");
  const create = useCreateBusinessTrip();
  const set = <K extends keyof TripForm>(k: K, v: TripForm[K]) => setForm((f) => ({ ...f, [k]: v }));
  const days = tripDays(form.start_date, form.end_date);

  function submit() {
    if (employees && !employeeId) {
      toast.error("Pilih karyawan");
      return;
    }
    const error = validateTripForm(form, todayWib());
    if (error) {
      toast.error(error);
      return;
    }
    create.mutate(
      { ...form, destination: form.destination.trim(), purpose: form.purpose.trim(), ...(employees ? { employee_id: employeeId } : {}) },
      {
        onSuccess: (json) => {
          toast.success(json.message || "Perjalanan Dinas tersimpan");
          setForm(EMPTY_TRIP_FORM);
          setEmployeeId("");
          onOpenChange(false);
        },
        onError: (e) => toast.error(e.message || "Gagal menyimpan Perjalanan Dinas"),
      }
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{employees ? "Catat Perjalanan Dinas" : "Ajukan Perjalanan Dinas"}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          {employees && (
            <Combobox
              options={employees}
              value={employeeId}
              onChange={setEmployeeId}
              placeholder="Pilih karyawan"
              searchPlaceholder="Cari nama/NIP…"
              emptyMessage="Karyawan tidak ditemukan"
              className="!w-full"
            />
          )}
          <div className="grid grid-cols-2 gap-3">
            <label className="text-xs font-medium text-gray-600">
              Tanggal mulai
              <Input type="date" min={todayWib()} value={form.start_date} onChange={(e) => set("start_date", e.target.value)} />
            </label>
            <label className="text-xs font-medium text-gray-600">
              Tanggal selesai
              <Input type="date" min={form.start_date || todayWib()} value={form.end_date} onChange={(e) => set("end_date", e.target.value)} />
            </label>
          </div>
          {days > 0 && <p className="text-xs text-gray-500">{days} hari · absen di tanggal ini tidak dicek radius toko</p>}
          <label className="block text-xs font-medium text-gray-600">
            Tujuan
            <Input value={form.destination} maxLength={200} placeholder="Mis. Gudang Cikarang" onChange={(e) => set("destination", e.target.value)} />
          </label>
          <label className="block text-xs font-medium text-gray-600">
            Keperluan
            <Textarea value={form.purpose} maxLength={1000} rows={3} onChange={(e) => set("purpose", e.target.value)} />
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Batal
          </Button>
          <Button disabled={create.isPending} onClick={submit}>
            {employees ? "Simpan" : "Kirim pengajuan"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

`components/perjalanan-dinas-page.tsx`:

```tsx
"use client";

import { useState } from "react";
import { toast } from "sonner";
import { BriefcaseIcon, UserGroupIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { tripRangeLabel } from "@/lib/hris/business-trips";
import { useEssMe } from "@/features/hris/ess/queries";
import { EssLoading, EssNotLinked } from "@/features/hris/ess/components/ess-states";
import { useBusinessTrips } from "../queries";
import { useDecideBusinessTrip } from "../mutations";
import type { BusinessTrip, TripAction } from "../types";
import { TripRequestDialog } from "./trip-request-dialog";
import { TripStatusBadge } from "./trip-status-badge";

/**
 * ESS → Perjalanan Dinas (/dashboard/me/perjalanan-dinas, EPIC-062): ajukan
 * rentang tanggal dinas; selama disetujui, absen tidak dicek radius toko.
 * Atasan langsung melihat antrean persetujuan timnya di halaman yang sama.
 */
export function EssPerjalananDinasPage() {
  const { data: me, isLoading } = useEssMe();
  const mine = useBusinessTrips({ employee_id: "me" }).data ?? [];
  const team = useBusinessTrips({ scope: "approvals", status: "pending" }).data ?? [];
  const decide = useDecideBusinessTrip();
  const [dialog, setDialog] = useState(false);
  const busyId = decide.isPending ? decide.variables?.id : null;

  function act(trip: BusinessTrip, action: TripAction) {
    let rejectionReason: string | undefined;
    if (action === "reject") {
      rejectionReason = window.prompt("Alasan menolak Perjalanan Dinas ini?") ?? undefined;
      if (!rejectionReason?.trim()) return;
    }
    decide.mutate(
      { id: trip.id, action, rejectionReason },
      {
        onSuccess: (json) => toast.success(json.message || "Berhasil diproses"),
        onError: (e) => toast.error(e.message || "Gagal memproses"),
      }
    );
  }

  if (isLoading) return <EssLoading />;
  if (!me?.employee) return <EssNotLinked feature="Perjalanan Dinas" />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between border-b border-gray-200/70 pb-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Perjalanan Dinas</h1>
          <p className="text-sm text-gray-500">Selama dinas disetujui, absen tidak dicek radius toko</p>
        </div>
        <Button className="gap-2" onClick={() => setDialog(true)}>
          <BriefcaseIcon className="h-4 w-4" /> Ajukan
        </Button>
      </div>

      {team.length > 0 && (
        <div className="rounded-xl border border-sky-200 bg-sky-50/70 p-5 shadow-sm">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-sky-800">
            <UserGroupIcon className="h-4 w-4" /> Persetujuan Perjalanan Dinas Tim
          </h3>
          <ul className="mt-3 divide-y divide-sky-100">
            {team.map((trip) => (
              <li key={trip.id} className="flex flex-wrap items-center justify-between gap-3 py-2.5">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">
                    {trip.employee?.full_name ?? "—"} · {tripRangeLabel(trip.start_date, trip.end_date)}
                  </p>
                  <p className="truncate text-xs text-gray-600">
                    {trip.destination} — {trip.purpose}
                  </p>
                </div>
                <div className="flex shrink-0 gap-2">
                  <Button size="sm" disabled={busyId === trip.id} onClick={() => act(trip, "approve")}>
                    Setujui
                  </Button>
                  <Button size="sm" variant="outline" disabled={busyId === trip.id} onClick={() => act(trip, "reject")}>
                    Tolak
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="rounded-xl border border-gray-200/70 bg-white shadow-sm">
        {mine.length === 0 ? (
          <p className="p-6 text-center text-sm text-gray-500">Belum ada pengajuan Perjalanan Dinas.</p>
        ) : (
          <ul className="divide-y divide-gray-100">
            {mine.map((trip) => (
              <li key={trip.id} className="flex flex-wrap items-center justify-between gap-3 p-4">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">{tripRangeLabel(trip.start_date, trip.end_date)}</p>
                  <p className="truncate text-xs text-gray-600">
                    {trip.destination} — {trip.purpose}
                  </p>
                  {trip.rejection_reason && <p className="text-xs text-red-600">Ditolak: {trip.rejection_reason}</p>}
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <TripStatusBadge status={trip.status} />
                  {trip.status === "pending" && (
                    <Button size="sm" variant="ghost" disabled={busyId === trip.id} onClick={() => act(trip, "cancel")}>
                      Batalkan
                    </Button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      <TripRequestDialog open={dialog} onOpenChange={setDialog} />
    </div>
  );
}
```

`components/business-trips-tab.tsx`:

```tsx
"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { TRIP_STATUS_META, tripRangeLabel, type TripStatus } from "@/lib/hris/business-trips";
import { useActiveEmployees } from "@/features/hris/attendance/queries";
import { useBusinessTrips } from "../queries";
import { useDecideBusinessTrip } from "../mutations";
import type { BusinessTrip, TripAction } from "../types";
import { TripRequestDialog } from "./trip-request-dialog";
import { TripStatusBadge } from "./trip-status-badge";

const ALL = "all";

/**
 * Absensi HRD → tab Perjalanan Dinas (EPIC-062): semua pengajuan, HRD bisa
 * menyetujui/menolak (cadangan atasan), membatalkan, dan mencatat dinas
 * karyawan (langsung disetujui).
 */
export function BusinessTripsTab() {
  const [status, setStatus] = useState<string>("pending");
  const [dialog, setDialog] = useState(false);
  const trips = useBusinessTrips(status === ALL ? {} : { status: status as TripStatus });
  const employees = useActiveEmployees().data;
  const employeeOptions = useMemo(
    () => (employees ?? []).map((e) => ({ value: e.id, label: e.nip ? `${e.full_name} (${e.nip})` : e.full_name })),
    [employees]
  );
  const decide = useDecideBusinessTrip();
  const busyId = decide.isPending ? decide.variables?.id : null;

  function act(trip: BusinessTrip, action: TripAction) {
    let rejectionReason: string | undefined;
    if (action === "reject") {
      rejectionReason = window.prompt("Alasan menolak Perjalanan Dinas ini?") ?? undefined;
      if (!rejectionReason?.trim()) return;
    }
    if (action === "cancel" && !window.confirm("Batalkan Perjalanan Dinas ini?")) return;
    decide.mutate(
      { id: trip.id, action, rejectionReason },
      {
        onSuccess: (json) => toast.success(json.message || "Berhasil diproses"),
        onError: (e) => toast.error(e.message || "Gagal memproses"),
      }
    );
  }

  const rows = trips.data ?? [];
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="w-48">
          <label className="mb-1 block text-xs font-medium text-gray-600">Status</label>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>Semua status</SelectItem>
              {(Object.keys(TRIP_STATUS_META) as TripStatus[]).map((s) => (
                <SelectItem key={s} value={s}>
                  {TRIP_STATUS_META[s].label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button onClick={() => setDialog(true)}>
          <Plus className="mr-1.5 h-4 w-4" /> Catat Perjalanan Dinas
        </Button>
      </div>
      <Card>
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <p className="p-6 text-center text-sm text-gray-500">Tidak ada Perjalanan Dinas untuk filter ini.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-gray-600">
                    <th className="p-3">Karyawan</th>
                    <th className="p-3">Tanggal</th>
                    <th className="p-3">Tujuan & keperluan</th>
                    <th className="p-3">Status</th>
                    <th className="p-3 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((trip) => (
                    <tr key={trip.id} className="border-b border-gray-50 align-top">
                      <td className="p-3">
                        <p className="font-medium text-gray-900">{trip.employee?.full_name ?? "—"}</p>
                        {trip.employee?.nip && <p className="text-xs text-gray-400">{trip.employee.nip}</p>}
                      </td>
                      <td className="p-3 whitespace-nowrap text-gray-700">{tripRangeLabel(trip.start_date, trip.end_date)}</td>
                      <td className="p-3 text-gray-700">
                        <p>{trip.destination}</p>
                        <p className="text-xs text-gray-500">{trip.purpose}</p>
                      </td>
                      <td className="p-3">
                        <TripStatusBadge status={trip.status} />
                        {trip.decided_by_name && <p className="mt-1 text-xs text-gray-400">oleh {trip.decided_by_name}</p>}
                        {trip.rejection_reason && <p className="text-xs text-red-600">{trip.rejection_reason}</p>}
                      </td>
                      <td className="p-3 text-right whitespace-nowrap">
                        {trip.status === "pending" && (
                          <>
                            <Button size="sm" disabled={busyId === trip.id} onClick={() => act(trip, "approve")}>
                              Setujui
                            </Button>{" "}
                            <Button size="sm" variant="outline" disabled={busyId === trip.id} onClick={() => act(trip, "reject")}>
                              Tolak
                            </Button>
                          </>
                        )}
                        {(trip.status === "pending" || trip.status === "approved") && (
                          <Button size="sm" variant="ghost" disabled={busyId === trip.id} onClick={() => act(trip, "cancel")}>
                            Batalkan
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
      <TripRequestDialog open={dialog} onOpenChange={setDialog} employees={employeeOptions} />
    </div>
  );
}
```

`frontend/src/app/dashboard/(dashboard)/me/perjalanan-dinas/page.tsx`:

```tsx
import { requireUser } from "@/lib/auth/require-user";
import { EssPerjalananDinasPage } from "@/features/hris/business-trips";

export default async function PerjalananDinasPage() {
  await requireUser();
  return <EssPerjalananDinasPage />;
}
```

`attendance-page.tsx`: impor `import { BusinessTripsTab } from "@/features/hris/business-trips";`, ubah `TabsList` menjadi `grid h-9 w-full max-w-xl grid-cols-4`, tambahkan `<TabsTrigger value="dinas">Perjalanan Dinas</TabsTrigger>` dan:

```tsx
        <TabsContent value="dinas" className="mt-4">
          <BusinessTripsTab />
        </TabsContent>
```

Perbarui JSDoc halaman: tambahkan butir "- Perjalanan Dinas: pengajuan dinas karyawan (EPIC-062), HRD bisa memutus dan mencatat."

- [ ] **Step 4: Jalankan test, tsc, eslint**

Run: `cd frontend && npx vitest run src/lib/hris/business-trips.test.ts src/features/hris`
Expected: PASS.
Run: `cd frontend && npx tsc --noEmit -p . && npx eslint src/lib/hris/business-trips.ts src/features/hris/business-trips src/features/hris/attendance "src/app/dashboard/(dashboard)/me/perjalanan-dinas"`
Expected: bersih.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/hris/business-trips.ts frontend/src/lib/hris/business-trips.test.ts frontend/src/features/hris/business-trips frontend/src/features/hris/attendance/components/attendance-page.tsx "frontend/src/app/dashboard/(dashboard)/me/perjalanan-dinas"
git commit -m "feat(hris-ui): Perjalanan Dinas di ESS (pengajuan + antrean atasan) dan tab HRD di Absensi, EPIC-062"
```

---

### Task 10: Verifikasi menyeluruh & dokumentasi

**Files:**
- Modify: `docs/epics/EPIC-062-absensi-geofence.md` (status, Automation Log)
- Modify: `docs/epics/README.md`
- Modify: `docs/hris/acuan-mom-20261019/status-requirement.md` (AB-01, AB-02, AB-03 + ringkasan)
- Modify: `docs/hris/acuan-mom-20261019/matriks-approval.md` (baris Perjalanan Dinas)

- [ ] **Step 1: Full suite**

Run: `cd backend && TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable' $HOME/.local/go/bin/go test ./... -count=1 > /tmp/epic062-go.txt 2>&1; grep -- '--- FAIL' /tmp/epic062-go.txt | sed 's/ (.*//' | sort -u`
Expected: nama yang gagal sama dengan baseline `main` (bandingkan dengan `diff`).

Run: `cd frontend && npx vitest run 2>&1 | tail -5 && npx tsc --noEmit -p .`
Expected: 0 gagal; tsc tanpa output.

- [ ] **Step 2: Cek manual di browser (DB test, bukan live)**

Jalankan API Go dan `next dev` terhadap DB test sesuai skill `run` proyek. Periksa:
1. Pengaturan → Bisnis → profil cabang: peta OSM tampil, klik peta mengisi Latitude/Longitude, mengetik koordinat memindahkan pin.
2. HRIS → Penempatan Karyawan → Lokasi Toko: hitungan "x dari y toko belum punya koordinat", klik peta + radius 250 menggambar lingkaran, simpan → toko tidak lagi "kosong".
3. ESS → Absensi dengan lokasi browser dipalsukan (DevTools → Sensors) ±300 m dari toko: popup muncul, "Tetap absen" aktif setelah 5 karakter, absen tercatat "Di luar area · 3xx m". Tanpa izin lokasi: "Belum dicek".
4. ESS → Perjalanan Dinas: ajukan, login sebagai atasan → setujui; absen jauh berikutnya tercatat "Perjalanan Dinas" tanpa popup.
5. HRIS → Absensi → Monitoring/Rekap: badge Masuk/Pulang dan keterangan tampil; tab Perjalanan Dinas memuat pengajuan.

- [ ] **Step 3: Dokumentasi**

- `status-requirement.md`: AB-01 → ✅ (kode; koordinat live menunggu K-14), AB-02 → ✅, AB-03 → ✅, dengan bukti `file:line` dan nama test; perbarui tabel ringkasan (hitungan ✅/🟡/❌).
- `matriks-approval.md`: tambahkan baris "Perjalanan Dinas | Atasan langsung (HRD cadangan & override) | Rentang ≤ 31 hari, diajukan sebelum berangkat | EPIC-062".
- Epic: Automation Log berisi commit Task 1–9, ruling R1–R11 yang dipakai (dan perubahan bila ada), bukti test, dan langkah rilis:
  1. Backup DB, terapkan migrasi `20261023090000` dan `20261023090100`.
  2. Jalankan `SELECT iam.sync_hris_staff_roles();` (grant `admin_hris` bertambah menu `ess.business-trip`).
  3. Build ulang API dan web.
  4. HR mengisi koordinat toko aktif lewat Lokasi Toko (K-14; data dari Paddy). Sebelum terisi, absen tercatat "Belum dicek".
  5. Tidak ada perubahan data live oleh agent.
- Set epic `status: review` dan baris registry README.

- [ ] **Step 4: Commit**

```bash
git add docs/epics docs/hris/acuan-mom-20261019
git commit -m "docs: EPIC-062 geofence absensi & Perjalanan Dinas selesai diimplementasikan, menunggu review"
```

## Follow-up (di luar rencana ini)

- Route fallback Next `frontend/src/lib/hris/attendance-repo.ts` (`clockIn`/`clockOut`) tidak menilai geofence; Go melayani `/api/hris` selama `BACKEND_URL` terpasang.
- Notifikasi HRD (lonceng/WhatsApp) untuk absen di luar area dan pengajuan Perjalanan Dinas baru.
- Filter "Di luar area saja" di Rekap dan kolom geofence di export CSV/XLSX/PDF absensi.
- Persetujuan Perjalanan Dinas masuk antrean gabungan "Persetujuan Tim" ESS saat Paket Approval atasan (langkah 5) dikerjakan.
- Pengajuan absen manual/koreksi karyawan (AB-04/AB-06) untuk kasus tanpa internet.
