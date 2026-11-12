# Struktur Bisnis Paddy Group Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Staging mengikuti struktur Paddy Group → {CV Cipta Kreasi Kreatif (HQ + Warehouse), Paddy (semua toko)} tanpa kehilangan riwayat, setelah orang yang terdampak dikonfirmasi.

**Architecture:**
- **Langkah 1 — laporan read-only.** Daftar orang dan data di Pudak/Salam untuk dikonfirmasi user.
- **Langkah 2 — satu skrip SQL** di erp-paddy (`apps/backend/database/sql/fixes/`) dengan pola skrip gabung yang sudah ada: pratinjau bawaan (ROLLBACK), `-v eksekusi=1`, dan NOTICE terstruktur. Skrip disertai tes integrasi Go.
- **Langkah 3 — eksekusi di staging** hanya setelah backup, pratinjau, dan izin user.

**Tech Stack:** PostgreSQL 16 (plpgsql DO block, psql), Go 1.26 integration tests (pgx), erp-paddy monorepo (`apps/backend`).

**Spec:** `docs/epics/EPIC-106-struktur-bisnis-paddy-group.md` (R1–R8), di repo paddydev (dokumen). Kode di `wit-id-indonesia/erp-paddy`.

## Global Constraints

- **Tidak ada perubahan data staging atau production tanpa izin eksplisit user** (R8). Production di luar rencana ini.
- **Kode tetap:** kode holding/perusahaan (`PADDY`, `ERP`, `PADDY-RETAIL`) dan semua ID cabang tidak berubah (R1). Nama baru persis `CV Cipta Kreasi Kreatif`.
- **Kode di erp-paddy:** branch dari `development` di worktree terpisah (bukan `~/WORK/erp-paddy`, karena itu checkout deploy staging), lalu PR ke `development`.
- **Dokumen di paddydev:** epic dan log di repo paddydev, disinkron dengan `tools/sync-docs-erp-paddy.sh`.
- **Tes Go:** jalankan di DB uji yang baru dimigrasi (`paddydev-sdd-testdb`, port 55499) dengan migrasi erp-paddy.
- **Backup staging** ke `/mnt/hdd1/backup/paddydev/staging-before-e106-<stamp>.dump`. Deploy erp-paddy tidak membuat backup.

## Review Focus

1. **Staf kantor pusat yang cabang default-nya Pudak/Salam** tidak boleh ikut pindah ke perusahaan Paddy tanpa konfirmasi. User yang ada di `struktur.hq_users` tetap di ERP dengan cabang `ERP-BANDUNG`. Eksekusi tanpa `struktur.hq_users` padahal ada user ERP di Pudak/Salam → BLOKIR (pratinjau tetap mencetak daftarnya). Untuk menyatakan "semua ikut toko", isi dengan `-`. Test: T2 `TestBusinessStructureKeepsConfirmedHQUsers`, `TestBusinessStructureBlocksUnconfirmedUsers`.
2. **4 user tanpa perusahaan (super admin)** tidak tersentuh. Test: T2 `TestBusinessStructureLeavesUnscopedUsers`.
3. **Payroll run `draft` yang memuat karyawan pindah perusahaan** memblokir eksekusi; run `approved/paid` tidak berubah. Test: T2 `TestBusinessStructureBlocksDraftPayroll`.
4. **Holding `PROLOGE` yang masih dirujuk tabel lain** tidak dihapus paksa: BLOKIR dengan nama tabel. Test: T2 `TestBusinessStructureBlocksReferencedHolding`.
5. **Skrip dijalankan dua kali** → jalan kedua mengubah 0 baris dan invarian tetap lulus. Riwayat absensi, penempatan, dan jadwal sama jumlahnya sebelum dan sesudah. Test: T2 `TestBusinessStructureIdempotentAndKeepsHistory`.

---

### Task 1: Laporan dampak (read-only) dan konfirmasi user

**Files:**
- Create (paddydev): `docs/hris/struktur-bisnis-20261112/dampak-pudak-salam.md` dan `.csv` (di `.share/` bila berisi data pribadi).

