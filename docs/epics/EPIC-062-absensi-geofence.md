# EPIC-062: Paket Absensi — Geofence Cabang & Perjalanan Dinas

status: on-progress
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
