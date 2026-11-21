# EPIC-116 Hapus Perusahaan CV Cipta Kreasi Kreatif (ERP) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Perusahaan `ERP` (CV Cipta Kreasi Kreatif, sudah kosong sesudah EPIC-115) bisa dihapus. Bila penghapusan perusahaan atau holding masih terhalang data, pesan yang muncul menyebut tabel dan jumlah barisnya, bukan error umum.

**Architecture:**
- **Kode:** `deleteBusinessEntity` (Go, `configuration/settings/business.go`) dan fallback Next memeriksa foreign key `ON DELETE RESTRICT/NO ACTION` yang masih menunjuk perusahaan/holding sebelum `DELETE`, lalu menolak dengan pesan berisi tabel dan jumlah baris.
- **Data:** skrip ops (pola EPIC-115) memindah riwayat gaji nonaktif `ERP` ke `PADDY-RETAIL`, menyimpan snapshot perusahaan dan pemetaan jurnalnya, lalu menghapus perusahaan `ERP`. Pemetaan jurnal `ERP` ikut terhapus lewat `ON DELETE CASCADE`; salinannya sudah ada di Paddy sejak EPIC-115. Skrip pembalik memulihkan semuanya dari snapshot.

**Tech Stack:** Go + pgx, Next.js fallback, PostgreSQL 16 (psql/plpgsql), Go integration test.

**Spec:** Laporan user 18-11-2026: "CV Cipta Kreasi Kreatif (as Company) ERP (code) status non aktif kenapa tidak bisa di delete? harusnya bs di delete karena sudah di assign sebagai holding kan harusnya CV Cipta Kreasi Kreatif PADDY-GROUP (code)".

Keputusan user 18-11:
- **K-1:** riwayat gaji nonaktif CV dipindah ke Paddy, lalu ERP dihapus.
- **K-2:** pesan hapus perusahaan/holding diperbaiki supaya menyebut data yang masih memakainya.

Fakta staging 18-11, sesudah EPIC-115:
- holding `PADDY-GROUP` "CV Cipta Kreasi Kreatif";
- perusahaan `ERP` nonaktif, tanpa branch, user, maupun gaji aktif;
- yang masih menunjuk ERP: `hris.employee_salary` 125 baris (nonaktif, FK RESTRICT) dan `accounting.journal_mappings` 26 baris (FK CASCADE);
- tidak ada payroll run ERP.

Akar masalah: `historyGuard` hanya memeriksa histori branch. DELETE lalu gagal karena FK RESTRICT dari riwayat gaji, dan error PostgreSQL itu tidak cocok dengan `deleteErrors` (`tidak dapat|tidak ditemukan`), sehingga muncul sebagai error umum.

Production sesudah EPIC-115 (dijadwalkan 18-11 22.00): perkiraan 127 gaji nonaktif dan 26 pemetaan, tanpa payroll run ERP.

## Global Constraints

- Kode di `wit-id-indonesia/erp-paddy`, branch `fix/epic116-hapus-perusahaan` dari `origin/development`, worktree `paddydev/.worktrees/erp-epic116`. Merge lewat PR.
- Skrip:
  - `apps/backend/database/sql/fixes/2026-11-18_hapus_perusahaan_erp.sql`;
  - `apps/backend/database/sql/fixes/2026-11-18_hapus_perusahaan_erp_balik.sql`.
- Setting `hapus.dari` (default `ERP`), `hapus.ke` (default `PADDY-RETAIL`), `hapus.mode`.
- Format keluaran NOTICE sama dengan EPIC-115.
- Pesan UI persis: `"Perusahaan ini tidak dapat dihapus karena masih dipakai: <tabel> (<n> baris)[, …] — nonaktifkan saja atau pindahkan datanya dulu"`. Untuk holding, awalannya `"Holding ini …"`.
- Eksekusi staging dan production hanya dengan izin user dan sesudah backup. Production hanya sesudah gladi di salinan production. Hindari jam 07–08, 13, 16–17, dan 21 WIB.

## Review Focus

