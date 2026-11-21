# EPIC-117: Paket Perbaikan Pra-Rilis (`MODULE-HRIS`, `MODULE-PAYROLL`)

status: ready-for-qa
environment: production
retries: 0

## Goal
Permintaan user 18-11-2026, sebelum merge `development` dan rilis production:

> sebelum merge development dan production deployment buat perbaikan ini
> 1. dropdown payroll manual ga tampil semua
> 2. "tambahin cuti unpaid dan paid di slip gaji ada potongan Rp 0 dan penambah Rp 0 dihilangkan aja"
> 3. karyawan di staging username password di hapus
> 4. ketika dimatikan clock out otomatis nya masih ke detect clock otomatis di sisi karyawannya
> 5. data reddie cross check lagi karena masih ada yg absensi, cuti, izin, dan lainnya terhitung dari tgl 21 oktober (login ke reddiedev) staging
> 6. potongan izin, sakit, di crosscheck lagi pada modul payroll (sesuai rule / epic payroll) karena akan dites hari senin untuk payroll sistem paddy
> 6. pengumuman belum muncul di beranda ess karyawan harus di klik dlu baru muncul dan tidak ke notifikasi lonceng

Klarifikasi user:
- Butir 1: "butir 1 sudah dicek dan sudah berfungsi ganti jadi task kalender kerja koreksi jadi admin HR bisa ajukan input karyawan yang ingin ajuka cuti atau izin dadakan, satu lg kalender kerja di ess karyawan tidak selalu tampil jadwalnya masih blank kosong sama yang sudah clock in dan ada cuti atau libur tim nya jadi tidak ada perbaiki bug nya"
- Butir 3: "supaya karyawan aktif tidak bisa lagi login ke server staging paddydev sisakan role admin HR dan superadmin"
- Butir 5: "data nya jadi masih ada tgl 21 oktober keatas login ke staging absensi clock in dan clock out lalu ada pengajuan semua yang masuknya ke staging bukan ke production jadi jika ada migrasikan ke production aga benar ketika payroll nya tidak ada data yang tidak kehitung di production"

Rencana: `docs/superpowers/plans/2026-11-18-paket-perbaikan-pra-rilis.md`.

## Tasks
| Butir | Task rencana |
|---|---|
| A. HR input cuti/izin dadakan dari kalender | Task 5 |
| B. Kalender ESS kosong dan info tim | Task 6 |
| C. Slip: cuti dibayar/tanpa bayaran, baris Rp 0 disembunyikan | Task 1 |
| D. Tutup login karyawan staging | Task 7 |
| E. Checkout otomatis yang dimatikan | Task 3 |
| F. Migrasi baris staging-only ke production | Task 8 |
| G. Cross-check potongan izin/sakit | Task 2 |
| H. Pengumuman di beranda ESS dan lonceng | Task 4 |

## Acceptance Criteria
- [ ] Setiap task lolos tes (Go, vitest, tsc) dan review akhir.
- [ ] Staging: login karyawan ditolak, login HR dan super_admin tetap bisa.
- [ ] Production: absen dan lembur staging-only masuk sebelum payroll Senin; baris bentrok dilaporkan ke HR.
- [ ] Laporan cross-check izin/sakit periode 21-10 s.d. 20-11 diserahkan ke HR.
- [ ] Rilis production berikutnya didahului gladi di salinan production.

## Automation Log
- 2026-11-18: Investigasi (baca-saja):
  - staging adalah salinan production 10-11 05.52 WIB; baris staging-only: 40 absen (32 bisa dimigrasi, 8 bentrok), 1 lembur pending, 1 koreksi absen yang diduga duplikat;
  - aturan checkout otomatis: production 4 aturan nonaktif dan 0 absen ditutup otomatis dalam 10 hari; staging 3 aturan "semua lokasi" masih aktif;
  - slip menampilkan baris katalog walau Rp 0; hari cuti dibayar tidak disimpan;
  - `CreateAnnouncement` tidak menulis ke `public.notifications`;
  - backend sudah mendukung HR mengajukan cuti atas nama karyawan; kalender ESS menelan error jadwal dan menyembunyikan jam shift di layar ponsel.
