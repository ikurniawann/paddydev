# EPIC-073: Jadwal Toko — Tampil untuk HR & Pintasan di Beranda HP (`MODULE-HRIS`)

status: coding
environment: dev
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