1. **Riwayat gaji ERP dengan `effective_date` sama dengan baris Paddy karyawan yang sama** (indeks `employee_salary_entity_effective_key`): harus BLOKIR `gaji_bentrok`, bukan gagal di tengah. Diuji di Task 2.
2. **Perusahaan yang masih punya branch, user, atau gaji aktif** (EPIC-115 belum dijalankan): skrip harus BLOKIR `perusahaan_masih_dipakai`. Diuji di Task 2.
3. **Tabel lain dengan kolom `*company_id` tanpa FK** yang masih menunjuk ERP: data akan yatim sesudah delete, jadi harus BLOKIR `tabel_belum_ditinjau`. Diuji di Task 2.
4. **Pembalik:** perusahaan, pemetaan jurnal beserta barisnya, dan company gaji kembali persis. Diuji di Task 3.
5. **Hapus perusahaan lewat UI saat masih ada gaji nonaktif:** pesan jelas, HTTP 400, data tidak berubah. Diuji di Task 1.

---

### Task 1: Pesan jelas saat hapus perusahaan/holding terhalang data

**Files:**
- Modify: `apps/backend/internal/modules/configuration/settings/business.go` (`deleteBusinessEntity`, fungsi baru `restrictRefs`)
- Modify: `apps/frontend/src/lib/configuration/business-repository.ts` (`deleteBusinessEntity`, paritas)
- Test: `apps/backend/internal/modules/configuration/settings/business_integration_test.go`, `apps/frontend/src/lib/configuration/business-delete.test.ts`

**Interfaces:**
- Produces: `func restrictRefs(ctx context.Context, q database.Querier, table, id string) ([]string, error)`, returning entries like `"hris.employee_salary (125 baris)"`. It covers every single-column FK to `configuration.<table>` whose `confdeltype` is `'r'` or `'a'` and that has at least one matching row.

- [ ] **Step 1: Tulis tes yang gagal** `TestDeleteCompanyRefusedByHistory`:
  - Buat perusahaan dengan satu karyawan dan satu baris `hris.employee_salary` nonaktif.
  - `DELETE /api/settings/business/company/<id>` → 400 `"Perusahaan ini tidak dapat dihapus karena masih dipakai: hris.employee_salary (1 baris) — nonaktifkan saja atau pindahkan datanya dulu"`, dan perusahaan masih ada.
  - Perusahaan tanpa rujukan → 200 dan terhapus.
- [ ] **Step 2: Jalankan, pastikan gagal.** `go test ./internal/modules/configuration/settings/ -run TestDeleteCompanyRefusedByHistory -count=1`. Expected: FAIL (500 atau error FK).
- [ ] **Step 3: Implementasi** `restrictRefs`, lalu panggil untuk tipe `company` dan `holding` sesudah `historyGuard` dan sebelum `DELETE`. Fallback Next: tangkap error PostgreSQL `23503` saat DELETE perusahaan/holding dan lempar `"<Perusahaan|Holding> ini tidak dapat dihapus karena masih dipakai data lain — nonaktifkan saja atau pindahkan datanya dulu"`. Fallback tidak menghitung baris. Tes vitest dengan `queryOne` di-mock untuk melempar `{code: "23503"}`.
- [ ] **Step 4: Jalankan** tes Step 2, seluruh paket settings, dan vitest `src/lib/configuration`. Expected: PASS.
- [ ] **Step 5: Commit.** `fix(configuration): pesan jelas saat hapus perusahaan/holding masih dipakai (EPIC-116)`

### Task 2: Skrip hapus perusahaan ERP

**Files:**
- Create: `apps/backend/database/sql/fixes/2026-11-18_hapus_perusahaan_erp.sql`
- Test: `apps/backend/internal/modules/hris/company_delete_integration_test.go` (memakai `opsScript`, `structureUser`, dan `runStructure`)

**Interfaces:**
- Produces:
  - audit `action = 'data_restructure.company_delete'`, `entity_id` = id perusahaan, `before` = `{"perusahaan": to_jsonb(companies row), "pemetaan": [to_jsonb(mapping) + "lines": [to_jsonb(line)]], "salary": [{id, company_id}]}`;
  - `func deleteCompanySQL(t *testing.T, execute bool) string`.

Urutan skrip:
1. Validasi: `dari` dan `ke` masing-masing tepat satu perusahaan dan berbeda. Bila tidak, EXCEPTION.
2. BLOKIR:
   - `perusahaan_masih_dipakai`: `dari` masih punya branch, user, atau gaji aktif (EPIC-115 harus sudah jalan);
   - `payroll_ada`: ada `hris.payroll_runs` untuk `dari`;
   - `gaji_bentrok`: baris gaji `dari` yang (employee_id, effective_date)-nya sudah ada di `ke`;
   - `tabel_belum_ditinjau`: kolom `*company_id` mana pun, kecuali `employee_salary` dan `journal_mappings`, yang masih berisi `dari`. Pola sweep sama dengan EPIC-115, tanpa allow-list COA/tahun fiskal/jurnal: bila masih ada baris di sana, penghapusan ditolak.
