# EPIC-062: Paket Absensi — Geofence Cabang & Perjalanan Dinas

status: ready-for-qa
environment: dev
retries: 0

## Goal
Absen ESS dicek di server terhadap lokasi toko tempat karyawan clock-in, sesuai MOM 19-10-2026 (paket eksekusi langkah 6 "Paket Absensi"). Cakupan:
- Geofence radius 100 m di sekitar toko yang dipilih dari penempatan aktif karyawan. Status (Valid / Invalid / Belum dicek / Perjalanan Dinas) dan jarak dalam meter disimpan untuk clock-in dan clock-out (AB-01, K-14).
- Popup ESS saat absen di luar area dengan keterangan wajib (AB-03).
- Pengajuan Perjalanan Dinas dengan rentang tanggal dan approval, yang membebaskan tanggal itu dari geofence (AB-02).
- HR mengisi koordinat toko lewat **peta** (OpenLayers + OSM, memakai stack peta yang sudah ada di situs dan member app): klik peta mengisi lat/lng, yang tetap bisa diketik. Dipasang di form profil cabang (Pengaturan → Bisnis) dan di HRIS → Penempatan Karyawan → "Lokasi Toko".
- HR melihat status, jarak, dan keterangan geofence di Monitoring dan Rekap absensi.

Pembatasan clock-in ke penempatan aktif karyawan tetap seperti sekarang (`frontend/src/features/hris/ess/components/ess-clock-panel.tsx`, `backend/internal/modules/hris/attendance_clock.go`, `placements.go` `clockBranch`).

Acuan: `docs/hris/acuan-mom-20261019/` — `status-requirement.md` (AB-01, AB-02, AB-03), `pertanyaan-rekomendasi.md` (K-14, urutan eksekusi langkah 6), `matriks-approval.md`.

## Tasks
Lihat `docs/superpowers/plans/2026-10-20-absensi-geofence.md` (10 task):
1. Aturan geofence (domain Go murni).
2. Clock-in/out menyimpan status & jarak geofence; di luar area wajib keterangan (422).
3. API lokasi toko untuk HR (koordinat + radius, audit).
4. Perjalanan Dinas (backend): tabel, approval, audit, pengecualian geofence.
5. Geofence tampil di Monitoring dan Rekap absensi HR.
6. Popup ESS di luar area dengan keterangan wajib.
7. Peta pemilih titik (OpenLayers + OSM) di form profil cabang.
8. Dialog "Lokasi Toko" untuk HR di Penempatan Karyawan (peta, tempel koordinat, radius).
9. UI Perjalanan Dinas (ESS + tab HR di Absensi).
10. Verifikasi menyeluruh & dokumentasi.

## Acceptance Criteria
- Clock-in dan clock-out oleh karyawan sendiri menyimpan `clock_in_geofence`/`clock_out_geofence` (`valid`, `invalid`, `unchecked`, `dinas`) dan jarak ke toko dalam meter (`*_distance_m`).
- Radius = 100 m, kecuali toko punya override `geofence_radius_m` (25–1000 m).
- Toko tanpa koordinat, perangkat tanpa GPS, atau koordinat tidak masuk akal (0,0, di luar rentang) → "Belum dicek", **tidak pernah** Invalid (K-14).
- Akurasi GPS: Invalid hanya bila jarak − min(akurasi, 100 m) > radius.
- Di luar area tanpa keterangan ≥ 5 karakter → 422 `geofence_note_required`. Belum ada baris absensi dan selfie belum disimpan. ESS menampilkan popup (jarak, toko, batas), lalu mengirim ulang dengan keterangan; absen tercatat Invalid dengan keterangan di `notes`.
- Absen yang dicatat HRD untuk karyawan lain, serta absen manual/koreksi HRD, tidak dinilai (kolom NULL).
- Karyawan bisa mengajukan Perjalanan Dinas: mulai ≥ hari ini, maks. 31 hari, tujuan dan keperluan wajib, tidak beririsan dengan pengajuan aktif.
- Perjalanan Dinas diputus atasan langsung (`reporting_to`) atau HRD (cadangan & override), tidak oleh pemohon sendiri. HRD bisa mencatat dinas karyawan (langsung disetujui) dan membatalkan. Setiap keputusan tercatat di audit log.
- Pada tanggal dinas yang disetujui, absen berstatus "Perjalanan Dinas" tanpa popup. Absen yang sudah terekam di rentang itu (periode belum closing) ikut diubah dari Invalid/Belum dicek menjadi Perjalanan Dinas saat disetujui.
- **Form profil cabang (Pengaturan → Bisnis) punya peta OpenLayers + OSM**: klik peta mengisi Latitude/Longitude (6 desimal), dan mengetik koordinat memindahkan pin.
- HR (menu Penempatan Karyawan) bisa mengisi koordinat dan radius tiap toko di dialog "Lokasi Toko" dengan peta yang sama (lingkaran radius tampil) atau dengan menempel koordinat Google Maps. Jumlah toko tanpa koordinat terlihat, dan setiap perubahan tercatat di audit log.
- Monitoring dan Rekap absensi HRD menampilkan badge geofence Masuk/Pulang (status + jarak) dan keterangan untuk absen di luar area.
- Status absensi dan payroll tidak berubah karena geofence.

