# Pindah Toko Karyawan Massal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR memindah banyak karyawan ke banyak toko sekaligus lewat template (dan lewat "Pindahkan ke toko"), sehingga penempatan utama, akun (cabang, perusahaan, holding, outlet), dan gaji lintas perusahaan ikut dalam satu aksi, dengan histori tetap.

**Architecture:**
- **Inti bersama:** satu fungsi transaksi per karyawan, `moveEmployeeTx`, yang mengakhiri penempatan lama, membuat penempatan utama baru, lalu menyinkronkan akun dan gaji (atau mengantre sinkron bila tanggalnya di masa depan).
- **Pemakai inti:**
  - "Pindahkan ke toko" (EPIC-070) dan template baru (pola `branch_code_import.go` EPIC-100: pratinjau rollback, fingerprint, file error);
  - job harian (pola `ScheduledResignLoop`) untuk antrean masa depan.

**Tech Stack:** Go 1.26 (pgx, excelize seperti `branch_code_import.go`), PostgreSQL 16, Next.js + React Query + Vitest (fitur `apps/frontend/src/features/hris/placements`).

**Spec:** `docs/epics/EPIC-107-pindah-toko-massal.md` (R1–R10), repo paddydev. Kode di `wit-id-indonesia/erp-paddy`.

## Global Constraints

- **Branch dan PR:** branch `feat/epic107-pindah-toko-massal` dari `development` di worktree terpisah (bukan `~/WORK/erp-paddy`); PR ke `development`.
- **Akses:** `placementManagers`; simpan butuh aksi `update`. Template tidak untuk delegasi toko.
- **Pesan:** pesan UI/error dalam Bahasa Indonesia, persis seperti di spec:
  - "Toko <kode> belum punya outlet aktif";
  - "gaji ganda";
  - "Saya sudah memeriksa peringatan";
  - "— pindahkan dulu lewat Pindah Toko Massal".
- **Batas dan tanggal:** batas 1.000 baris berisi per file. Tanggal memakai WIB (`domain.TodayWIB`).
- **Tidak menghapus apa pun (R10):** penempatan diakhiri, outlet dinonaktifkan, gaji dipindah perusahaannya.
- **Tes Go** jalan di DB uji yang baru dimigrasi (`paddydev-sdd-testdb`, port 55499), dua kali berturut-turut sebelum PR.

## Review Focus

1. **Akun scope perusahaan/holding atau super admin** tidak boleh berubah; yang muncul hanya PERHATIAN. Test: T1 `TestPlanAccountSyncLeavesNonBranchScope`, T2 `TestSyncEmployeeAccountCompanyScope`.
2. **Pindah dua kali sebelum tanggal berlaku** → antrean lama dibatal, hanya yang terakhir dijalankan. Test: T4 `TestAccountSyncQueueKeepsLatestMove`.
3. **Baris error di tengah file/pilihan** tidak boleh meninggalkan penempatan setengah jadi untuk karyawan itu; baris lain tetap jalan saat pratinjau, dan simpan ditolak. Test: T3 `TestBulkMoveRowErrorLeavesEmployeeUntouched`, T5 `TestStoreMoveImportRejectsCommitWithErrors`.
4. **File berubah, atau karyawan dipindah orang lain, antara pratinjau dan simpan** → 409 dan tidak ada yang tertulis. Test: T5 `TestStoreMoveImportFingerprintMismatch`.
5. **Toko ditutup setelah semua karyawannya dipindah** → Nonaktif di Kode & Status Toko lolos. Test: T5 `TestStoreMoveThenDeactivateBranch`.

---

### Task 1: Keputusan sinkron akun (domain)

**Files:**
- Create: `apps/backend/internal/modules/hris/domain/account_sync.go`
- Test: `apps/backend/internal/modules/hris/domain/account_sync_test.go`

