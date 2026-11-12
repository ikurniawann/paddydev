# EPIC-106: Struktur Bisnis Paddy Group — Satu Holding, Dua Perusahaan Sejajar (`MODULE-CONFIGURATION`, lintas modul)

status: coding
environment: dev
retries: 0

## Goal
Keputusan user 12-11-2026 (target "Sesudah"):

| Tingkat | | |
|---|---|---|
| Holding | **Paddy Group** (`PADDY`) | |
| Perusahaan (sejajar) | **CV Cipta Kreasi Kreatif** (`ERP`, dulu "Paddy Kantor Pusat") | **Paddy** (`PADDY-RETAIL`) |
| Cabang | Paddy HQ Bandung (`ERP-BANDUNG`) ★ Kantor Pusat<br>Warehouse Paddy (`ODOO-WAREHOUSE-PADDY`) | semua toko, ditambah Paddy Pudak (`ODOO-PADDY-PUDAK`) dan Paddy Salam (`ODOO-PADDY-SALAM`); cabang nonaktif (Event PVJ, Paddy Store Kopo/PVJ, dst.) tetap di sini |

- Holding lama `PROLOGE` ("CV Cipta Kreasi Kreatif", warisan template Mizu dan rebrand 15-10) **dinonaktifkan, tidak dihapus**.
- **Tidak ada penggabungan perusahaan.** Skrip `2026-11-11_gabung_paddy_ke_kantor_pusat.sql` tidak dipakai dan sekarang diberi peringatan "JANGAN DIJALANKAN".
- Kode ERP, PADDY-RETAIL, dan id cabang tidak berubah.

Kode ada di `wit-id-indonesia/erp-paddy`, branch `fix/epic106-struktur-bisnis` (PR ke `development`). Epic dan rencana ada di repo paddydev ini.

## Kondisi staging (read-only, 12-11-2026)
- **Holding:** `PROLOGE` berisi ERP; `PADDY` berisi PADDY-RETAIL.
- **ERP "Paddy Kantor Pusat":** 4 cabang (HQ Bandung, Warehouse Paddy, Pudak, Salam), belum ada kantor pusat, 157 user, 1 payroll run `draft` November.
- **PADDY-RETAIL "Paddy":** 52 cabang (41 aktif, 11 nonaktif) dan 192 user. Ada 4 super admin tanpa holding/perusahaan.
- **Tabel ber-`company_id`:** 121 tabel; yang berisi data hanya `branches`, `users`, `employee_salary`, `payroll_runs`, COA (PADDY-RETAIL), `fiscal_years` (PADDY-RETAIL), dan `journal_mappings`. Untuk `holding_id`, 2 tabel (`companies`, `users`). Tidak ada baris lain milik ERP yang menempel ke cabang Pudak/Salam.
- **Pudak/Salam:**
  - 37 user dengan cabang default di sana. **35 di antaranya punya penempatan utama aktif di HQ Bandung** (penempatan Pudak/Salam mereka berakhir 09–15 Okt) dengan gaji ERP. Absen mereka 30 hari terakhir: 365 di Pudak/Salam dan 68 di HQ.
  - Penempatan utama aktif di Pudak/Salam hanya 1 orang ("IT Paddy", tanpa gaji). 1 user lagi (Silvi) berpenempatan di toko PPGA dengan gaji ERP.
  - History: 387 absensi, 19 penempatan, 2 gudang, 37 alokasi payroll draft, dan 3 delegasi jadwal.
- **Selisih yang sudah ada sebelumnya:** 9 karyawan aktif yang gajinya hanya di perusahaan lain dari penempatan utamanya. Dilaporkan, tidak diubah.

## Ruling
- **R1 — Kode dan id tetap;** yang berubah hanya nama dan relasi. Holding `PROLOGE` dinonaktifkan.
- **R2 — Perusahaan ERP** diganti nama menjadi `CV Cipta Kreasi Kreatif` dan pindah ke holding PADDY.
- **R3 — Pudak dan Salam** pindah ke PADDY-RETAIL; tanda kantor pusatnya dilepas.
- **R4 — Perusahaan user mengikuti penempatan utama aktif (brief langkah c).**
  - User yang cabang default-nya Pudak/Salam tetapi penempatan utamanya di perusahaan lain (35 staf HQ): cabang default diganti ke cabang penempatan utama (HQ Bandung), perusahaan tetap ERP.
  - Alasannya: hak akses ber-scope cabang (`scope.RowInScope`) mensyaratkan perusahaan user sama dengan perusahaan barisnya, sedangkan absen memakai penempatan, bukan `users.branch_id` (`placements.go` `ClockPlacements`). Jadi absen masuk tidak terganggu.
  - Selain itu user mengikuti cabangnya (PADDY-RETAIL).