## Pertanyaan terbuka
Rekomendasi di bawah dipakai sebagai ruling rencana (R1–R11). **User perlu mengonfirmasinya sebelum eksekusi**, karena butir-butir ini belum ada di `pertanyaan-rekomendasi.md` (keputusan 20-10-2026 hanya mencakup K-14).

| No | Pertanyaan | Rekomendasi |
|---|---|---|
| P-1 | Radius per toko atau satu angka global? | Global 100 m (konstanta, nilai MOM) + override opsional per toko (`geofence_radius_m`, 25–1000 m) yang diisi HR. Toko di dalam mal, gudang, atau HQ bisa lebih luas dari 100 m. Satu kolom nullable lebih murah daripada setting global yang tidak pernah diubah |
| P-2 | Bagaimana memperlakukan akurasi GPS yang buruk? | Toleransi = akurasi yang dilaporkan, dibatasi 100 m: Invalid hanya bila jarak − min(akurasi, 100) > radius. Tanpa batas, akurasi 5 km (lokasi dari IP/Wi-Fi) atau akurasi palsu akan meloloskan semua orang |
| P-3 | Absen di luar area diblokir atau boleh dengan keterangan? | Boleh dengan keterangan wajib, status Invalid. MOM AB-03 menulis "popup + kolom keterangan", dan memblokir akan menghentikan absen yang sah (antar barang, gudang, sinyal GPS buruk di dalam mal). HR meninjau di Rekap; tidak ada potongan otomatis |
| P-4 | Siapa yang menyetujui Perjalanan Dinas? `matriks-approval.md` tidak memuatnya | Atasan langsung (`reporting_to`), dengan HRD sebagai cadangan dan override, sama dengan pola time-off/lembur (Q-APV-3). HRD bisa mencatat dinas langsung (penugasan). Bila Paddy ingin HR saja, cukup ubah `CanDecideBusinessTrip` |
| P-5 | Apa arti clock-out di luar area? | Dinilai dengan aturan yang sama terhadap toko absensi hari itu, kolom terpisah, dan keterangan wajib bila Invalid. Tidak mengubah jam kerja/payroll; hanya bahan tinjauan HR (mis. pulang lebih awal dari luar toko) |
| P-6 | ESS offline (internet mati)? | ESS adalah web app tanpa antrean offline: absen gagal seperti sekarang. Kasus ini masuk pengajuan absen manual/koreksi karyawan → approval HR (AB-04/AB-06, K-9), di epic terpisah. Tanpa GPS tetapi online → "Belum dicek" |
| P-7 | Koordinat 52 cabang live diisi siapa dan kapan? | Sesuai K-14: Paddy mengirim daftar koordinat cabang aktif, HR mengisi lewat Lokasi Toko (peta) setelah rilis. Agent tidak mengubah data live. Selama kosong, absen "Belum dicek" |