3. Snapshot (lihat Interfaces).
4. Eksekusi: `UPDATE hris.employee_salary SET company_id = ke WHERE company_id = dari`, lalu `DELETE FROM configuration.companies WHERE id = dari` (pemetaan dan barisnya ikut lewat cascade).
5. INVARIAN: perusahaan `dari` tidak ada, dan 0 gaji yang menunjuk id lama.
6. Audit hanya saat eksekusi.

- [ ] **Step 1: Tulis tes yang gagal.** Fixture memakai holding acak, perusahaan S (kosong, nonaktif) dan T, 1 karyawan dengan 1 gaji S nonaktif, dan 1 pemetaan jurnal S dengan 2 baris. Tes:
  - `TestCompanyDeletePreviewChangesNothing` (savepoint seperti EPIC-115);
  - `TestCompanyDeleteMovesHistoryAndDeletes`: S hilang, gaji di T, pemetaan S dan barisnya hilang, 1 audit;
  - `TestCompanyDeleteRefuses`, berupa tabel subtes untuk `perusahaan_masih_dipakai` (branch di S), `payroll_ada`, `gaji_bentrok` (gaji T dengan tanggal sama), dan `tabel_belum_ditinjau` (`configuration.company_appearance` milik S).
- [ ] **Step 2: Jalankan, pastikan gagal.** `go test ./internal/modules/hris/ -run 'CompanyDelete' -count=1`. Expected: FAIL (file belum ada).
- [ ] **Step 3: Tulis skrip**, dengan header dan pola yang sama seperti skrip EPIC-115.
- [ ] **Step 4: Jalankan** tes Step 2 dan `-run 'BranchCompanyMove|BusinessStructure|CompanyDelete'`. Expected: PASS.
- [ ] **Step 5: Commit.** `chore(ops): skrip hapus perusahaan ERP sesudah pindah (EPIC-116)`

### Task 3: Skrip pembalik

**Files:**
- Create: `apps/backend/database/sql/fixes/2026-11-18_hapus_perusahaan_erp_balik.sql`
- Modify: `company_delete_integration_test.go`

Pembalik membaca snapshot `data_restructure.company_delete` terbaru yang belum dibalik, lalu:
1. memasukkan kembali baris perusahaan dengan `jsonb_populate_record`;
2. memasukkan kembali pemetaan beserta barisnya dengan id yang sama;
3. mengembalikan `company_id` gaji yang masih di `ke`;
4. mencatat audit `data_restructure.company_delete_balik`.

- [ ] **Step 1: Tulis tes yang gagal** `TestCompanyDeleteRevertRestores`: md5 (perusahaan, pemetaan dan barisnya, gaji) sesudah pembalik sama dengan sebelum hapus. Pembalik kedua: 0 baris.
- [ ] **Step 2: Jalankan, pastikan gagal.** Expected: FAIL (file belum ada).
- [ ] **Step 3: Tulis pembalik.**
- [ ] **Step 4: Jalankan** `go test ./internal/modules/hris/... ./internal/modules/configuration/...`. Expected: PASS.
- [ ] **Step 5: Commit.** `chore(ops): pembalik hapus perusahaan ERP (EPIC-116)`

### Task 4: Review, PR, staging, gladi, production

- [ ] Review akhir oleh reviewer segar, PR ke `development`, merge sesudah CI hijau.
- [ ] Staging (izin user):
  - backup;
  - pratinjau: 0 BLOKIR, angka 125 gaji / 26 pemetaan;
  - eksekusi, lalu cek: ERP hilang dari Struktur Bisnis, `/api/ready` 200, dan HR tidak melihat error di halaman gaji karyawan yang riwayatnya pindah.
- [ ] Gladi di salinan production **sesudah** EPIC-115 production: pratinjau, eksekusi, pembalik, md5 sama.
- [ ] Production (izin user, jam yang diizinkan): backup, pratinjau, eksekusi, cek.
- [ ] Catat di `docs/epics/EPIC-116-hapus-perusahaan-erp.md` dan perbarui struktur di EPIC-106/115 serta memori.
