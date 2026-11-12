# EPIC-106: Struktur Bisnis Paddy Group — Satu Holding, Dua Perusahaan Sejajar (`MODULE-CONFIGURATION`, lintas modul)

status: backlog
environment: dev
retries: 0

## Goal
Keputusan user 12-11-2026 (target "Sesudah"):

| Tingkat | | |
|---|---|---|
| Holding | **Paddy Group** (kode `PADDY`) | |
| Perusahaan (sejajar) | **CV Cipta Kreasi Kreatif** (kode `ERP`) | **Paddy** (kode `PADDY-RETAIL`) |
| Cabang | Paddy HQ Bandung (`ERP-BANDUNG`) ★ Kantor Pusat<br>Warehouse Paddy (`ODOO-WAREHOUSE-PADDY`) | semua toko, termasuk Paddy Pudak (`ODOO-PADDY-PUDAK`) dan Paddy Salam (`ODOO-PADDY-SALAM`) |

"CV Cipta Kreasi Kreatif" yang tadinya **holding** (kode `PROLOGE`) turun menjadi **perusahaan** dan sejajar dengan Paddy. Nama itu warisan migrasi template `business_hierarchy` (holding "Prologe"), yang diganti nama oleh seeder `paddy-branding.js` pada 15-10. **Tidak ada penggabungan perusahaan.** Skrip `2026-11-11_gabung_paddy_ke_kantor_pusat.sql` tidak dipakai.

**Eksekusi di staging menunggu struktur dan daftar orang terdampak dikonfirmasi user.** Perubahan ini memengaruhi hak akses, gaji/payroll, dan semua modul yang memakai cakupan perusahaan atau holding. Production di luar epic ini.

## Kondisi staging (read-only, 12-11-2026)
- **Holding:** `PROLOGE` "CV Cipta Kreasi Kreatif" berisi 1 perusahaan (ERP). `PADDY` "Paddy Group" berisi 1 perusahaan (PADDY-RETAIL).
- **ERP "Paddy Kantor Pusat":** 4 cabang (HQ Bandung, Warehouse Paddy, Paddy Pudak, Paddy Salam), belum ada Kantor Pusat, 157 user, 254 baris gaji, 1 payroll run `draft`, 26 pemetaan jurnal.
- **PADDY-RETAIL "Paddy":** 52 cabang (49 toko + 3 nonaktif), 192 user, 349 baris gaji, COA 204 akun, 1 tahun fiskal, 10 pemetaan jurnal.
- **Kolom `company_id` di data:** dari 121 tabel ber-`company_id`, hanya 7 yang berisi data, yaitu `configuration.branches`, `configuration.users`, `hris.employee_salary`, `hris.payroll_runs`, `accounting.chart_of_accounts`, `accounting.fiscal_years`, dan `accounting.journal_mappings`.
  - Jadwal, penempatan, absensi, gudang, dan transaksi menempel ke **cabang** (`branch_id`). ID cabang tidak berubah, jadi semuanya ikut tanpa disalin.
- **Data yang menempel ke Paddy Pudak dan Paddy Salam:** 1 penempatan aktif, **37 user** dengan cabang default di sana (perusahaan ERP), 15 baris gaji ERP, **387 absensi dalam 30 hari terakhir**, dan 2 gudang. Angka ini mengisyaratkan staf kantor pusat yang tercatat di sana, jadi harus dikonfirmasi (R4).
- **Kode program:** tidak ada yang menulis kode holding/perusahaan secara tertanam. Hak akses (`platform/scope`) membaca holding, perusahaan, dan cabang dari data user.

## Ruling
- **R1 — Kode dan ID tidak berubah.** `ERP`, `PADDY-RETAIL`, `PADDY` dan semua ID cabang tetap; yang berubah hanya nama dan relasi. Holding `PROLOGE` dihapus bila tidak ada lagi yang merujuknya. Bila masih ada, skrip BLOKIR dengan rincian.
- **R2 — Perusahaan ERP:** nama menjadi `CV Cipta Kreasi Kreatif`, `holding_id` menjadi Paddy Group.
- **R3 — Paddy Pudak dan Paddy Salam** pindah ke PADDY-RETAIL (keputusan user 12-11).
- **R4 — Orang mengikuti cabang, setelah dikonfirmasi.**
  - Invarian: `users.company_id` = perusahaan dari `users.branch_id`, dan `users.holding_id` = holding perusahaannya.
  - Baris `hris.employee_salary` yang masih berlaku mengikuti perusahaan cabang penempatan utama aktif karyawannya.
  - **Daftar 37 user, 15 baris gaji, dan pemilik 387 absensi** di Pudak/Salam dikirim ke user/HR dulu. Yang ternyata staf kantor pusat dipindah dulu cabang default-nya ke Paddy HQ Bandung (lewat UI atau lampiran skrip), baru skrip struktur dijalankan.
- **R5 — Kantor Pusat:** `ERP-BANDUNG` ditandai `is_primary`. PADDY-RETAIL tidak punya kantor pusat.
- **R6 — Payroll dan akuntansi:**
  - Payroll run `draft` yang memuat karyawan yang pindah perusahaan menghalangi eksekusi. HR menghapus atau menyusun ulang draft itu.
  - Run yang sudah `approved/paid` tidak diubah.
  - COA, tahun fiskal, dan pemetaan jurnal tidak dipindah, karena masing-masing tetap di perusahaannya.
- **R7 — Skrip satu file, pratinjau bawaan.** Mengikuti pola skrip gabung erp-paddy:
  - mode pratinjau (semua di-ROLLBACK) dan `-v eksekusi=1`;
  - output NOTICE `PINDAH`/`INVARIAN`/`BLOKIR`/`HASIL`;
  - idempoten: jalan kedua mengubah 0 baris.
- **R8 — Urutan dan izin:**
  1. PR skrip dan tes ke erp-paddy `development`. Deploy staging tidak menjalankan skrip.
  2. Laporan dampak dikonfirmasi user.
  3. Backup DB staging.
  4. Pratinjau di staging.
  5. Izin user.
  6. Eksekusi.
  7. QA lintas modul.

  Production butuh epic dan izin tersendiri.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-12-struktur-bisnis-paddy-group.md`.
- [ ] T1 Laporan dampak orang dan data (read-only), konfirmasi user
- [ ] T2 Skrip SQL struktur + tes integrasi (erp-paddy)
- [ ] T3 PR ke erp-paddy development
- [ ] T4 Backup, pratinjau, dan eksekusi di staging (dengan izin), QA lintas modul
- [ ] T5 Catatan epic, memori, sinkron dokumen

## Automation Log
- 2026-11-12: Epic dibuat. Struktur target dikonfirmasi user ("yang di sesudah target"). Belum ada data yang diubah. Pratinjau skrip gabung erp-paddy di staging sudah di-ROLLBACK.