## Catatan
- **Peta pemilih titik (permintaan user 20-10-2026):** sekarang koordinat cabang diketik tangan (Pengaturan → Bisnis → profil cabang: input angka Latitude/Longitude, tanpa peta). Proyek sudah memakai OpenLayers 10 + OSM di `frontend/src/features/site/components/branch-map.tsx` (peta lokasi situs) dan `frontend/src/features/member-app/components/geo-map.tsx` (rute member app). Keduanya hanya menampilkan, tidak bisa memilih titik. Rencana membuat komponen bersama `frontend/src/components/maps/location-picker-map.tsx` dengan stack dan pola setup yang sama (klik = pin, lingkaran radius geodesik). Komponen dimuat lewat `next/dynamic` `ssr: false`, dan dipakai di form profil cabang serta dialog Lokasi Toko HRIS (Task 7–8).
- Role HR live (`hris_manager`, `hris_non_payroll`, `hris_non_rekrutmen_kpi`, `admin_hris`) tidak punya menu `settings.business`, jadi HR mengisi koordinat lewat HRIS (menu Penempatan Karyawan), bukan lewat profil cabang. Koordinat yang sama dipakai peta situs publik.
- Route fallback Next untuk clock-in (`frontend/src/lib/hris/attendance-repo.ts`) tidak diubah: semua `/api/hris` dilayani Go.
- Migrasi: `20261023090000_attendance_geofence.sql`, `20261023090100_business_trips.sql` (setelah EPIC-061 `20261022090000`–`20261022090200`).
- Membatalkan Perjalanan Dinas yang sudah disetujui **tidak** mengembalikan absen yang sudah diubah menjadi `dinas` (ruling P17): HR meninjau absen di rentang itu secara manual bila perlu.
- Menu `ess.business-trip` ditempatkan sejajar dengan `ess.overtime` (di bawah `ess.requests` bila Lembur ada di sana, selain itu langsung di bawah `ess`; ruling P16) dan ditambahkan ke daftar menu yang dipertahankan `database/seeders/iam-menus.sql`.

## Automation Log
- 2026-10-20: Epic dibuat (status `backlog`). Kode dan DB test (port 55499, hanya baca) diperiksa. Temuan:
  - `configuration.branches.lat/lng` (`numeric(9,6)`) sudah ada; DB test 0 dari 3 terisi, live 0 dari 52 (K-14).
  - `ActivePlacements` sudah mengembalikan `lat/lng` ke ESS, dan ESS memakainya hanya untuk memilih toko terdekat.
  - Clock-in menerima `clock_in_location` {latitude, longitude, accuracy} dan `notes`, tetapi ESS tidak mengirim `notes`. Tidak ada cek radius.
  - Status absensi `remote` ("Remote / dinas luar") hanya bisa diset HR lewat koreksi.
  - Profil cabang butuh menu `settings.business` yang tidak dimiliki role HR live.
  - Peta OpenLayers + OSM sudah dipakai di situs dan member app.

  Rencana ditulis di `docs/superpowers/plans/2026-10-20-absensi-geofence.md` (10 task, ruling R1–R11). Pertanyaan terbuka P-1 s.d. P-7 menunggu konfirmasi user. `status-requirement.md` AB-01/02/03 kini menunjuk epic ini.
