# EPIC-074: Absensi Tim — Atasan Melihat Presensi Anggota Timnya (`MODULE-HRIS`)

status: on-progress
environment: dev
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