**Interfaces:**
- Produces:
  ```go
  type AccountState struct {
      UserID, Role, Scope   string            // Scope = business_scope ("" bila kosong)
      BranchID, CompanyID   *string
      DefaultWarehouseID    *string
      ActiveOutletBranch    map[string]string // warehouse id outlet aktif -> branch id
  }
  type BranchTarget struct {
      BranchID, BranchCode, CompanyID, HoldingID string
      DefaultOutletID                            *string // nil = toko tanpa outlet aktif
  }
  type AccountSyncDecision struct {
      Outcome           string   // AccountUpdate | AccountUnchanged | AccountSkipped | AccountError
      Reason            string   // isi untuk Skipped (PERHATIAN) dan Error
      DeactivateOutlets []string // outlet aktif di toko lama
      CompanyChanged    bool
  }
  func PlanAccountSync(acc *AccountState, oldBranchID string, t BranchTarget) AccountSyncDecision
  ```
  Konstanta: `AccountUpdate = "update"`, `AccountUnchanged = "unchanged"`, `AccountSkipped = "skipped"`, `AccountError = "error"`.

- [ ] **Step 1: Tes gagal** (`account_sync_test.go`), tabel kasus:
  - `TestPlanAccountSyncNoAccount`: `acc == nil` → Skipped, Reason `"Karyawan tanpa akun"`.
  - `TestPlanAccountSyncLeavesNonBranchScope`:
    - Scope `"company"` → Skipped, Reason `"akun scope perusahaan — ubah di Pengguna bila perlu"`;
    - Role `"super_admin"` → Skipped.
  - `TestPlanAccountSyncNoOutletAtTarget`: `DefaultOutletID == nil` → Error, Reason `"Toko PDK belum punya outlet aktif"`.
  - `TestPlanAccountSyncMoves`: BranchID lama, outlet aktif `w1` di toko lama dan `w9` di toko lain → Update, `DeactivateOutlets == ["w1"]`, dan `CompanyChanged` true bila CompanyID berbeda.
  - `TestPlanAccountSyncAlreadyThere`: BranchID dan CompanyID sudah sama dengan target, dan outlet default = `DefaultOutletID` → Unchanged.
- [ ] **Step 2:** `go test ./internal/modules/hris/domain -run PlanAccountSync` → FAIL (undefined).
- [ ] **Step 3:** Implementasikan `PlanAccountSync`. Urutan aturan: nil, lalu super_admin, lalu scope ≠ branch, lalu outlet target, lalu unchanged, lalu update.
- [ ] **Step 4:** Jalankan lagi → PASS.
- [ ] **Step 5: Commit** `feat(hris): keputusan sinkron akun saat pindah toko (EPIC-107)`

### Task 2: Sinkron akun + gaji dalam transaksi

**Files:**
- Create: `apps/backend/internal/modules/hris/account_sync.go`
- Test: `apps/backend/internal/modules/hris/account_sync_integration_test.go`

**Interfaces:**
- Consumes: T1 `PlanAccountSync`, `BranchTarget`, `AccountState`.
- Produces:
  ```go
  type accountSyncOutcome struct {
      Account     domain.AccountSyncDecision
      SalaryMoved int
      Warnings    []string // PERHATIAN per baris
  }
  // rowReason != "" = error per baris (tidak ada yang ditulis); err = gagal fatal.
  func syncEmployeeAccount(ctx context.Context, q database.Querier, actor *auth.User,
      employeeID, oldBranchID, newBranchID string) (out accountSyncOutcome, rowReason string, err error)
  func loadBranchTarget(ctx context.Context, q database.Querier, branchID string) (domain.BranchTarget, error)
  ```
  - Outlet default target: `configuration.warehouses` aktif di toko itu, `ORDER BY is_default DESC, created_at`.
  - Penulisan:
    - `configuration.users` (`branch_id`, `company_id`, `holding_id`, `default_warehouse_id`);
    - `configuration.user_warehouses` (insert/aktifkan outlet baru, nonaktifkan `DeactivateOutlets`);
    - `hris.employee_salary.company_id` (gaji aktif perusahaan lama, hanya bila `CompanyChanged`).
  - Audit (`writeEmployeeAudit`): `user.branch_sync` dan `salary.company_move`.
  - Aturan gaji R4. Error per baris:
    - `"gaji ganda"`;
    - `"payroll <KODE> <tahun>-<bulan> (<status>) memuat karyawan ini — selesaikan dulu"`;
    - `"payroll <KODE_BARU> <tahun>-<bulan> belum dibayar — periode ganda"`.

    PERHATIAN: `"payroll <KODE> <tahun>-<bulan> draft — hitung ulang"`.

