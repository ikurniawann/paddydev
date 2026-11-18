# EPIC-115 Pindah Branch CV Cipta Kreasi Kreatif ke Paddy — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Kedua branch milik perusahaan CV Cipta Kreasi Kreatif (`ERP`) pindah ke perusahaan Paddy (`PADDY-RETAIL`) beserta gudangnya, akunnya, dan gaji aktif stafnya. Perusahaan CV Cipta Kreasi Kreatif sendiri tetap ada.
- Paddy HQ Bandung (`ERP-BANDUNG`, gudang `WH-01` Gudang 1);
- Warehouse Paddy (`ODOO-WAREHOUSE-PADDY` di production, `WAREHOUSE-PADDY` di staging sesudah EPIC-107; gudang `MAIN`).

**Architecture:** Skrip ops SQL baru dengan pola EPIC-106 (`2026-11-12_struktur_bisnis_paddy_group.sql`):
- pratinjau bawaan (ROLLBACK) dan `-v eksekusi=1`;
- snapshot baris yang berubah di `audit.audit_log`;
- BLOKIR, PERHATIAN, dan INVARIAN;
- skrip pembalik yang membaca snapshot.

Skrip memindahkan **semua branch perusahaan sumber**, tidak memakai daftar kode, karena kode Warehouse Paddy berbeda antara staging dan production. Jumlah branch yang pindah dicetak dan diperiksa. History (absen, penempatan, jadwal, stok, transaksi) menempel ke `branch_id`/`warehouse_id`, jadi ikut tanpa disalin.

**Tech Stack:** PostgreSQL 16 (psql, plpgsql), Go integration test (pola `apps/backend/internal/modules/hris/business_structure_integration_test.go`).

**Spec:** Permintaan user 18-11-2026: "CV Cipta Kreasi Kreatif company itu tidak ada yang dipindah adalah branch dibawah company tsb, yaitu Paddy HQ Bandung (Kantor Pusat, ERP-BANDUNG, Gudang 1 WH-01) … Warehouse Paddy (ODOO-WAREHOUSE-PADDY, Main Storage MAIN) pindah jadi ke dibawah paddy code paddy-retail".

Keputusan user 18-11:
- **K-1:** gaji aktif staf ikut pindah ke Paddy.
- **K-2:** Paddy HQ Bandung menjadi Kantor Pusat Paddy.

Fakta production 18-11 (baca-saja):

| | Nilai |
|---|---|
| Branch ERP | 2 (ERP-BANDUNG ★, ODOO-WAREHOUSE-PADDY), kode tidak bentrok di PADDY-RETAIL |
| User ber-perusahaan ERP | 156: 155 scope `branch` (89 di ERP-BANDUNG, 66 di Warehouse), 1 scope `company` tanpa cabang (Bungsu Tri Sutrisna, role `employee`) |
| Penempatan aktif | 160 (93 ERP-BANDUNG, 67 Warehouse) |
| Gaji ERP | 266 baris, 139 aktif |
| Payroll run | tidak ada |
| Data ERP lain | `accounting.journal_mappings` 26 baris (milik perusahaan, tetap). COA hanya ada di PADDY-RETAIL. |
| Kantor Pusat PADDY-RETAIL | belum ada |

Staging (`paddydev-db`, paddydev.reddie.id): 2 branch ERP (ERP-BANDUNG ★, WAREHOUSE-PADDY), 155 user ERP, 129 gaji aktif, tidak ada payroll run.

## Global Constraints

- Skrip di repo `wit-id-indonesia/erp-paddy`, branch `chore/epic115-pindah-cabang-erp` dari `origin/development`, worktree `paddydev/.worktrees/erp-epic115`. Merge lewat PR.
- Lokasi skrip:
  - `apps/backend/database/sql/fixes/2026-11-18_pindah_cabang_erp_ke_paddy.sql`;
  - `apps/backend/database/sql/fixes/2026-11-18_pindah_cabang_erp_ke_paddy_balik.sql`.
