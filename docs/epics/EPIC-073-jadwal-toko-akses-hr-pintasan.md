# EPIC-073: Jadwal Toko — Tampil untuk HR & Pintasan di Beranda HP (`MODULE-HRIS`)

status: ready-for-qa
environment: production
retries: 0

## Goal
Laporan user 30-10-2026: di video tutorial penempatan kerja oleh atasan, menu **Jadwal Toko** ada di Area Karyawan, tetapi di production dan staging tidak terlihat.

Penyelidikan (read-only production + akun pengatur demo di staging):
- Menu `ess.store-schedule` ada, aktif, dan di-grant ke semua peran (26). 21 pengatur di production semuanya karyawan aktif dengan akun login aktif.
- Menu sengaja **hanya tampil untuk pengatur aktif** (EPIC-071 R8/R17). Akun pengatur demo di staging melihatnya di laptop. Akun admin/HR yang dipakai user bukan pengatur, sehingga menunya tidak tampil. Video direkam dengan akun pengatur demo.
- Di HP, Jadwal Toko tidak ada di Beranda maupun bar bawah. Menu itu hanya bisa dicapai lewat **Lainnya → Area Karyawan → Jadwal Toko**, padahal koordinator toko umumnya memakai HP.

Keputusan user 30-10-2026: **"HR juga + pintasan HP"**.

## Ruling
- **R1** Menu Jadwal Toko tampil untuk pengatur aktif **atau** akun yang punya aksi `update` di menu Jadwal Kerja HR (`hris.kepegawaian.schedules`), sama dengan syarat server untuk mengubah jadwal (impor/simpan sel).
- **R2** Halaman Jadwal Toko untuk HR memakai **mode HR**: semua cabang/toko, sama seperti HRIS → Jadwal Kerja. Tab Penempatan tampil bila akun punya menu penempatan (prefix `placementManagers`, syarat rute penempatan). Bila akun HR sekaligus pengatur, mode HR yang dipakai karena lebih luas.
- **R3** `GET /api/hris/me/schedule-access` menambah `hr` (update di Jadwal Kerja) dan `hr_can_place` (menu penempatan). `is_delegate`/`can_place`/`branches` tetap sama.
- **R4** Beranda ESS menampilkan kartu pintasan **Jadwal Toko** untuk pengatur aktif dan HR (R1). Untuk pengatur, kartu menyebut jumlah toko wilayahnya. Kartu tampil di semua ukuran layar dan paling berguna di HP.
- **R5** Tanpa migrasi. Hak server tidak berubah: rute jadwal/penempatan tetap memeriksa HR atau penunjukan seperti EPIC-071.

### Ruling saat implementasi & review (R6+)
- **R6** `hr` (update di `hris.kepegawaian.schedules`) selalu berarti `hr_can_place` juga true, karena menu itu berada di bawah prefix `hris.kepegawaian` yang termasuk `placementManagers`. Jadi mode HR selalu menampilkan tab Penempatan, sama dengan yang sudah diizinkan server. Flag tetap dikirim untuk berjaga-jaga.
- **R7** Mode HR di halaman Jadwal Toko memakai halaman Jadwal Kerja & Penempatan dalam bentuk tersemat (`embedded`): tanpa judul HR ganda, "Atur penempatan karyawan" dan "Atur jadwal" berpindah tab di halaman yang sama, subjudul menyebut "semua toko".
- **R8** `schedule-access` menghitung `hr`/`hr_can_place` dari grant yang sudah dibaca `actorFor`, tanpa kueri tambahan. Beranda memuat akses Jadwal Toko paralel dengan data beranda.
- **R9** Kartu Beranda hanya tampil di Beranda ESS, yang menolak akun tanpa data karyawan. Akun HR tanpa data karyawan tetap mendapat menu di sidebar/Lainnya.

## Tasks
1. Server: `myScheduleAccess` + tes integrasi.
2. Menu: `canOpenJadwalToko` (pengatur ATAU update Jadwal Kerja) dipakai dua layout + tes.
3. Halaman Jadwal Toko mode HR + tes.
4. Kartu pintasan Beranda + tes.
5. Gerbang, QA staging (akun pengatur demo + akun HR demo, laptop & HP), PR, production.

