# EPIC-115: Pindah Branch CV Cipta Kreasi Kreatif ke Paddy (`MODULE-CONFIGURATION`)

status: ready-for-qa
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
- [x] Staging: pratinjau, eksekusi, smoke.
- [x] Gladi di salinan production: pratinjau, eksekusi, pembalik.
- [x] Production.
- [ ] QA HR dengan akun asli: staf HQ dan gudang membuka halaman yang biasa dipakai.

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
- 2026-11-18 20.51 WIB: Izin user: "lanjut setelah berhasil tidak ada error proses untuk masuk production tanpa error dan bug". PR #18 di-merge (51e68c3).
- 2026-11-18 20.51: Staging dibackup (`/mnt/hdd1/backup/paddydev/staging-before-pindah-cabang-erp-20261118-205111.dump`, 4.608 entri). Pratinjau berhenti pada **9 penghalang `tabel_belum_ditinjau`**, karena di staging ERP masih memiliki:
  - 25.493 produk dan 8.725 varian;
  - 1.992 bahan baku dan 1.469 varian bahan baku;
  - 2 PO, 2 GRN, 2 AP invoice, dan 3+2 rencana produksi.
  
  Ada juga 16 event jurnal yang hanya dipetakan untuk ERP. Eksekusi tidak dijalankan.
- Keputusan user sesudah itu:
  - data milik perusahaan ikut pindah;
  - pemetaan jurnal disalin ke Paddy.
  
  Skrip diperluas (PR #19, 925d8a6) dengan `pindah.tabel_ikut`, penghalang `ap_invoice_bentrok`, dan salinan pemetaan beserta barisnya; pembalik ikut mengembalikan keduanya. Production per 18-11 tidak punya produk, bahan baku, PO, atau GRN. Baris pemetaan ERP di production tidak berisi akun (73 baris), jadi finance tetap perlu mengisinya.
- 2026-11-18 20.56: Staging dieksekusi, 0 penghalang, 37.992 baris:
  - 2 branch, 155 akun, 129 gaji;
  - produk, bahan baku, PO, GRN, AP, dan rencana produksi;
  - 16 pemetaan disalin.
  
  Invarian OK dan HISTORY sama. Pratinjau ulang menghasilkan BLOKIR `jumlah_branch 0`. Smoke: `/login` dan `/api/ready` 200, 0 error API, 0 user beda perusahaan, 0 produk beda perusahaan gudang.
- 2026-11-18 20.56: Gladi di salinan production (`/mnt/hdd1/backup/paddydev/prod-copy-gladi-epic115-20261118-205639.dump`, container internal):
  - pratinjau dan eksekusi: 0 penghalang, 313 baris (2 branch / 156 akun / 139 gaji / 16 pemetaan);
  - pembalik: md5 identik (98f2699a…);
  - container gladi dibongkar sesudahnya.
- 2026-11-18 22.00: **Production dieksekusi**:
  - backup `~/backups/prod-db-before-pindah-cabang-erp-20261118-220025.dump` dan salinannya di `/mnt/hdd1/backup/paddydev/` (sha a61b7a1c…, 4.412 entri);
  - pratinjau sama persis dengan gladi;
  - eksekusi 22.00.50 WIB: 313 baris, invarian OK, COMMIT; pratinjau ulang menghasilkan BLOKIR `jumlah_branch 0`.
  
  Smoke:
  - `/login` dan `/api/ready` 200, 0 error API selama 5 menit;
  - ERP 0 branch; PADDY-RETAIL 56 branch dengan 1 kantor pusat (ERP-BANDUNG);
  - 0 user beda perusahaan;
  - gaji aktif ERP 0 (sisa 127 riwayat nonaktif), PADDY-RETAIL 311;
  - pemetaan PADDY-RETAIL 26.
  
  PERHATIAN: Bungsu Tri Sutrisna (employee, scope perusahaan) sekarang melihat seluruh Paddy.
- Sisa:
  - QA HR dengan akun asli;
  - hapus perusahaan ERP (EPIC-116, PR #20);
  - finance mengisi akun di pemetaan jurnal Paddy yang kosong.