**Interfaces:**
- Produces: daftar konfirmasi `hq_users.csv` (`user_id, email, nama, keputusan` = `HQ` | `TOKO`), dipakai T2/T4.

- [ ] **Step 1:** Jalankan kueri read-only (`SET default_transaction_read_only = on`) di DB staging untuk:
  - 37 user ERP dengan `branch_id` Pudak/Salam (nama, email, role, jabatan karyawan, penempatan utama aktif);
  - 15 baris gaji ERP milik karyawan yang ditempatkan di sana (berlaku/tidak);
  - pemilik 387 absensi 30 hari terakhir (nama, jumlah hari, penempatan utamanya);
  - 2 gudang di Pudak/Salam (nama, aktif);
  - payroll run ERP `draft` (periode, jumlah karyawan, apakah memuat orang di atas);
  - holding `PROLOGE`: semua tabel yang merujuknya, termasuk `users.holding_id`.
- [ ] **Step 2:** Tulis ringkasan dan CSV-nya. Kirim ke user dengan pertanyaan per orang: tetap kantor pusat (HQ) atau ikut toko (TOKO).
- [ ] **Step 3: Stop.** Lanjut ke T4 hanya setelah user mengembalikan keputusan. T2 dan T3 boleh dikerjakan paralel karena tidak mengubah data.

### Task 2: Skrip SQL struktur + tes integrasi (erp-paddy)

**Files:**
- Create: `apps/backend/database/sql/fixes/2026-11-12_struktur_bisnis_paddy_group.sql`
- Create: `apps/backend/internal/modules/configuration/settings/business_structure_integration_test.go`. Pola: `hris/company_merge_integration_test.go`, yang menjalankan file SQL lewat psql/pgx di DB uji.

**Interfaces:**
- **Pemakaian skrip:**
  - `psql "$URL" -v ON_ERROR_STOP=1 -f …sql` untuk pratinjau (ROLLBACK);
  - `-v eksekusi=1` untuk eksekusi;
  - `PGOPTIONS='-c struktur.hq_users=<uuid>,<uuid>'` untuk user yang dikonfirmasi HQ (T1), dibaca dengan `current_setting('struktur.hq_users', true)` seperti `gabung.dari` di skrip gabung. Cabang default mereka dipindah ke `ERP-BANDUNG` sebelum aturan "orang mengikuti cabang".
- **Urutan di dalam satu transaksi:**
  1. user `hq_users`: `branch_id` → cabang `ERP-BANDUNG`;
  2. cabang `ODOO-PADDY-PUDAK` dan `ODOO-PADDY-SALAM`: `company_id` → PADDY-RETAIL;
  3. perusahaan `ERP`: `name` → `CV Cipta Kreasi Kreatif`, `holding_id` → holding `PADDY`;
  4. `configuration.users` dengan perusahaan: `company_id` = perusahaan dari `branch_id` (bila `branch_id` terisi), dan `holding_id` = holding perusahaannya;
  5. `hris.employee_salary` yang masih berlaku: `company_id` = perusahaan cabang penempatan utama aktif;
  6. `configuration.branches.is_primary` = true untuk `ERP-BANDUNG`;
  7. hapus holding `PROLOGE` bila tidak dirujuk.
- **BLOKIR** (eksekusi ditolak, pratinjau tetap mencetak):
  - payroll run `draft` memuat karyawan yang `company_id` gajinya berubah;
  - `PROLOGE` masih dirujuk;
  - ada id di `hq_users` yang tidak ada atau bukan user ERP;
  - `struktur.hq_users` kosong padahal ada user ERP di Pudak/Salam (isi `-` bila semua ikut toko).
- **NOTICE:**
  - `PINDAH <tabel> <n>`;
  - `HISTORY <tabel> <sebelum> <sesudah>` untuk attendance, employee_placements, work_schedules (bila ada), warehouses;
  - `INVARIAN <nama> OK|GAGAL <n>`;
  - `HASIL PRATINJAU|EKSEKUSI <ringkasan>`.