- 2026-11-18: Rencana ditulis. Keputusan user:
  - **K-A1:** input HR dari kalender menunggu persetujuan atasan langsung (bukan langsung disetujui);
  - **K-B1:** tim = rekan terjadwal di toko yang sama pada tanggal itu, fallback atasan langsung yang sama;
  - **K-F1:** 8 absen bentrok dan 1 koreksi yang diduga duplikat tidak dimigrasi, didaftar untuk HR;
  - eksekusi Native.
- 2026-11-18 malam: Eksekusi Native dengan TDD di erp-paddy `fix/epic117-pra-rilis` (dari cbcf50a), 9 commit:
  - 0bbf93d C slip: baris Rp 0 disembunyikan; kolom `paid_leave_days`; Cuti dibayar/tanpa bayaran di web dan PDF;
  - 222f73e G tes aturan izin/sakit dari baris cuti sampai potongan (laporan: `docs/hris/acuan-mom-20261019/crosscheck-izin-sakit-2026-11.md`);
  - 5c48776 E aturan checkout otomatis per lokasi mengalahkan aturan semua lokasi;
  - 0ee888a H pengumuman membunyikan lonceng (`notified_at`, loop 5 menit), tautan ke detail, beranda/feed selalu dimuat ulang;
  - e73041b A tombol Input cuti/izin di detail hari kalender Rekap Absensi (pengajuan tetap menunggu atasan, K-A1);
  - 5aba937 B roster per tanggal mengalahkan pola mingguan, error jadwal tampil, label jam di ponsel;
  - dbd06eb B lapisan tim `GET /api/hris/me/team-calendar` (K-B1);
  - 7e10a13 D skrip tutup login staging (ban 2999-12-31, bukan `infinity` yang lolos di login Next);
  - 23f8cbf F skrip migrasi staging-only ke production + pemuat CSV + pembalik (K-F1).
- Temuan data (baca-saja production): kedua potongan izin/sakit aktif; 7 pengajuan cuti periode ini masih pending dan harus diputus sebelum payroll dihitung; 51 dari 315 karyawan aktif berakun tidak punya jadwal sama sekali (kalender kosong karena data).
- Gerbang: vitest 6896 lulus, tsc 0, eslint 0. Go suite penuh dan review akhir sedang berjalan.
- 2026-11-18 23.4x: Review akhir oleh reviewer segar (opus): 0 kritis, 3 penting, 7 minor. Diperbaiki (42df89c):
  - pembalik migrasi menolak bila payroll sudah dihitung sesudah migrasi;
  - pemuat CSV memakai `HEADER MATCH`, jadi kolom yang tertukar ditolak (diuji dengan psql nyata);
  - lembur yang bentrok dengan pengajuan aktif production dilaporkan, tidak menggagalkan migrasi;
  - skrip kunci login mengunci tabel akun selama transaksi.

  Minor yang ditunda: retry 403 jadwal (~3 dtk), run lama menampilkan "Cuti dibayar 0 hari" sampai dihitung ulang, md5 pembalik rapuh terhadap ALTER TABLE, komentar basi di `payslip_pdf.go`, tim kantor pusat bisa panjang, nama karyawan kosong bila `employee_id` di URL tidak ada di daftar.
