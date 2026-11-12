# Struktur Bisnis Paddy Group Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Staging (lalu production, dengan izin) mengikuti struktur Paddy Group → {CV Cipta Kreasi Kreatif (HQ ★ + Warehouse), Paddy (semua toko + Pudak + Salam)} tanpa kehilangan riwayat; holding PROLOGE nonaktif.

**Architecture:**
- **Satu skrip SQL ops** di erp-paddy `apps/backend/database/sql/fixes/` dengan pola skrip gabung:
  - pratinjau bawaan (ROLLBACK), `-v eksekusi=1`;
  - NOTICE `STRUKTUR`/`PINDAH`/`RINCI`/`HISTORY`/`PERHATIAN`/`INVARIAN`/`BLOKIR`/`HASIL`;
  - snapshot nilai lama di `audit.audit_log`, yang dibaca oleh skrip pembalik.
- **Kode yang bergantung pada holding** ikut diperbaiki: seeder, skrip scope, dan pemilih scope UI.
- **Eksekusi manual** lewat psql setelah backup dan izin user. Deploy tidak menjalankan skrip.

**Tech Stack:** PostgreSQL 16 (plpgsql), Go 1.26 integration tests (pgx), Next.js/Vitest, Node `node:test`.

**Spec:** `docs/epics/EPIC-106-struktur-bisnis-paddy-group.md` (R1–R14) dan brief eksekusi user 12-11-2026.

## Global Constraints

- **Izin:** tidak ada perubahan data staging/production tanpa izin eksplisit user. Production juga menghindari jam 07–08, 13, 16–17, dan 21 WIB, dengan backup dulu dan tanpa sesi/akun sementara.
- **Kode:** `PADDY`, `ERP`, `PADDY-RETAIL`, id cabang tetap. Nama baru persis `CV Cipta Kreasi Kreatif`.
- **Lokasi kerja erp-paddy:** worktree `.worktrees/erp-epic106` (bukan `~/WORK/erp-paddy`, yang merupakan checkout deploy staging), branch `fix/epic106-struktur-bisnis` dari `development`, PR ke `development`.
- **Tes Go:** jalankan di DB uji baru yang dimigrasi (`paddydev-sdd-testdb` port 55499, DB `e106_test`).
- **Backup:** staging ke `/mnt/hdd1/backup/paddydev/staging-before-struktur-bisnis-<stamp>.dump` (`umask 077` di subshell).

## Review Focus

1. **Staf HQ yang cabang default-nya toko pindahan** tetap di ERP dengan cabang default = cabang penempatan utama. Test: `TestBusinessStructureMovesToTarget` (hq user), `TestBusinessStructureKeepsUsersInScope`.
2. **Super admin tanpa holding dan user toko lain** tidak tersentuh. Test: `TestBusinessStructureMovesToTarget` (super admin, retail user).
3. **Payroll yang belum final memuat karyawan yang gajinya pindah** → BLOKIR. Gaji tidak aktif tetap di ERP. Test: `TestBusinessStructureRefusesWhatNeedsReview/payroll_belum_final`, `…MovesToTarget` (ended salary).
4. **Dijalankan dua kali / dibalik dua kali** → 0 baris, `updated_at` tidak berubah, dan pembalik mengembalikan baris yang sama. Test: `TestBusinessStructureSecondRunChangesNothing`, `TestBusinessStructureRevertRestoresTheSameRows`.
5. **Seeder dijalankan ulang setelah perubahan** tidak membuat ERP kedua, tidak menimpa nama Paddy Group, dan tidak mengaktifkan PROLOGE lagi. Test: verifikasi seeder 2× di DB uji (Task 3), `items-business-scope.test.js`.

---

### Task 1: Dampak dan gladi (read-only) ✅
- [x] Kueri read-only staging: struktur, 37 user Pudak/Salam (35 berpenempatan HQ), history per FK cabang, tabel `company_id`/`holding_id` berisi data, dan selisih gaji-penempatan.
- [x] Gladi di salinan staging (`stg_e106`): pratinjau, eksekusi, jalan kedua, pembalik, md5 identik. Angka tercatat di Automation Log epic.

### Task 2: Skrip SQL + pembalik + tes integrasi ✅
**Files:**
- `apps/backend/database/sql/fixes/2026-11-12_struktur_bisnis_paddy_group.sql`
- `apps/backend/database/sql/fixes/2026-11-12_struktur_bisnis_paddy_group_balik.sql`
- `apps/backend/internal/modules/hris/business_structure_integration_test.go`

