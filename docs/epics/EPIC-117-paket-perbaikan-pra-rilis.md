# EPIC-117: Paket Perbaikan Pra-Rilis (`MODULE-HRIS`, `MODULE-PAYROLL`)

status: backlog
environment: dev
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