- 2026-10-20: Permintaan tambahan user: pilih titik toko lewat peta (bukan hanya ketik lat/lng) dengan komponen OpenLayers + OSM yang sudah ada. Dicatat di Goal, Acceptance Criteria, Catatan, dan Task 7 (form profil cabang) + Task 8 (Lokasi Toko HRIS).
- 2026-10-20: **Keputusan user**: "lanjutkan sampai semua epic completed", yaitu ikuti rekomendasi P-1 s.d. P-7: radius global 100 m + override per toko 25–1000 m; toleransi akurasi min(akurasi, 100 m); di luar area boleh absen dengan keterangan wajib (Invalid, tanpa blokir); Perjalanan Dinas diputus atasan langsung dengan HRD sebagai cadangan; clock-out dinilai dengan aturan sama; tanpa mode offline; koordinat 52 cabang live diisi HR setelah rilis (agent tidak mengubah data live). Eksekusi dimulai (subagent-driven).
- 2026-10-20: Task 4 (backend Perjalanan Dinas): tabel `hris.business_trips` + menu `ess.business-trip`, route `GET/POST /api/hris/business-trips` dan `POST /api/hris/business-trips/{id}/decide` (Go saja). Tanggal pengecualian geofence selalu dari server: clock-in = tanggal baris (hari ini WIB untuk karyawan), clock-out = tanggal baris absensi tersimpan (shift malam lewat tengah malam tetap `dinas`). Relabel saat disetujui melewati periode closing. Bukti: `TestBusinessTripFlow`, `TestBusinessTripRejectCancelAndClosedPeriod`, `TestBusinessTripDateIsServerSide` PASS; paket hris = baseline.
- 2026-10-20: **Implementasi selesai (Task 1–10), status `review`.** Subagent-driven; setiap task ditinjau terpisah. Commit pada branch `docs/epic062-geofence-absensi` (awal `c908a81`):

  | Task | Commit | Isi |
  |---|---|---|
  | 1 | `f63eeba` | Domain geofence Go murni (jarak, status, toleransi akurasi, validasi lokasi toko) |
  | 2 | `5c6172e` | Clock-in/out menyimpan status & jarak; di luar area wajib keterangan (422) |
  | 3 | `3f3b0f4` | `PUT /api/hris/branches/{id}/location` (koordinat + radius, audit) |
  | 4 | `50f802a` | Perjalanan Dinas backend (tabel, approval, audit, pengecualian geofence), menu `ess.business-trip` |
  | 5 | `c1e6351` | Geofence tampil di Monitoring dan Rekap absensi HRD |
  | 6 | `6b5ede7`, `c03e02c` | Popup ESS absen di luar area, badge absen hari ini; perbaikan teks bantuan (radius dari server) |
  | 7 | `a3c803d`, `c7f763f` | Peta pemilih titik OpenLayers/OSM di form profil cabang; perbaikan atribusi OSM, dispose, paste koma desimal |
  | 8 | `02f2794` | Dialog Lokasi Toko di Penempatan Karyawan |
  | 9 | `4f14d99`, `d8df58b` | UI Perjalanan Dinas (ESS + tab HRD di Absensi); perbaikan: Setujui/Tolak disembunyikan pada dinas HRD sendiri |
  | 10 | (commit docs ini) | Verifikasi menyeluruh & dokumentasi |

