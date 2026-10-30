# EPIC-074: Absensi Tim — Atasan Melihat Presensi Anggota Timnya (`MODULE-HRIS`)

status: ready-for-qa
environment: production
retries: 0

## Goal
Permintaan user 30-10-2026: atasan bisa melihat absensi/presensi anggota timnya. Contoh kasus: **Azka** (Store Supervisor - Jawa Barat 2) belum bisa melihat absen timnya.

Kondisi per 30-10 (main `a8169dc0`):
- Area Karyawan sudah punya **Shift Tim** (atur jadwal bawahan langsung) dan **Persetujuan Tim**, tetapi belum ada tampilan absensi tim. `GET /api/hris/attendance` untuk non-HR selalu dipaksa ke data sendiri. Rekap harian `GET /api/hris/attendance/daily-roster` khusus HR (`IsHR`).
- Sejak perbaikan atasan 26-10 (`.share/akun/perubahan-atasan-posisi-20261026.md`), staf toko Azka berpindah atasan ke koordinator (mis. Rizal, Riri), sedangkan koordinator melapor ke Azka. Jadi staf toko adalah **bawahan tidak langsung** Azka.
- HR Monitoring (`AttendanceMonitoringTab` + `RosterRow`, server `RosterRows` + `buildRoster`) sudah menghitung status harian per karyawan: hadir, terlambat, belum absen, tidak hadir, cuti/izin, libur, libur nasional, tanpa jadwal.

## Ruling (dibuat tanpa tanya; user minta langsung direncanakan)
- **R1** "Tim" = semua karyawan **aktif** di bawah atasan dalam rantai `reporting_to`, sampai 10 tingkat (langsung dan tidak langsung), atasan sendiri dikecualikan. Pola yang sama dengan wilayah pengatur (EPIC-071, `delegateSubtree`). Tanpa ini Azka hanya melihat koordinator.
- **R2** Isi per hari sama dengan HR Monitoring: status, shift, jam masuk/pulang, menit terlambat, badge lokasi (geofence) dan kurang jam, jenis cuti. **Tanpa foto selfie**: endpoint foto khusus HR, dan atasan cukup melihat status.
- **R3** Anggota tidak langsung menampilkan "Atasan: <nama>" supaya atasan tahu jalurnya.
- **R4** Hanya baca. Koreksi tetap lewat Pengajuan Absensi karyawan atau HR.
- **R5** Tanggal bebas (default hari ini WIB), sama dengan HR Monitoring. "Lewat toleransi" hanya untuk hari ini.
- **R6** Menu ESS baru **Absensi Tim** (`ess.team-attendance`, `/dashboard/me/absensi-tim`), di-grant ke semua peran (pola `ess.store-schedule`). Menu hanya tampil untuk akun yang punya ≥ 1 bawahan aktif (R1). Halaman yang dibuka tanpa tim menampilkan penjelasan.
- **R7** Beranda ESS menampilkan kartu **Absensi Tim hari ini** (hadir, terlambat, belum absen) untuk atasan, menuju halaman Absensi Tim. Pelajaran EPIC-073: di HP, menu Area Karyawan tersembunyi di "Lainnya".
- **R8** HR Monitoring, hak HR, dan aturan approval tidak berubah. Migrasi hanya menambah menu.

### Ruling saat implementasi & review (R9+)
- **R9** Catatan bebas absensi (`notes`) hanya dikirim ke atasan bila absen masuk/pulang di luar area (geofence `invalid`), sama dengan yang ditampilkan halaman.
- **R10** Atasan yang data karyawannya nonaktif tidak punya tim, termasuk bila halaman dibuka lewat URL.
- **R11** Akun tanpa tim langsung mendapat respons kosong, tanpa kueri roster perusahaan. Beranda memanggilnya di setiap kunjungan.
- **R12** `/api/hris/me/team-attendance` hanya ada di Go (tanpa rute cadangan Next), tercatat di `backend-routes.test.ts` seperti EPIC-071.

## Tasks
Rencana: `docs/superpowers/plans/2026-10-30-absensi-tim-atasan.md`.

## Acceptance Criteria
- Atasan (mis. Azka) membuka Area Karyawan → Absensi Tim. Halaman itu memuat seluruh tim (langsung dan tidak langsung) untuk tanggal terpilih, dengan status, jam, dan ringkasan jumlah. Anggota tidak langsung menyebut atasannya.
- Karyawan tanpa bawahan tidak melihat menu maupun kartu. API mengembalikan daftar kosong untuk mereka, dan tidak pernah memuat karyawan di luar tim.
- Tidak ada foto selfie di respons tim.
- Kartu Beranda tampil untuk atasan di HP dan laptop.
- go test, pnpm test, lint, tsc hijau; CI 4/4; QA staging (laptop + HP).