- [ ] **Step 1: Tes gagal** (fixture: dua perusahaan; toko A di perusahaan P, toko B di P, toko C di perusahaan R; outlet per toko; karyawan + akun scope branch di A, gaji aktif P):
  - `TestSyncEmployeeAccountSameCompany`:
    - A→B: akun `branch_id` B, outlet default = outlet B, outlet A `is_active=false`;
    - gaji tetap P; `SalaryMoved == 0`; audit `user.branch_sync` ada.
  - `TestSyncEmployeeAccountCrossCompany`:
    - A→C: akun `company_id` R dan `holding_id` = holding R;
    - gaji aktif pindah ke R, gaji tidak aktif tetap P; audit `salary.company_move` ada.
  - `TestSyncEmployeeAccountCompanyScope`: akun scope `company` → tidak berubah; Warnings berisi "akun scope perusahaan".
  - `TestSyncEmployeeAccountRowErrors`, subtes:
    - `gaji_ganda`: sudah ada gaji aktif di R;
    - `payroll_completed`: run P `completed` memuat karyawan;
    - `periode_ganda`: run P `paid` 2026-11 memuat karyawan dan run R 2026-11 `draft`;
    - `tanpa_outlet`: C tanpa outlet.

    Tiap subtes: `rowReason` memuat teks spec, dan state akun/gaji sama persis dengan sebelum.
  - `TestSyncEmployeeAccountDraftWarns`: run P `draft` memuat karyawan → sukses dengan Warnings "hitung ulang".
- [ ] **Step 2:** `go test ./internal/modules/hris -run SyncEmployeeAccount -count=1` → FAIL.
- [ ] **Step 3:** Implementasikan. Semua cek error per baris dijalankan **sebelum** menulis apa pun.
- [ ] **Step 4:** Jalankan lagi → PASS.
- [ ] **Step 5: Commit** `feat(hris): sinkron akun dan gaji saat pindah toko (EPIC-107)`

### Task 3: "Pindahkan ke toko" ikut sinkron (refaktor inti per karyawan)

**Files:**
- Modify: `apps/backend/internal/modules/hris/placements.go` (loop `BulkPlacementPlan` mode `move`, sekitar baris 668–780)
- Create: `apps/backend/internal/modules/hris/employee_move.go`
- Test: `apps/backend/internal/modules/hris/employee_move_integration_test.go`
- Modify: `apps/frontend/src/features/hris/placements/components/bulk-placement-dialog.tsx` (+ test) — kolom "Akun" dan "Gaji" di tabel pratinjau
- Modify: `apps/frontend/src/features/hris/placements/types.ts`

**Interfaces:**
- Consumes: T2 `syncEmployeeAccount`; T4 `enqueueAccountSync` (sementara: bila `start > today`, kembalikan Warnings `"akun ikut pada <start>"` tanpa menulis antrean; T4 mengisinya).
- Produces:
  ```go
  type employeeMove struct {
      EmployeeID, BranchID, Start string
      Notes                       *string
      Action                      string // "placement.bulk_move" | "placement.template_move"
  }
  type employeeMoveResult struct {
      Outcome, Reason       string // domain.PlacementMoved | PlacementSkipped | "error"
      FromBranch, ToBranch  string
      Account               domain.AccountSyncDecision
      SalaryMoved           int
      Warnings              []string
  }
  func moveEmployeeTx(ctx context.Context, r Repository, actor *auth.User, mv employeeMove, today string) (employeeMoveResult, error)
  ```
  - Isi `moveEmployeeTx` adalah isi loop move yang ada, ditambah `syncEmployeeAccount`.
  - Setiap karyawan dibungkus `SAVEPOINT emp_move`. Bila ada `rowReason`, `ROLLBACK TO SAVEPOINT` lalu `Outcome = "error"`.
- JSON hasil per baris `bulkPlacementResult` ditambah `account` (`{outcome, reason, branch_from, branch_to, company_from, company_to}`), `salary_moved`, dan `warnings`.

- [ ] **Step 1: Tes gagal:**
  - `TestBulkMoveSyncsAccount`: POST bulk move 2 karyawan A→C (lintas perusahaan, `step=commit`) → kedua akun di R, gaji di R, penempatan A berakhir H-1.
  - `TestBulkMoveRowErrorLeavesEmployeeUntouched`: satu karyawan punya gaji ganda di R → baris itu `error` dan penempatannya masih di A (tanpa end_date); karyawan lain pindah.
  - `TestBulkMovePreviewWritesNothing`: `step=preview` → hasil memuat `account.company_to`, dan DB tidak berubah.
  - Vitest `bulk-placement-dialog.test.tsx`: pratinjau menampilkan "Akun: A → C" dan "Gaji pindah ke R"; baris error menampilkan alasan.