- Setting psql: `pindah.dari` (default `ERP`), `pindah.ke` (default `PADDY-RETAIL`), `pindah.kantor_pusat` (default `ERP-BANDUNG`), `pindah.jumlah_branch` (default `2`), `pindah.mode` (`pratinjau`|`eksekusi`). Override lewat `PGOPTIONS`, seperti `struktur.*`.
- Keluaran `RAISE NOTICE` berformat TAB dengan awalan yang sama dengan EPIC-106: `STRUKTUR`, `PINDAH`, `HISTORY`, `USER`, `PERHATIAN`, `INVARIAN`, `BLOKIR`, `HASIL`.
- Perusahaan sumber **tidak** dinonaktifkan, dihapus, atau diganti nama. COA, tahun fiskal, pemetaan jurnal, payroll run, dan gaji tidak aktif tetap di perusahaannya.
- Eksekusi hanya dengan izin user, sesudah backup `pg_dump -Fc` (mode 600). Production hanya sesudah gladi di salinan production. Hindari jam 07–08, 13, 16–17, dan 21 WIB. Tidak membuat sesi atau akun sementara di production.

## Review Focus

1. **Staf yang punya gaji aktif di kedua perusahaan,** atau sudah punya baris gaji Paddy dengan `effective_date` sama: unique index `employee_salary_one_active_per_entity` / `employee_salary_entity_effective_key` akan gagal di tengah eksekusi. Harus menjadi **BLOKIR** sebelum ada perubahan. Diuji di Task 1.
2. **Payroll run perusahaan sumber yang belum dibayar:** memindah gaji membuat run itu kehilangan karyawan saat dihitung ulang. Kondisi ini harus BLOKIR. Diuji di Task 1.
3. **Paddy sudah punya Kantor Pusat lain saat eksekusi:** harus BLOKIR, karena dua Kantor Pusat dalam satu perusahaan dilarang. Diuji di Task 1.
4. **Akun ber-scope `company` di perusahaan sumber** (Bungsu): sesudah pindah, scope-nya menjadi seluruh Paddy. Tampil sebagai PERHATIAN, tidak diblokir. Diuji di Task 1.
5. **Jalan kedua dan pembalik:** jalan kedua mengubah 0 baris, dan pembalik mengembalikan baris yang sama persis. Diuji di Task 1 dan Task 2.

---

### Task 1: Skrip pindah branch beserta tes integrasinya

**Files:**
- Create: `apps/backend/database/sql/fixes/2026-11-18_pindah_cabang_erp_ke_paddy.sql`
- Test: `apps/backend/internal/modules/hris/branch_company_move_integration_test.go` (memakai `opsScript` dari `business_structure_integration_test.go`)

**Interfaces:**
- Produces:
  - konstanta tes `moveScript = "2026-11-18_pindah_cabang_erp_ke_paddy.sql"`;
  - `func moveSQL(t *testing.T, execute bool) string`, which equals `opsScript(t, moveScript, "SELECT set_config('pindah.mode', 'eksekusi', true);", execute)`;
  - entri audit `action = 'data_restructure.branch_company'` dengan `old_values` = `{"branches":[{id,company_id,is_primary}], "users":[{id,company_id,holding_id}], "salary":[{id,company_id}]}`.

Perubahan skrip (urutan):
1. Validasi: `pindah.dari` dan `pindah.ke` masing-masing tepat satu perusahaan, berbeda, dan di holding yang sama. Bila tidak, `RAISE EXCEPTION`.
2. `v_moving` = semua branch milik `dari`. Bila `cardinality(v_moving) <> pindah.jumlah_branch`, BLOKIR `jumlah_branch <n> (diharapkan <m>)`.
3. BLOKIR lain:
   - `kode_bentrok`: kode branch sudah ada di `ke`;
   - `kantor_pusat_ganda`: `ke` sudah punya branch `is_primary` lain;
   - `kantor_pusat_tidak_ada`: `pindah.kantor_pusat` tidak ada di `v_moving`;
   - `gaji_bentrok`: karyawan dengan gaji aktif di `dari` yang sudah punya gaji aktif di `ke`, atau baris `ke` dengan `effective_date` sama;
   - `payroll_belum_final`: run `dari` yang belum berstatus final. **Cek nilai status final yang dipakai modul payroll sebelum menulis kondisi ini**, lalu catat di ledger.