- 2026-10-20: **Ruling R1–R11 (rencana) dan P1–P18 (controller) yang dipakai, beserta perubahan:**
  - R1–R11 dipakai apa adanya (radius 100 m + override 25–1000 m; status valid/invalid/unchecked/dinas; toleransi `min(akurasi, 100 m)`; di luar area boleh absen dengan keterangan; clock-out dinilai terpisah; HRD untuk karyawan lain dan koreksi manual → NULL; Perjalanan Dinas atasan langsung + HRD; koordinat diisi HR lewat HRIS; peta pemilih titik bersama; tanpa mode offline).
  - **P1** setiap task yang menambah route Go menjalankan `make -C backend generate` dan meng-commit `go-routes.generated.json`. **P2** `/api/hris/business-trips` masuk `goNativePrefixes` (Go saja, tanpa fallback Next).
  - **P4 diubah menjadi "kontrol atribusi saja"**: peta memakai `Attribution({collapsible:false})` tanpa tombol zoom/rotasi, jadi klik peta tidak pernah zoom **dan** kredit OSM (lisensi) tetap tampil; field tidak dibungkus `<label>`. Dipakai di picker (Task 7) dan dialog Lokasi Toko (Task 8). `BranchMap`/`GeoMap` lama tetap tanpa atribusi (di luar cakupan, lihat follow-up).
  - **P5 tanggal dari server**: untuk absen sendiri, tanggal geofence dan pengecualian dinas memakai `TodayWIB`; `date` di body diabaikan untuk non-HRD (`TestBusinessTripDateIsServerSide`, `TestSelfClockIgnoresBodyDate`). Ruling tambahan Task 2: clock-out memakai tanggal baris absensi tersimpan (`attendanceDate(att)`) untuk semua actor, sehingga shift malam lewat tengah malam tetap `dinas`.
  - **P6** `submitClock` memakai parser error `lib/api-client` yang dibagi (membawa `details`). **P7** tes relabel melewati periode closing, tolak, batal setelah disetujui. **P8** tes `submitClock` → `ClockGeofenceError` pada 422 dan alur panel 422 → popup → kirim ulang payload sama + keterangan. **P9** `isGeofenceStatus` memakai `Object.hasOwn`.
  - **P10** teks popup/bantuan ESS memakai `radius_m` dari server (radius toko sendiri, bukan angka 100 tetap); konstanta radius default frontend satu tempat.
  - **P11** handler aksi bersama untuk dua komponen Perjalanan Dinas (`use-trip-decision.tsx`); Go memakai `auditMeta(r)`.
  - **P13 ClockPlacement.geofence_radius_m dibuang** (YAGNI; perubahan `/me/placements` tidak dikerjakan); `saveBranchLocation` bertipe subset respons API yang nyata.
  - **P16 menu diletakkan sejajar `ess.overtime`** (migrasi menempatkan `ess.business-trip` pada induk yang sama dengan Lembur: `ess.requests` bila Lembur ada di sana, selain itu langsung di bawah `ess`) dan ditambahkan ke daftar pertahanan `iam-menus.sql`.
  - **P17 tidak ada pembalikan saat dibatalkan**: membatalkan dinas yang sudah disetujui tidak mengembalikan absen yang sudah berstatus `dinas` (HR meninjau manual). **P18** endpoint lokasi memakai guard `placementManagers` yang sama dengan route Penempatan (sedikit lebih luas dari R9).
  - Ruling Task 3/9: respons toko tanpa kunci = null, jadi UI selalu mengirim ketiga kunci (`lat`, `lng`, `geofence_radius_m`).

- 2026-10-20: **Bukti test (Task 10).**
  - Go: `go test -p 1 ./... -count=1` pada DB test (port 55499): 28 nama `--- FAIL` **identik** dengan baseline (`diff` kosong; 27 gagal lama + `TestHrisStaffRolesDeriveFromAdminHris`, karena branch ini belum membawa kode EPIC-061 sementara DB test sudah memiliki migrasinya). Tidak ada kegagalan baru; test baru EPIC-062 (domain geofence/trip/lokasi, `TestClock*Outside*`, `TestClockGeofenceUnchecked`, `TestSelfClockIgnoresBodyDate`, `TestGeofenceInRosterAndList`, `TestBranchLocation`, `TestBusinessTrip*`) PASS.
  - Frontend: `npx vitest run` = 775 file lulus, 2 dilewati; 5601 test lulus, 10 dilewati, 0 gagal. `npx tsc --noEmit -p .` = tanpa output (exit 0).
  - Cek manual di browser **belum dijalankan** (tidak ada server dev/API pada sesi verifikasi); daftar di bawah wajib dijalankan sebelum rilis.