- [ ] **Step 1: Failing tests** (fixture minimal di DB uji: dua holding, dua perusahaan, empat cabang, user, gaji, payroll run, absensi):
  - `TestBusinessStructurePreviewChangesNothing`: pratinjau → semua tabel sama persis.
  - `TestBusinessStructureMovesToTarget`: eksekusi menghasilkan:
    - ERP bernama `CV Cipta Kreasi Kreatif` di holding PADDY;
    - Pudak/Salam di PADDY-RETAIL;
    - user dan gaji mengikuti cabang;
    - `ERP-BANDUNG` `is_primary`;
    - `PROLOGE` hilang.
  - `TestBusinessStructureKeepsConfirmedHQUsers`: user di `hq_users` tetap ERP dengan cabang `ERP-BANDUNG`.
  - `TestBusinessStructureBlocksUnconfirmedUsers`: eksekusi tanpa `struktur.hq_users` saat ada user ERP di Pudak → BLOKIR. Dengan `-` → lanjut dan user ikut PADDY-RETAIL.
  - `TestBusinessStructureLeavesUnscopedUsers`: user tanpa `company_id` tidak berubah.
  - `TestBusinessStructureBlocksDraftPayroll`: draft run memuat karyawan pindah → BLOKIR, tidak ada yang berubah. Run `approved` tidak menghalangi dan tidak berubah.
  - `TestBusinessStructureBlocksReferencedHolding`: baris lain merujuk `PROLOGE` → BLOKIR dengan nama tabelnya.
  - `TestBusinessStructureIdempotentAndKeepsHistory`: eksekusi dua kali → jalan kedua `PINDAH … 0`, dan jumlah absensi/penempatan/gudang sama.
- [ ] **Step 2:** Jalankan → FAIL (file belum ada).
- [ ] **Step 3:** Tulis skrip.
- [ ] **Step 4:** `go test ./internal/modules/configuration/settings -run BusinessStructure -count=1` → PASS. Lalu jalankan suite backend penuh di DB baru.
- [ ] **Step 5: Commit** `fix(settings): skrip struktur bisnis Paddy Group (EPIC-106)`

### Task 3: PR ke erp-paddy `development`

- [ ] **Step 1:** Push branch, buka PR ber-base `development`, dan tunggu CI hijau. Merge memicu deploy staging otomatis, tapi skrip tidak ikut dijalankan.
- [ ] **Step 2:** Review seluruh branch oleh reviewer terpisah. Critical/Important diperbaiki test-first sebelum merge.

### Task 4: Staging — backup, pratinjau, izin, eksekusi, QA

- [ ] **Step 1:** Syarat: T1 sudah dikonfirmasi user, dan T3 sudah merge.
- [ ] **Step 2:** `df -h /`, lalu backup `(umask 077; docker exec paddydev-db pg_dump -Fc …) > /mnt/hdd1/backup/paddydev/staging-before-e106-<stamp>.dump`.
- [ ] **Step 3:** Pratinjau dengan `PGOPTIONS='-c struktur.hq_users=…'` dari T1. Kirim output NOTICE lengkap ke user dan **tunggu izin**.
- [ ] **Step 4:** Eksekusi. Jalankan pratinjau lagi dan pastikan semua `PINDAH … 0` dan `INVARIAN … OK`.
- [ ] **Step 5: QA staging** (sesi sementara, data uji dihapus):
  - Struktur Bisnis menampilkan Paddy Group → CV Cipta Kreasi Kreatif (HQ ★, Warehouse) dan Paddy (toko);
  - HRIS: daftar karyawan per perusahaan, jadwal toko Pudak/Salam, penempatan, absensi hari ini, dan pratinjau payroll per perusahaan;
  - user HQ dan user toko login: cakupan menu dan data sesuai perusahaannya;
  - POS/gudang Pudak/Salam masih bisa dibuka.

### Task 5: Catatan

- [ ] Update Automation Log epic, registry, dan memori. Merge dokumen di paddydev, lalu jalankan `bash tools/sync-docs-erp-paddy.sh`.