4. PERHATIAN: user `dari` dengan scope selain `branch`, yaitu nama, role, dan scope.
5. Eksekusi:
   - `configuration.branches.company_id` dipindah ke `ke` untuk `v_moving`; `is_primary` tetap, jadi ERP-BANDUNG menjadi Kantor Pusat `ke` (K-2);
   - `configuration.users.company_id` dipindah ke `ke` untuk semua user `dari`, dan `holding_id` mengikuti holding `ke`;
   - `hris.employee_salary.company_id` dipindah ke `ke` untuk gaji `dari` yang `is_active` (K-1).
6. INVARIAN sesudah eksekusi:
   - `dari` punya 0 branch, 0 user, dan 0 gaji aktif;
   - semua user yang `branch_id`-nya di `v_moving` ber-perusahaan `ke`;
   - `ke` punya tepat 1 Kantor Pusat;
   - jumlah HISTORY per kolom yang mereferensikan `branches` sama sebelum dan sesudah.
   
   Bagian HISTORY disalin dari skrip EPIC-106: loop FK satu kolom ke `configuration.branches`, ditambah FK ke `configuration.warehouses` untuk gudang di `v_moving`.

Langkah:
- [ ] **Step 1: Tulis tes yang gagal** di `branch_company_move_integration_test.go`. Fixture dibuat dengan kode acak, seperti `newStructureFixture`:
  - satu holding;
  - perusahaan sumber S dengan 2 branch: HQ (`is_primary`) dan gudang, masing-masing satu warehouse;
  - perusahaan tujuan T dengan 1 toko;
  - 2 user scope `branch` di HQ dan gudang, 1 user scope `company` di S;
  - 2 karyawan dengan penempatan dan gaji aktif di S, ditambah 1 gaji tidak aktif di S;
  - 1 absen di HQ.
  
  Settings `pindah.dari/ke/kantor_pusat` diisi kode fixture. Tes:
  - `TestBranchCompanyMovePreviewChangesNothing`: pratinjau, lalu state (company tiap branch, user, dan gaji) identik dengan sebelum, dan tidak ada baris audit.
  - `TestBranchCompanyMoveMovesBranchesUsersSalaries`: eksekusi, lalu:
    - kedua branch di T dan HQ `is_primary` di T;
    - 3 user di T dengan holding = holding T;
    - 2 gaji aktif di T, gaji tidak aktif tetap di S;
    - S tetap aktif dengan nama sama.
  - `TestBranchCompanyMoveKeepsHistory`: absen dan warehouse tetap menunjuk branch yang sama.
  - `TestBranchCompanyMoveSecondRunChangesNothing`: eksekusi kedua menghasilkan BLOKIR `jumlah_branch 0 (diharapkan 2)`, dan tidak ada perubahan.
  - `TestBranchCompanyMoveRefusesWhatNeedsReview`: masing-masing dalam savepoint terpisah, eksekusi ditolak dengan pesan berisi alasannya:
    - (a) T sudah punya Kantor Pusat → `kantor_pusat_ganda`;
    - (b) karyawan punya gaji aktif di S dan T → `gaji_bentrok`;
    - (c) run payroll S berstatus draft → `payroll_belum_final`;
    - (d) kode branch S sudah ada di T → `kode_bentrok`.
  - `TestBranchCompanyMoveWarnsCompanyScope`: keluaran pratinjau memuat `PERHATIAN` dengan nama user scope `company`.