- 2026-10-20: **Cek manual sebelum rilis** (di DB test, bukan live):
  1. Pengaturan → Bisnis → profil cabang: peta OSM tampil, klik peta mengisi Latitude/Longitude (6 desimal), mengetik koordinat memindahkan pin.
  2. HRIS → Penempatan Karyawan → Lokasi Toko: hitungan "x dari y toko belum punya koordinat", klik peta + radius 250 menggambar lingkaran, simpan → toko tidak lagi "kosong".
  3. ESS → Absensi dengan lokasi browser dipalsukan (DevTools → Sensors) ±300 m dari toko: popup muncul, "Tetap absen" aktif setelah 5 karakter, absen tercatat "Di luar area · 3xx m". Tanpa izin lokasi: "Belum dicek".
  4. ESS → Perjalanan Dinas: ajukan, login sebagai atasan → setujui; absen jauh berikutnya tercatat "Perjalanan Dinas" tanpa popup.
  5. HRIS → Absensi → Monitoring/Rekap: badge Masuk/Pulang dan keterangan tampil; tab Perjalanan Dinas memuat pengajuan.
  6. Atribusi OSM terlihat di peta pemilih titik, dan mengklik atribusi tidak memindahkan pin.
  7. Klik di peta tidak pernah men-zoom peta (tidak ada tombol zoom; scroll/gerak tetap normal).
  8. Teks popup ESS menampilkan radius milik toko itu dari server (mis. toko dengan override 250 m menampilkan 250 m, bukan 100 m).
  9. Tab Perjalanan Dinas HRD menyembunyikan Setujui/Tolak pada perjalanan dinas milik HRD yang sedang login.

- 2026-10-20: **Langkah rilis** (semua oleh manusia; agent tidak mengubah data live):
  1. Backup DB, lalu terapkan migrasi `20261023090000_attendance_geofence.sql` dan `20261023090100_business_trips.sql` (setelah migrasi EPIC-061 `20261022090000`–`20261022090200`). Terapkan migrasi di luar jam sibuk absen (07:00–10:00 WIB): migrasi menambah CHECK dan indeks pada `hris.attendance` yang mengunci tulis sebentar.
  2. Jalankan `SELECT iam.sync_hris_staff_roles();` (grant `admin_hris` bertambah menu `ess.business-trip`).
  3. Build ulang API dan web.
  4. HR mengisi koordinat toko aktif lewat Lokasi Toko (K-14; data dari Paddy). Sebelum terisi, absen tercatat "Belum dicek".
  5. Tidak ada perubahan data live oleh agent.

- 2026-10-20: **Review akhir**: Ready to merge, 0 Critical/Important. Perbaikan: (1) teks bantuan panel absen ESS menyatakan GPS wajib; (2) test integrasi radius per toko (400 m valid, 250 m 422 dengan `radius_m` 250); (3) siklus modul `geofence.ts` ↔ `coords.ts` diputus (`GEOFENCE_DEFAULT_RADIUS_M` pindah ke `lib/maps/coords.ts`, diekspor ulang dari `geofence.ts`); (4) langkah rilis: migrasi di luar jam sibuk absen. Ditunda: keterlambatan diukur saat kirim ulang setelah 422, filter Rekap termasuk "Belum dicek", notifikasi atasan untuk pengajuan dinas pending.
- 2026-10-20: **Temuan minor ditunda** (ringkasan dari ledger; tidak memblokir rilis):
  - Task 1: batas clamp `h` dan kasus batas sudah ditutup di Task 2; hitungan karakter catatan di klien memakai code point seperti rune Go (Task 6).
  - Task 2: indeks parsial `idx_attendance_geofence_invalid` belum dipakai query (menunggu filter "Di luar area saja"); `date` di body untuk non-HRD diabaikan diam-diam (P5).
  - Task 3: snapshot "sebelum" tanpa `FOR UPDATE`; kunci absen = null; pembulatan `numeric(9,6)` ke 0,0; celah test (payload `after`, error tipe, radius saja, 401).
  - Task 4: status pengajuan diputuskan sebelum `CanDecide` (membocorkan status per id, sama dengan lembur); HR yang membatalkan menimpa `decided_by` dan audit "sebelum" tidak memuatnya; celah test (daftar `?employee_id` non-HR, relabel baris lama saat HR menugaskan, scope approvals non-manager).
  - Task 5: test Go hanya mencakup clock-in invalid (belum baris NULL/manual dan clock-out); wrapper badge kosong pada baris NULL; fallback "—" mengecek falsy, bukan `isGeofenceStatus`; keterangan tidak diatribusikan masuk/pulang; belum ada test komponen; field geofence di `RosterAttendance` wajib padahal fallback Next tidak mengisinya.
  - Task 6: dialog menutup sebelum hasil kirim ulang (galat jaringan → ulang selfie); kedip konten saat animasi tutup; dua ekspor bernama `ApiError` (klien vs server); error boundary menampilkan nama "ApiError:".
  - Task 7: animasi fly-to berjalan di tiap ketukan saat mengetik koordinat.
  - Task 8: siklus modul `geofence.ts` ↔ `coords.ts` (tanpa pemakaian saat inisialisasi; konstanta sebaiknya dipindah ke modul daun); dialog mempertahankan draf saat ditutup/dibuka dan tidak sinkron ulang ke nilai tersimpan; `missingCoords` menduplikasi `hasCoords`; belum ada test `onSuccess`/invalidasi; a11y `aria-pressed` pada daftar dan "0 dari 0" saat memuat.
  - Task 9: `LIMIT 200` pada daftar pengajuan memotong diam-diam "Semua status" di tab HRD.