## Automation Log
- 2026-10-30: Epic dibuat. Penyelidikan:
  - Kode `main` `a8169dc0`.
  - Staging read-only: Azka 26 bawahan langsung, data per 22-10.
  - Catatan perubahan atasan 26-10: staf toko Azka berpindah ke koordinator.
  - Pembacaan production diblokir pengaman izin, jadi tidak dipakai.
- 2026-10-30: **Implementasi** (eksekusi inline):
  - `50fe0256`: server.
  - `6d7fe991`: migrasi menu + visibilitas.
  - `895a3808`: halaman.
  - `4cfe81af`: kartu Beranda.
  - `c7664591`: manifest rute.
- 2026-10-30: **Review akhir** (opus): Ready with fixes, 0 kritis. Diperbaiki di `29f5ef34` (R9–R11; tes tim kosong kini memuat karyawan lain). Ditunda (minor): badge ringkasan nol masih tampil di halaman tim kosong; urutan menu 44 sama dengan Persetujuan Tim (diurutkan menurut nama).
- 2026-10-30: **Gerbang lokal:** `go vet` bersih; `go test -p 1 ./internal/modules/hris/...` ok; frontend `tsc` 0, `lint` 0, `pnpm test` 869 file / 6306 tes lulus.
- 2026-10-30 16:28: **Staging** image `local-20261030-162850` (branch `29f5ef34`). Migrasi `20261030120000_ess_team_attendance_menu.sql` diterapkan (grant 26 peran, verify 0 beda). Backup `~/Desktop/paddydev-backups/staging-db-before-epic074-20261030-162410.dump`.
- 2026-10-30: **QA staging** dengan data demo: atasan → koordinator → 2 staf, 1 staf absen terlambat hari ini, dan 1 karyawan biasa. Laptop 1280 + HP 390, 8/8 lulus.
  - Atasan: menu (sidebar / Lainnya) dan kartu "1 hadir · 1 terlambat · 0 belum absen" tampil; halaman memuat 3 anggota, dan staf menampilkan "Atasan: QA074 Koordinator…".
  - Karyawan biasa: tanpa menu, tanpa kartu; halaman "Belum ada anggota tim."
  - Foto tidak ada di respons; 0 halaman meluber; 0 error konsol; 0 respons API ≥ 400.
  - Data demo dihapus (0 tersisa).
- 2026-10-30: **PR #55 merged** (`36b374f5`) sesudah tes migrasi dibuat tahan terhadap CI paralel.
- 2026-10-30: **Catatan tertunda:**
  - `9c42fd14`: halaman Absensi Tim tanpa anggota tidak lagi menampilkan filter dan ringkasan bernilai nol.
  - Urutan menu 44 yang sama dengan Persetujuan Tim **dibiarkan**: nomor urut kembar sudah lazim di menu ESS (20 dipakai tiga menu, 45 dua menu) dan diurutkan menurut nama secara konsisten. Mengubahnya butuh migrasi baru tanpa manfaat.
- 2026-10-30 20:02 WIB: **Deploy production** bersama EPIC-073, 074, 075 + PR #57 (catatan tertunda) + PR #58 (tab HP & redirect menu 404), atas izin eksplisit user ("Ya, deploy prod").
  - Image `local-20261030-200228` = main `589ac8c8`. Migrasi `20261030120000_ess_team_attendance_menu.sql` diterapkan (566 migrasi, verify 0 beda; menu Absensi Tim grant 26 peran). Container healthy, log API 0 error, absensi tetap masuk, 21 pengatur utuh, redirect `kpi-templates` → `kpi-config` dan `/dashboard/shop` → `/dashboard/shop/orders` aktif.
  - Backup `~/backups/prod-db-before-main-e073-075-20261030-200119.dump`. Source lama `~/paddydev-src.before-main-e073-075-20261030-200119` (= `28721f40`). Rollback: kembalikan folder source itu lalu jalankan `tools/deploy-paddydev.sh`. Migrasi menu boleh tetap ada: menunya tersembunyi bagi akun tanpa bawahan.
  - Status tetap `ready-for-qa` sampai HR/atasan mencoba di production (tanpa sesi uji di production).
