# EPIC-115: Pindah Branch CV Cipta Kreasi Kreatif ke Paddy (`MODULE-CONFIGURATION`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 18-11-2026: perusahaan CV Cipta Kreasi Kreatif (`ERP`) **tidak** dipindah. Yang pindah adalah kedua branch-nya, masuk ke bawah Paddy (`PADDY-RETAIL`):
- Paddy HQ Bandung (`ERP-BANDUNG`, Kantor Pusat, gudang `WH-01` Gudang 1);
- Warehouse Paddy (`ODOO-WAREHOUSE-PADDY`, gudang `MAIN` Main Storage).

Langkah ini merevisi struktur EPIC-106, yang menaruh kedua branch itu di CV Cipta Kreasi Kreatif.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-18-pindah-cabang-erp-ke-paddy.md`.
1. Skrip ops pindah branch beserta tes integrasinya.
2. Skrip pembalik.
3. PR, staging, gladi di salinan production, production. Setiap langkah eksekusi butuh izin user.

## Keputusan
- **K-1 (user 18-11):** gaji aktif staf HQ dan gudang ikut pindah ke Paddy. Gaji tidak aktif tetap sebagai riwayat di CV Cipta Kreasi Kreatif.
- **K-2 (user 18-11):** Paddy HQ Bandung menjadi Kantor Pusat Paddy.
- Semua akun ber-perusahaan CV Cipta Kreasi Kreatif ikut pindah ke Paddy, karena scope akses mensyaratkan perusahaan akun = perusahaan baris. Di production per 18-11 ada 156 akun. Satu di antaranya ber-scope perusahaan (Bungsu Tri Sutrisna, role employee) dan sesudah pindah melihat seluruh Paddy (PERHATIAN).
- Skrip memindah **semua** branch perusahaan sumber, bukan daftar kode, karena kode Warehouse Paddy berbeda di staging (`WAREHOUSE-PADDY`, sesudah EPIC-107). Jumlahnya diperiksa (2).
- CV Cipta Kreasi Kreatif tetap ada beserta COA, tahun fiskal, dan 26 pemetaan jurnalnya. Posting transaksi di HQ dan gudang sesudah pindah memakai pemetaan Paddy atau global; event yang tidak punya pemetaan di Paddy ditampilkan sebagai PERHATIAN.

## Acceptance Criteria
- [x] Pratinjau tidak mengubah apa pun. Eksekusi memindah branch, akun, dan gaji aktif, dan HQ menjadi Kantor Pusat tujuan.
- [x] History (absen, penempatan, gudang) tetap. Jalan kedua ditolak.
- [x] Penghalang: jumlah branch, kantor pusat ganda/tidak ada, kode/gaji bentrok, payroll sumber/tujuan belum final, tabel ber-`*company_id` yang belum ditinjau.
- [x] Pembalik mengembalikan baris yang sama, dan menolak gaji bentrok atau perubahan sesudah pindah.
- [ ] Staging: pratinjau, eksekusi, smoke.
- [ ] Gladi di salinan production: pratinjau, eksekusi, pembalik.
- [ ] Production.

## Automation Log
- 2026-11-18: Data production dicek (baca-saja): 2 branch, 156 akun, 160 penempatan aktif, 139 gaji aktif, tanpa payroll run, dan Paddy belum punya Kantor Pusat. Staging: 2 branch, 155 akun, 129 gaji aktif.
- 2026-11-18: Eksekusi Native dengan TDD. Commit di erp-paddy `chore/epic115-pindah-cabang-erp`:
  - 8b4c51b: skrip pindah;
  - 863a3c0: pembalik;
  - dfa8533: perbaikan review akhir.
- 2026-11-18: Review akhir oleh reviewer segar (opus): tidak ada temuan kritis, 4 penting, 4 minor. Keempat temuan penting diperbaiki dengan tes RED→GREEN:
  - penghalang mencakup semua kolom `*company_id`;
  - payroll run tujuan yang belum dibayar memblokir;
  - PERHATIAN untuk pemetaan jurnal;
  - pembalik menolak perubahan sesudah pindah.
  
  Gerbang: `go test ./internal/modules/hris/...` ok.
- **Minor yang ditunda:**
  - akun tanpa perusahaan dengan cabang default HQ baru gagal di invarian, tidak di penghalang awal;
  - `DROP TABLE` tanpa `pg_temp`;
  - beberapa penghalang belum punya tes;
  - HISTORY hanya menghitung FK branch.
- 2026-11-18: PR [wit-id-indonesia/erp-paddy#18](https://github.com/wit-id-indonesia/erp-paddy/pull/18) dibuka. Staging, gladi, dan production menunggu izin user.