- [ ] **Step 2:** Jalankan (`go test ./internal/modules/hris -run BulkMove -count=1`; `pnpm --dir apps/frontend exec vitest run src/features/hris/placements`) → FAIL.
- [ ] **Step 3:** Ekstrak `moveEmployeeTx` dan pakai di `BulkPlacementPlan`. Lalu kolom pratinjau di dialog.
- [ ] **Step 4:** Tes T3 dan semua tes `placements` yang sudah ada → PASS.
- [ ] **Step 5: Commit** `feat(hris): pindahkan ke toko ikut memperbarui akun dan gaji (EPIC-107)`

### Task 4: Antrean + job harian untuk tanggal masa depan

**Files:**
- Create: `apps/backend/database/migrations/deltas/20261206090000_account_branch_syncs.sql`
- Create: `apps/backend/internal/modules/hris/account_sync_queue.go`
- Modify: `apps/backend/internal/modules/hris/module.go` (jalankan loop di samping `ScheduledResignLoop`)
- Modify: `apps/backend/internal/modules/hris/employee_move.go` (start > today → enqueue)
- Test: `apps/backend/internal/modules/hris/account_sync_queue_integration_test.go`

**Interfaces:**
- **Tabel** `hris.account_branch_syncs` (pola `scheduled_resignations`): `id`, `employee_id` FK, `placement_id` FK `hris.employee_placements` ON DELETE CASCADE, `old_branch_id`, `new_branch_id`, `effective_date`, `created_by`, `created_at`, `applied_at`, `cancelled_at`, `last_error`.
  - Indeks unik "satu pending per karyawan", dan indeks `due`.
- **Fungsi:**
  ```go
  func enqueueAccountSync(ctx context.Context, q database.Querier, actor *auth.User, employeeID, placementID, oldBranchID, newBranchID, date string) error // membatalkan pending lama
  func (s *Service) RunAccountBranchSyncs(ctx context.Context, now time.Time) (applied, failed int, err error)
  func (s *Service) AccountBranchSyncLoop(ctx context.Context, every time.Duration)
  const AccountBranchSyncEvery = time.Hour
  ```
- **Aturan job:**
  - Proses yang `effective_date <= today` dengan `FOR UPDATE SKIP LOCKED`, satu transaksi per baris.
  - Penempatan yang sudah berakhir sebelum `effective_date` → `cancelled_at` dengan alasan.
  - `rowReason` dari `syncEmployeeAccount` → `last_error` diisi, tetap pending, dan dicoba lagi tiap tick.

- [ ] **Step 1: Tes gagal:**
  - `TestAccountSyncQueueAppliesOnDate`: pindah dengan start = besok → akun belum berubah. `RunAccountBranchSyncs` dengan now = besok 01:00 WIB → akun dan gaji berubah, `applied_at` terisi.
  - `TestAccountSyncQueueKeepsLatestMove`: pindah A→B (besok), lalu A→C (lusa) → hanya satu pending (C); B `cancelled_at` terisi.
  - `TestAccountSyncQueueRetriesRowError`: gaji ganda saat jatuh tempo → `last_error` berisi "gaji ganda" dan baris tetap pending.
- [ ] **Step 2:** FAIL (tabel belum ada). Terapkan migrasi ke DB uji (`go run ./cmd/migrate -apply`).
- [ ] **Step 3:** Implementasikan. Mendaftarkan migrasi tidak perlu: filenya dibaca otomatis menurut nama.
- [ ] **Step 4:** PASS. Tes `ScheduledResign` yang sudah ada tetap PASS.
- [ ] **Step 5: Commit** `feat(hris): antrean sinkron akun untuk pindah toko bertanggal (EPIC-107)`

### Task 5: Template Pindah Toko Massal