- **R5 — Gaji:** hanya gaji **aktif** karyawan yang penempatan utama aktifnya di Pudak/Salam yang pindah ke PADDY-RETAIL; gaji tidak aktif adalah riwayat. Hasil di staging: 0 baris.
- **R6 — Holding user** mengikuti holding perusahaannya. User tanpa perusahaan di PROLOGE pindah ke PADDY. Super admin tanpa holding tidak disentuh.
- **R7 — ERP-BANDUNG** ditandai kantor pusat. COA, tahun fiskal, pemetaan jurnal, dan payroll run tidak dipindah karena milik perusahaan, bukan cabang.
- **R8 — Payroll draft ERP November** mengalokasikan 37 baris biaya ke Pudak/Salam (staf ERP yang absen di toko). Ini **PERHATIAN, bukan BLOKIR**: draft dihitung ulang sebelum diproses. Run ERP yang belum final dan memuat karyawan yang **gajinya** pindah memblokir eksekusi.
- **R9 — Penghalang (BLOKIR):**
  - perusahaan lain masih di holding lama;
  - kode cabang bentrok di PADDY-RETAIL;
  - kantor pusat ERP lain;
  - gaji ganda;
  - payroll belum final memuat karyawan yang gajinya pindah;
  - tabel lain dengan `company_id` ERP dan `branch_id` Pudak/Salam;
  - tabel lain yang merujuk holding lama;
  - history berubah;
  - invarian gagal.
- **R10 — Pembalik** `…_balik.sql` mengembalikan baris dari snapshot di `audit.audit_log` (action `data_restructure.business`), dengan angka per tabel yang sama.
- **R11 — Seeder** `business-hierarchy.js` sekarang membangun struktur target. ERP dicari per kode di holding mana pun dan dipindah ke Paddy Group, tidak pernah membuat ERP kedua, dan PROLOGE yang kosong dinonaktifkan. `items-business-scope.js` (erp-paddy) mencari scope per kode perusahaan dan cabang; holding hanya disaring bila `SEED_HOLDING_CODE` diisi.
- **R12 — UI:** pemilih scope akun tidak menawarkan holding, perusahaan, atau cabang nonaktif untuk pilihan baru; pilihan akun lama tetap tampil.
- **R13 — Migrasi tidak diubah.** DB baru dari nol masih membangun PROLOGE → ERP dari delta `business_hierarchy`. Menambah migrasi struktur akan menjalankannya otomatis di staging/prod tanpa pratinjau dan izin, jadi untuk DB baru struktur target dibangun dengan seeder.
- **R15 — Review akhir (reviewer terpisah, 12-11). Temuan Important diperbaiki, masing-masing dengan tes merah lalu hijau:**
  - "Penempatan utama" sekarang = yang berlaku **hari ini WIB** (`start_date <= hari ini <= end_date`, karyawan aktif), satu definisi untuk gaji, user, dan laporan.
  - Akun ber-scope perusahaan/holding tetap di perusahaannya.
  - BLOKIR baru:
    - `outlet_di_cabang_pindahan`, `penempatan_masa_depan_beda_perusahaan`, `karyawan_ganda`, `penempatan_perusahaan_lain`;
    - `payroll_belum_dibayar` (status selain `paid`, termasuk `completed`), `payroll_periode_ganda`.
  - Pratinjau mencetak `USER` per orang di cabang pindahan.
  - Pembalik membalik semua snapshot yang belum dibalik (terbaru dulu).
  - `-v eksekusi=0` tetap pratinjau; pemindaian tabel lewat pg_catalog.
  - Seeder memakai kode perusahaan sendiri (`ERP`), karena salinan paddydev `scripts/items-business-scope.js` masih bernilai `MIZU`.
- **R16 — Keputusan user sebelum eksekusi staging.** Pratinjau staging memblokir 2 orang:
  - **Febri Ayu Astiani:** penempatan utama HQ Bandung, outlet default di gudang Pudak.
  - **Adrianus Adrian Chandra:** penempatan utama HQ Bandung, outlet default di gudang Salam.

  Mengikuti penempatan akan membuat outlet mereka berada di luar scope. Pilihan:
  - (a) HR memindah outlet default mereka ke HQ;
  - (b) HR memindah penempatan utama mereka ke toko, sehingga mereka ikut PADDY-RETAIL dan gajinya ikut pindah; tetapi payroll draft ERP November memuat mereka, jadi draft dihapus atau dihitung ulang dulu.

  Selain itu, 35 staf berpenempatan HQ tercatat absen di Pudak/Salam (365 dari 433 absen dalam 30 hari). Bila mereka sebenarnya kru toko, penempatan utamanya perlu dibetulkan HR **sebelum** skrip dijalankan.