- [ ] **Step 2: Jalankan, pastikan gagal.** `go test ./internal/modules/hris/ -run 'BranchCompanyMove' -count=1`. Expected: FAIL karena file skrip belum ada.
- [ ] **Step 3: Tulis skrip** sesuai urutan di atas. Header komentar menjelaskan tujuan, perubahan, cara pakai (pratinjau, eksekusi, pembalik, PGOPTIONS), dan format keluaran, seperti header EPIC-106.
- [ ] **Step 4: Jalankan** tes Step 2 dan `go test ./internal/modules/hris/ -run 'BusinessStructure|BranchCompanyMove' -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `chore(ops): skrip pindah branch CV Cipta Kreasi Kreatif ke Paddy (EPIC-115)`

### Task 2: Skrip pembalik

**Files:**
- Create: `apps/backend/database/sql/fixes/2026-11-18_pindah_cabang_erp_ke_paddy_balik.sql`
- Modify: `branch_company_move_integration_test.go`

**Interfaces:**
- Consumes: entri audit `data_restructure.branch_company` dari Task 1.
- Produces: `func moveRevertSQL(t *testing.T, execute bool) string`.

Pembalik membaca snapshot audit **terakhir yang belum dibalik** untuk aksi itu, mengembalikan `company_id`/`is_primary` branch, `company_id`/`holding_id` user, dan `company_id` gaji persis dari snapshot, lalu mencatat audit `data_restructure.branch_company_balik` yang menunjuk snapshot itu. Skrip ditolak (EXCEPTION) bila sesudah snapshot ada baris yang diubah lagi, misalnya user dipindah perusahaan oleh admin. Pola cek ini mengikuti pembalik EPIC-106.

- [ ] **Step 1: Tulis tes yang gagal** `TestBranchCompanyMoveRevertRestoresTheSameRows`: lakukan eksekusi lalu pembalik; md5 dari `string_agg` (branch, user, gaji milik fixture) sama dengan sebelum eksekusi. Pembalik kedua menghasilkan EXCEPTION "tidak ada snapshot yang belum dibalik".
- [ ] **Step 2: Jalankan, pastikan gagal.** Expected: FAIL karena file pembalik belum ada.
- [ ] **Step 3: Tulis pembalik** dengan pola `2026-11-12_struktur_bisnis_paddy_group_balik.sql`.
- [ ] **Step 4: Jalankan** `go test ./internal/modules/hris/ -run 'BranchCompanyMove' -count=1`, lalu seluruh `go test ./internal/modules/hris/... -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** `chore(ops): pembalik pindah branch ERP ke Paddy (EPIC-115)`

### Task 3: PR, staging, gladi production, serah terima

- [ ] **Step 1:** Review akhir branch oleh reviewer segar, lalu PR ke `development`. Merge menunggu izin user. Skrip ops tidak berjalan otomatis saat deploy.
- [ ] **Step 2 (izin user):** di staging:
  - backup `/mnt/hdd1/backup/paddydev/staging-before-pindah-cabang-erp-<stamp>.dump` (mode 600);
  - pratinjau; cek 0 BLOKIR, angka PINDAH (2 branch / 155 user / 129 gaji), dan daftar PERHATIAN;
  - eksekusi, lalu pratinjau ulang (BLOKIR `jumlah_branch 0`);
  - smoke: minta HR login dengan akun QA staf HQ dan gudang, cek halaman yang dibuka sama seperti sebelumnya; Struktur Bisnis menampilkan Paddy HQ Bandung ★ di bawah Paddy dan CV Cipta Kreasi Kreatif tanpa branch.
- [ ] **Step 3:** gladi di salinan production (restore lokal di network internal, seperti gladi 0.8):
  - pratinjau: 2 branch / 156 user / 139 gaji dan 0 BLOKIR;
  - eksekusi lalu pembalik, md5 sama;
  - salinan dihapus sesudahnya.
- [ ] **Step 4 (izin user, jam yang diizinkan):** di production:
  - backup `~/backups/prod-db-before-pindah-cabang-erp-<stamp>.dump` beserta salinannya di `/mnt/hdd1/backup/paddydev/`;
  - pratinjau, eksekusi, pratinjau ulang;
  - smoke baca-saja.
- [ ] **Step 5:** Catat di `docs/epics/EPIC-115-pindah-cabang-erp-ke-paddy.md` (paddydev): keputusan K-1/K-2, angka pratinjau/eksekusi, dan backup. Perbarui struktur sesudah di `docs/epics/EPIC-106-struktur-bisnis-paddy-group.md` (CV Cipta Kreasi Kreatif tanpa branch). Perbarui memori `struktur-bisnis-epic106`.