**Interfaces:**
- Parameter `struktur.holding|holding_lama|pusat|nama_pusat|toko|kantor_pusat|cabang_pindah` (PGOPTIONS), dengan default kode staging.
- Mode `struktur.mode` diaktifkan oleh `-v eksekusi=1`.
- Audit `data_restructure.business`: `before` = snapshot, `after.pindah` = jumlah per tabel. Pembalik menulis `data_restructure.business_revert`.

- [x] Tes merah (file belum ada), lalu skrip ditulis, lalu 8/8 hijau:
  - `PreviewChangesNothing`, `MovesToTarget`, `KeepsHistoryOnBranches`, `SecondRunChangesNothing`;
  - `KeepsUsersInScope`, `PayrollCountsMovedStoresInRetail`;
  - `RefusesWhatNeedsReview` (6 kasus), `RevertRestoresTheSameRows`.

### Task 3: Kode yang bergantung pada PROLOGE/holding ✅
- [x] `apps/frontend/src/lib/configuration/business-scope.ts` `selectableScopeItems` dipakai `business-scope-picker.tsx`. Tes: `business-scope.selectable.test.ts` (RED → GREEN).
- [x] `apps/backend/database/scripts/items-business-scope.js`: holding default PADDY, filter holding hanya bila `SEED_HOLDING_CODE` diisi. Tes: `items-business-scope.test.js` (`node --test`, DB uji; RED → GREEN).
- [x] Seeder paddydev `backend/database/seeders/business-hierarchy.js` membangun struktur target. RED: di DB berstruktur baru ia menimpa nama Paddy Group. GREEN: 2× jalan di DB baru maupun lama menghasilkan PADDY > ERP "CV Cipta Kreasi Kreatif", PROLOGE nonaktif.
- [x] Peringatan "JANGAN DIJALANKAN" di `2026-11-11_gabung_paddy_ke_kantor_pusat.sql`.
- Tidak diubah, karena aman:
  - login (`identity/service.go` hanya memeriksa password dan banned);
  - `scope.Load` (tanpa join holding);
  - halaman Struktur Bisnis (memuat semua holding dan menampilkan badge Nonaktif);
  - `paddy-branding.js` (hanya rename nama PROLOGE, no-op);
  - migrasi (R13).

### Task 4: Gate, review, PR
- [ ] gofmt, go vet, `go test ./...` di DB uji 2× berturut-turut, tsc, eslint, vitest penuh.
- [ ] Review akhir oleh reviewer terpisah. Temuan Critical/Important diperbaiki dengan tes merah lalu hijau, kemudian gate diulang.
- [ ] Push, PR ke `development`, CI hijau, merge (deploy staging otomatis, skrip tidak ikut jalan).

### Task 5: Staging
- [ ] Backup: `(umask 077; docker exec paddydev-db pg_dump -U paddydev -d paddydev -Fc > /mnt/hdd1/backup/paddydev/staging-before-struktur-bisnis-<stamp>.dump)`.
- [ ] Pratinjau dari checkout yang sudah merge, lalu laporkan angkanya ke user dan **tunggu izin**.
- [ ] Eksekusi, lalu jalankan pratinjau lagi (semua `PINDAH … 0`, `INVARIAN … OK`).
- [ ] Verifikasi:
  - halaman Struktur Bisnis;
  - jadwal, penempatan, karyawan, payroll, POS, dan gudang Pudak/Salam;
  - login super admin dan HR;
  - `/api/ready` 200.
- [ ] Bila keliru: jalankan pembalik atau restore backup, perbaiki, ulangi.

### Task 6: Production (menunggu izin)
- [ ] Gladi di salinan DB prod terbaru (pratinjau, eksekusi, jalan kedua, pembalik), lalu laporkan ke user.
- [ ] Eksekusi hanya dengan izin eksplisit, di jam sepi, setelah backup prod; caranya saat rilis prod pertama dari erp-paddy atau lewat psql di VPS, sesuai keputusan user.

### Task 7: Catatan
- [ ] Automation Log epic, registry, dan memori diperbarui. Merge dokumen di paddydev, lalu `bash tools/sync-docs-erp-paddy.sh`.