- Gerbang akhir: semua paket Go ok, vitest 6896 lulus, tsc 0, eslint 0. Branch belum di-push; PR, staging, gladi, dan production menunggu izin user.
- Runbook: migrasi delta sebelum binary Go baru; migrasi F sebelum payroll Senin dihitung; pembalik F hanya sebelum payroll; foto absen staging tidak ikut pindah.
- 2026-11-19: Izin user ("Yes, open PR", lalu "Yes, check and merge"). PR [wit-id-indonesia/erp-paddy#21](https://github.com/wit-id-indonesia/erp-paddy/pull/21) dibuka; CI hijau (audit, Go, lint, unit test). Di-merge ke `development` (squash 622b25f).
- Sisa, masing-masing butuh izin user:
  - deploy staging;
  - kunci login staging (Task 7);
  - gladi rilis + migrasi F di salinan production;
  - production sebelum payroll Senin;
  - HR memutus 7 pengajuan cuti yang masih pending.
- 2026-11-19 00.45–02.00 WIB: **Rilis production** atas instruksi user ("deploy prod untuk PR semua epic yg kita kerjakan lakukan jadwal malam ini … pertama lancar dlu di development baru di prod"):
  - EPIC-113 (PR #16) di-rebase ke atas EPIC-117 (konflik `postgres_runs.go`, `slip_lines.go`, `calculator.go`, slip view digabung). Tes hris dan payroll hijau, CI hijau. Di-merge ke `development` (56fab25). Staging otomatis ter-deploy (`local-20261119-013453`, 657 migrasi, 0 error API).
  - Gladi di salinan production 00.49:
    - 26 migrasi dalam 1 dtk, verify 12.379 objek dengan 0 selisih;
    - jumlah data tetap;
    - health 200;
    - crawl super admin 379, HR 378, karyawan 377 halaman: 0 masalah baru (CRM Partners #418 sudah ada sebelumnya);
    - log API 0 error;
    - migrasi F: pratinjau, eksekusi, jalan kedua 0, pembalik md5 identik.
  - Gladi menemukan bug: skrip F gagal `relation "employees" does not exist` karena psql di server tidak memakai search_path aplikasi. Diperbaiki dengan RED→GREEN (PR #23).
  - Backup `~/backups/prod-db-before-rilis-epic117-20261119-014*.dump` (4.397 entri, salinan di `/mnt/hdd1/backup/paddydev/`, sha cocok) dan `~/paddydev-src.before-rilis-epic117-*`.
  - PR [#22](https://github.com/wit-id-indonesia/erp-paddy/pull/22) `development` → `production` (80afcce, tree = 56fab25). Deploy 01.46–01.55 WIB, image `local-20261119-014657`: 26 migrasi, verify 0 selisih, "Deploy selesai".
  - Smoke production:
    - `/login`, `/api/health`, `/api/ready` 200;
    - 0 error API selama 20 menit;
    - karyawan 648, gaji aktif 313, payroll run 0 (sama dengan sebelum rilis);
    - 610 sesi tetap aktif (tidak ada logout massal);
    - crawl 67 halaman publik: 0 masalah.
- 2026-11-19 ±01.58: **Migrasi F di production.** Cek ulang menyeluruh staging vs production (semua tabel hris/configuration/public/auth): staging adalah salinan production 10-11 05.52 WIB, jadi data 21-10 s.d. 10-11 sudah ada di production. Data baru karyawan/HR di staging sesudahnya hanya 40 absen, 1 lembur, dan 1 koreksi absen.
  - Tidak dibawa, karena production lebih baru:
    - roster 200 baris (production punya roster sendiri untuk tanggal yang sama);
    - 8 penempatan (production sudah berubah; Amyrah sudah resign);
    - koreksi absen Pasha 10-11 (production sudah punya koreksi manual yang disetujui);
    - payroll run uji HR 18-11;
    - perubahan kode toko EPIC-107 (menunggu QA HR).
  - Backup `prod-db-before-migrasi-staging-*.dump` (4.596 entri). 69 foto absen disalin (1 foto sudah hilang di staging).
  - Eksekusi: **32 absen + 1 lembur (Gantari Garini 18-11) masuk**, jalan kedua 0, 0 error API.
  - **8 absen bentrok untuk dicek HR** (staging vs production, jam masuk):
    - Asriani 16-11 (07.14 vs 10.51);
    - Deny Maulana Ramdhani 13-11 (07.59 vs 10.14);
    - Ifatul Fauziah 13-11 (13.07 vs 13.15);
    - Nasya Vilana Mardiansyah Putri 11-11 (15.21 vs 06.38) dan 16-11 (22.15 vs 15.57);
    - Pasha Mauludfi Lesmana 10-11 (13.32 vs 08.00) dan 11-11 (07.52 vs 08.00);
    - Yana Suryana 10-11 (09.07 vs 18.12).
- 2026-11-19 ±02.00: **Login staging dikunci** (backup `staging-before-tutup-login-*.dump`): 348 akun dikunci, 27 sesi dihapus; yang tersisa bisa login hanya hrd 1 dan super_admin 4.
- Sisa:
  - PR [#23](https://github.com/wit-id-indonesia/erp-paddy/pull/23) (search_path skrip) belum bisa di-merge karena GitHub Actions menolak menjalankan job (pembayaran akun gagal / batas belanja). Deploy staging otomatis juga ikut tertahan.
  - HR memutus 7 pengajuan cuti pending dan mengecek 8 absen bentrok sebelum payroll Senin.
  - QA HR dengan akun asli.