**Files:**
- Create: `apps/backend/internal/modules/hris/domain/store_move_upload.go` (+ `_test.go`)
- Create: `apps/backend/internal/modules/hris/store_move_import.go`
- Modify: `apps/backend/internal/modules/hris/routes.go`
- Modify: `apps/backend/internal/modules/hris/domain/branch_code_upload.go:77` (pesan R9)
- Test: `apps/backend/internal/modules/hris/store_move_import_integration_test.go`
- Regenerate: `apps/frontend/src/lib/go-routes.generated.json` (`make -C apps/backend generate`)

**Interfaces:**
- Consumes: T3 `moveEmployeeTx`; pola `branch_code_import.go` (`exportFile`, fingerprint, `error_file`).
- **Routes:**
  - `GET /api/hris/employee-placements/move-template?branch_id=<uuid opsional>` → xlsx, sheet `Pindah Toko`, kolom urut:
    - `NIP`, `Nama`, `Departemen`, `Jabatan`, `Toko Utama Saat Ini`, `Perusahaan Saat Ini`, `Cabang Akun Saat Ini` (terkunci);
    - `Kode Toko Baru` (dropdown kode toko aktif), `Berlaku Mulai`, `Catatan`.
  - `POST /api/hris/employee-placements/move-import`, multipart `file`, `step=preview|commit`, `fingerprint`, `confirm_warnings=true`, `format=error_file`. Respons: `{step, fingerprint, counts:{moved,skipped,error}, rows:[{row, nip, name, outcome, reason, from, to, account, salary_moved, warnings}]}`.
- **Domain:**
  ```go
  type StoreMoveRow struct{ Row int; NIP, BranchCode, Start, Notes string }
  type StoreMoveRefs struct {
      Employees   map[string]StoreMoveEmployee // NIP -> {ID, Active, PrimaryBranchID}
      ActiveCodes map[string]string             // KODE -> branch id
  }
  type StoreMoveLine struct{ Row int; EmployeeID, BranchID, Start, Notes, Skip, Error string }
  func ValidateStoreMoveUpload(rows []StoreMoveRow, refs StoreMoveRefs, today string) []StoreMoveLine
  ```
  - Pesan error:
    - `"NIP tidak dikenal"`, `"NIP ganda di file"`, `"Karyawan nonaktif"`;
    - `"Kode toko tidak dikenal atau nonaktif"`;
    - `"Format tanggal YYYY-MM-DD"`;
    - `"Lebih dari 1000 baris"` (error berkas).
  - Lewati (bukan error): `"Kode Toko Baru kosong"`, `"Toko tidak berubah"`.
  - Periode absensi tertutup dicek di service (`periodContaining`) dengan pesan `"Berlaku Mulai di periode absensi yang sudah ditutup"`.
- **Pesan R9 (EPIC-100):** `"Toko %s masih dipakai: %s — pindahkan dulu lewat Pindah Toko Massal"`.

- [ ] **Step 1: Tes gagal:**
  - Unit `TestValidateStoreMoveUpload`: tabel kasus semua pesan di atas, kode huruf kecil/spasi dinormalisasi `upper(trim)`, dan Start kosong = today.
  - `TestStoreMoveTemplate`: unduhan semua → baris = karyawan aktif. `branch_id=A` → hanya karyawan dengan penempatan utama di A. Kolom dan urutan sesuai.
  - `TestStoreMoveImportMovesToManyStores`: file berisi 3 karyawan A → B, C, B. Preview tidak menulis; commit → penempatan, akun, dan gaji sesuai; audit `placement.template_move` per karyawan.
  - `TestStoreMoveImportRejectsCommitWithErrors`: satu baris NIP tidak dikenal → commit 422 dan tidak ada yang tertulis.
  - `TestStoreMoveImportFingerprintMismatch`: setelah preview, karyawan dipindah lewat API lain → commit 409.
  - `TestStoreMoveImportNeedsWarningConfirm`: ada PERHATIAN (draft payroll) dan `confirm_warnings` kosong → 422 "Saya sudah memeriksa peringatan".
  - `TestStoreMoveThenDeactivateBranch`: semua karyawan A dipindah via template → impor Kode & Status Toko `A → Nonaktif` lolos.
  - `TestBranchCodeImportPointsToStoreMove`: Nonaktif toko berisi karyawan → pesan memuat "Pindah Toko Massal".
  - `TestStoreMoveAccess`: staf tanpa menu Penempatan → 403. Pengatur jadwal toko (delegasi EPIC-071) → 403 untuk template dan impor. HR dengan menu tetapi tanpa aksi `update` → preview boleh, commit 403.