- 2026-10-20: **Follow-up (di luar epic ini):**
  - `BranchMap` dan `GeoMap` (situs publik, member app) tidak menampilkan atribusi OSM (sudah begitu sebelum EPIC-062); tambahkan seperti picker.
  - `hiring_manager` dihitung sebagai HR untuk Perjalanan Dinas (`hrRoles`, konsisten dengan lembur/cuti); konfirmasi ke user apakah itu diinginkan.
  - Daftar Perjalanan Dinas dibatasi `LIMIT 200`; tambahkan paginasi/filter bila volume tumbuh.
  - Route fallback Next `frontend/src/lib/hris/attendance-repo.ts` (`clockIn`/`clockOut`) tidak menilai geofence; Go melayani `/api/hris` selama `BACKEND_URL` terpasang.
  - Notifikasi HRD (lonceng/WhatsApp) untuk absen di luar area dan pengajuan Perjalanan Dinas baru.
  - Filter "Di luar area saja" di Rekap dan kolom geofence di export CSV/XLSX/PDF absensi.
  - Persetujuan Perjalanan Dinas masuk antrean gabungan "Persetujuan Tim" ESS saat Paket Approval atasan (langkah 5) dikerjakan.
  - Pengajuan absen manual/koreksi karyawan (AB-04/AB-06) untuk kasus tanpa internet.
- 2026-10-20: Temuan lanjutan: kuasa HR Perjalanan Dinas (lihat semua, memutus cadangan/override, membatalkan yang disetujui, mencatat dinas karyawan lain) hanya `super_admin`/`admin`/`hrd`; `hiring_manager` (peran rekrutmen) tidak, dan berperilaku seperti karyawan biasa (ajukan sendiri, memutus hanya sebagai atasan langsung). Go: `Actor.IsHRD` (`tripHRRoles`), `IsHR` tidak berubah untuk fitur lain; frontend: tab Perjalanan Dinas di Absensi HRD hanya untuk peran itu (`canManageTrips`).
- 2026-10-21 07:14 WIB: **Deploy live** main `5035ae5` (PR #4–#8), image `local-20261021-071158`. Backup sebelum deploy: `~/Desktop/paddydev-backups/paddydev-db-before-epic059-062-deploy-20261021-071142.dump`. 9 migrasi diterapkan (20261020150000 s.d. 20261023090100), cek skema 0 berbeda; `SELECT iam.sync_hris_staff_roles()` dijalankan (grant aktif admin_hris 57, hris_manager 57, hris_non_payroll 52, hris_non_rekrutmen_kpi 40; hak hapus daun HRIS hanya hris_manager = 18). Container sehat tanpa restart; /login, /api/ready 200. 9 cek manual browser lulus di DB test (Playwright headless, kamera & GPS palsu) sebelum deploy. Migrasi dijalankan pukul 07:14 WIB (dalam jam sibuk absen, atas instruksi user; kunci tulis singkat). Koordinat 52 toko live masih kosong: absen tercatat "Belum dicek" sampai HR mengisi Lokasi Toko (K-14).