- **R14 — Urutan dan izin:**
  1. PR dengan CI hijau, lalu merge (deploy staging tidak menjalankan skrip).
  2. Backup staging.
  3. Pratinjau, angkanya dilaporkan ke user.
  4. Izin user.
  5. Eksekusi.
  6. Verifikasi.

  Production butuh izin tersendiri dan gladi di salinan prod.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-12-struktur-bisnis-paddy-group.md`.
- [x] T1 Laporan dampak (read-only) dan gladi di salinan staging
- [x] T2 Skrip SQL struktur + pembalik + tes integrasi Go (8 tes)
- [x] T3 Kode yang bergantung pada PROLOGE/holding: seeder, skrip scope, pemilih scope UI, peringatan skrip gabung
- [x] T4a Gate lokal 2× + review akhir + perbaikan temuan
- [ ] T4b PR ke erp-paddy development, CI hijau, merge
- [ ] T5 Staging: backup, pratinjau ke user, eksekusi berizin, verifikasi
- [ ] T6 Production: gladi di salinan prod terbaru, menunggu izin user
- [ ] T7 Catatan epic, memori, sinkron dokumen

## Automation Log
- 2026-11-12: Epic dibuat. Struktur target dikonfirmasi user ("yang di sesudah target").
- 2026-11-12: Brief eksekusi user diterima: PROLOGE dinonaktifkan (bukan dihapus), langkah c mengikuti penempatan utama, ditambah SQL pembalik dan tes.
- 2026-11-12: Gladi di salinan staging (pg_dump → DB `stg_e106` di container uji; staging tidak disentuh):
  - **Pratinjau:** 0 penghalang, semua INVARIAN OK.
  - **STRUKTUR sesudah:** PADDY > ERP "CV Cipta Kreasi Kreatif" (ERP-BANDUNG★, ODOO-WAREHOUSE-PADDY) dan PADDY-RETAIL (43 aktif / 54 cabang termasuk Pudak, Salam). PROLOGE nonaktif dengan 0 perusahaan.
  - **Eksekusi:** 162 baris, yaitu branches 2, kantor pusat 1, companies 1, holdings 1, users 157 (37 perusahaan/cabang, 157 holding), dan salary 0.
  - **HISTORY tetap:** absensi 387, penempatan 19, alokasi payroll 37, gudang 2, delegasi jadwal 3.
  - **Jalan kedua:** 0 baris.
  - **Pembalik:** 162 baris dengan angka per tabel yang sama. md5 struktur kembali identik dengan staging asli. Pembalik kedua 0 baris.
- 2026-11-12: Review akhir: 5 temuan Important diperbaiki (R15). Tes struktur 11/11 hijau (11 kasus penghalang).
- 2026-11-12: Gladi ulang di salinan staging yang baru:
  - **Pratinjau (dan `-v eksekusi=0`):** 37 baris USER, terdiri dari 35 ke ERP-BANDUNG/ERP, 1 Silvi tetap di Pudak lalu ke PADDY-RETAIL, dan 1 IT Paddy tetap di Salam lalu ke PADDY-RETAIL. **BLOKIR 2:** Febri dan Adrian (`outlet_di_cabang_pindahan`).
  - **Simulasi keputusan (a) di salinan:** outlet keduanya dipindah ke HQ. Eksekusi 162 baris dengan 0 penghalang dan semua INVARIAN OK. Jalan kedua 0 baris. Pembalik 162 baris dari 1 snapshot. Pembalik kedua tidak menemukan snapshot. md5 kembali identik.
- 2026-11-12 20:22 WIB: erp-paddy PR #1 (CI 4/4 hijau) di-squash-merge ke development (a4548e2). Deploy Staging sukses, `/api/ready` 200.
- 2026-11-12 20:50 WIB: backup staging `/mnt/hdd1/backup/paddydev/staging-before-struktur-bisnis-20261112-205052.dump` (3,9 MB, 495 tabel data, mode 600).
- 2026-11-12 20:51 WIB: **pratinjau di staging** (ROLLBACK; staging tidak berubah):
  - PINDAH: branches 2, users 157, salary 0.
  - USER 37: 35 dari Pudak/Salam ke ERP-BANDUNG dengan perusahaan tetap ERP, 2 ke PADDY-RETAIL.
  - **BLOKIR 2:** outlet Febri Ayu Astiani (Pudak) dan Adrianus Adrian Chandra (Salam).
  - **Eksekusi menunggu keputusan user untuk R16 dan izin.**