- [ ] **Step 2:** FAIL.
- [ ] **Step 3:** Implementasikan:
  - `ValidateStoreMoveUpload`, lalu service `StoreMoveTemplate` / `ImportStoreMoves`;
  - commit dalam satu transaksi dengan `pg_advisory_xact_lock(hashtext('hris.store-move-import'))` dan memanggil `moveEmployeeTx` per baris;
  - routes, lalu generate manifest.
- [ ] **Step 4:** PASS, ditambah `go test ./internal/modules/hris -run 'BranchCode|StoreMove|BulkMove' -count=1`.
- [ ] **Step 5: Commit** `feat(hris): template pindah toko massal (EPIC-107)`

### Task 6: UI Pindah Toko Massal

**Files:**
- Create: `apps/frontend/src/features/hris/placements/components/store-move-dialog.tsx` (+ `store-move-dialog.test.tsx`)
- Modify: `apps/frontend/src/features/hris/placements/api.ts` (`downloadStoreMoveTemplate(branchId?: string)`, `importStoreMoveFile(file: File, step: "preview" | "commit", fingerprint?: string, confirmWarnings?: boolean)`, `downloadStoreMoveErrors(file: File)`), `types.ts`
- Modify: `apps/frontend/src/features/hris/placements/components/placements-page.tsx` (tombol "Pindah Toko Massal" di samping "Kode & Status Toko")
- Modify: `apps/frontend/src/features/hris/placements/components/branch-code-dialog.tsx` (baris tolak Nonaktif → tombol "Pindah karyawan toko ini" yang membuka dialog tersaring ke toko itu)

**Interfaces:**
- Consumes: T5 routes/respons.
- Produces: `StoreMoveDialog({ open, onOpenChange, branchId?: string })`.

- [ ] **Step 1: Tes gagal** (`store-move-dialog.test.tsx`):
  - langkah 1 menampilkan pilihan "Semua karyawan" / toko dan tombol "Unduh template";
  - unggah → tabel pratinjau dengan kolom NIP, Nama, Toko, Akun, Gaji, Status, Keterangan, plus ringkasan "3 dipindah · 1 dilewati · 0 error";
  - ada error → tombol Simpan nonaktif dan tautan "Unduh baris error";
  - ada PERHATIAN → checkbox "Saya sudah memeriksa peringatan" wajib sebelum Simpan;
  - Simpan memanggil `importStoreMoveFile(file, "commit", fingerprint, true)`, lalu toast sukses dan invalidasi query placements.

  Tes `branch-code-dialog.test.tsx`: tombol "Pindah karyawan toko ini" membuka `StoreMoveDialog` dengan `branchId`.
- [ ] **Step 2:** `pnpm --dir apps/frontend exec vitest run src/features/hris/placements` → FAIL.
- [ ] **Step 3:** Implementasikan. Ikuti tata letak `branch-code-dialog.tsx` (Dialog, langkah unduh, unggah, pratinjau) dan baca panduan `node_modules/next/dist/docs/` bila menyentuh API Next.
- [ ] **Step 4:** PASS, ditambah `pnpm --dir apps/frontend check-types` dan `pnpm --dir apps/frontend lint`.
- [ ] **Step 5: Commit** `feat(hris): dialog pindah toko massal (EPIC-107)`

### Task 7: Gate, review, PR, staging

- [ ] gofmt bersih, go vet, `go test ./...` 2× berturut-turut di DB uji baru; tsc, eslint, vitest penuh.
- [ ] Review akhir oleh reviewer terpisah (model terkuat) dengan Review Focus di atas. Temuan Critical/Important diperbaiki dengan tes merah lalu hijau; gate diulang.
- [ ] PR ke `development`. CI hijau, lalu merge (deploy staging otomatis; migrasi tabel antrean ikut jalan).
- [ ] QA staging (izin user; backup dulu; data uji dikembalikan):
  - template per toko Pudak;
  - pindah 2 karyawan ke 2 toko berbeda;
  - pindah lintas perusahaan HQ → Pudak;
  - berlaku besok;
  - toko kosong lalu Nonaktif;
  - `/api/ready` 200.
- [ ] Catat di Automation Log epic, memori, merge dokumen paddydev, lalu `bash tools/sync-docs-erp-paddy.sh`.