## Acceptance Criteria
- Akun HR dengan update Jadwal Kerja melihat Area Karyawan → Jadwal Toko. Halamannya menampilkan semua toko (Jadwal), dan tab Penempatan bila punya menu penempatan.
- Pengatur melihat kartu Jadwal Toko di Beranda HP, dan tetap melihat menu seperti sebelumnya.
- Karyawan biasa tidak melihat menu maupun kartu.
- go test, pnpm test, lint, tsc hijau; CI 4/4; QA staging laptop + HP tanpa error konsol/API.

## Automation Log
- 2026-10-30: Penyelidikan selesai. Bukti: query read-only production (menu, grant, 21 pengatur aktif) dan akun pengatur demo di staging (menu tampil di laptop, tidak ada di Beranda HP), akun demo dihapus. User memilih "HR juga + pintasan HP".
- 2026-10-30: **Implementasi** `2457e4ac` (server), `507ba0f3` (menu), `bda7a238` (mode HR), `26a585bc` (kartu Beranda). Tes: `TestScheduleAccessHRFlags`, `canOpenJadwalToko`, halaman mode HR, `JadwalTokoShortcut`.
- 2026-10-30: **Review** (opus): Ready, 0 kritis/penting. Temuan minor tetap diperbaiki di `19217706` (R6–R8), karena terlihat di HP dan user meminta tidak ada yang tertunda.
- 2026-10-30: **Gerbang lokal:** `go vet` bersih; `go test -p 1 ./internal/modules/hris/...` ok; frontend `tsc` 0, `lint` 0, `pnpm test` 866 file / 6297 tes lulus.
- 2026-10-30 14:59: **Staging** image `local-20261030-145920` (branch `19217706`), tanpa migrasi. Backup `~/Desktop/paddydev-backups/staging-db-before-epic073-20261030-145045.dump`.
- 2026-10-30: **QA staging** dengan 3 akun demo (pengatur 1 toko ber-hak penempatan; HR `hrd` + `hris_manager` terhubung ke karyawan demo; karyawan biasa), laptop 1280 + HP 390: 12/12 lulus.
  - Menu: pengatur dan HR ada (sidebar laptop, sheet Lainnya di HP); karyawan biasa tidak.
  - Kartu Beranda: pengatur "…di 1 toko Anda", HR "Mode HR…"; karyawan biasa tidak ada.
  - Halaman: pengatur mode toko (PPVJ); HR "Mode HR" dengan tab Penempatan; karyawan biasa kartu "bukan pengatur".
  - Tidak ada halaman meluber, 0 error konsol, 0 respons API ≥ 400. Log API hanya mencatat 4 permintaan yang dibatalkan browser saat probe pindah halaman (`context canceled`), bukan galat aplikasi.
  - Data demo dihapus (0 tersisa; 10 pengatur staging utuh).
- 2026-10-30: **PR #54 merged** (`a8169dc0`), CI 4/4 hijau. Staging sudah menjalankan isi yang sama (QA 12/12 di atas). Production belum (menunggu izin user). Status `ready-for-qa`.
- 2026-10-30 20:02 WIB: **Deploy production** bersama EPIC-073, 074, 075 + PR #57 (catatan tertunda) + PR #58 (tab HP & redirect menu 404), atas izin eksplisit user ("Ya, deploy prod").
  - Image `local-20261030-200228` = main `589ac8c8`. Migrasi `20261030120000_ess_team_attendance_menu.sql` diterapkan (566 migrasi, verify 0 beda; menu Absensi Tim grant 26 peran). Container healthy, log API 0 error, absensi tetap masuk, 21 pengatur utuh, redirect `kpi-templates` → `kpi-config` dan `/dashboard/shop` → `/dashboard/shop/orders` aktif.
  - Backup `~/backups/prod-db-before-main-e073-075-20261030-200119.dump`. Source lama `~/paddydev-src.before-main-e073-075-20261030-200119` (= `28721f40`). Rollback: kembalikan folder source itu lalu jalankan `tools/deploy-paddydev.sh`. Migrasi menu boleh tetap ada: menunya tersembunyi bagi akun tanpa bawahan.
  - Status tetap `ready-for-qa` sampai HR/atasan mencoba di production (tanpa sesi uji di production).
