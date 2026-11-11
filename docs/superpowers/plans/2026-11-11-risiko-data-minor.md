# Tutup Risiko Data dari Minor yang Ditunda Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Menutup enam risiko data atau kegagalan dari minor EPIC-095/098/099/100 yang ditunda, ditambah dua tes flaky.

**Architecture:** Perbaikan kecil dan terpisah, masing-masing dengan tes yang gagal dulu:
- advisory lock untuk nomor urut NIP dan impor toko;
- pemetaan deadlock → 409;
- penghapusan fallback GRN;
- audit pembatalan oleh job;
- cek siklus atasan di validasi impor (domain murni);
- jam tetap dan percobaan ulang di dua tes.

**Tech Stack:** Go 1.26 (pgx, pgconn), PostgreSQL 16, Next.js 16.4.2 (satu file TS).

**Spec:** `docs/epics/EPIC-105-risiko-data-minor.md` (R1–R6). Lokasi kode: laporan agen pencari, sudah ditulis di setiap task.

## Global Constraints

- Branch `feat/epic105-risiko-data` (worktree `.worktrees/epic105`) dari `main` 4b113831.
- Pesan untuk pengguna dipakai persis seperti di spec, dalam bahasa Indonesia.
- Tidak ada migrasi skema. Advisory lock memakai `pg_advisory_xact_lock(hashtext('<kunci>'))`, sama dengan pola yang sudah ada (mis. `ticketing/capacity.go:18`).
- Kunci lock: `hris.employee-nip-seq` untuk NIP; `hris.branch-code-import` untuk impor toko.
- **Tes Go:** jalankan di DB uji yang baru dimigrasi (`paddydev-sdd-testdb`, port 55499; `go` di `~/.local/go/bin`). Tes konkuren memakai `testutil.DB(t)` dengan dua transaksi nyata, lalu dibersihkan.
- Production tidak disentuh.

## Review Focus

1. **NIP dari dua jalur berbeda bersamaan** (mis. HR membuat karyawan, sementara rekrutmen mengonversi kandidat) tidak menghasilkan nomor urut ganda. Setiap jalur harus mengambil lock di transaksi yang sama dengan INSERT. Test: T1 `TestCheckNewNIPConcurrentSeries`, plus pemeriksaan setiap pemanggil.
2. **GRN untuk gudang aktif di toko aktif tetap berhasil** dengan cakupan bisnis yang sama seperti sebelumnya. Hanya gudang tanpa cakupan yang ditolak. Test: T3 `TestReceivingScopeActive` / `TestReceivingScopeNoBusiness`.
3. **Impor karyawan mode Timpa yang membalik atasan dua karyawan lama** (A atasan B menjadi B atasan A) tidak dianggap siklus, kalau barisnya memang mengganti keduanya. Siklus dihitung dari kondisi setelah impor. Test: T5 `TestValidateEmployeeImportSwapIsNotCycle`.
4. **Pembatalan oleh job tetap terjadi** walau penulisan audit membutuhkan nama karyawan. Nama diambil dalam transaksi yang sama, dan bila tidak ada, label memakai NIP. Test: T4 memperluas `TestRunScheduledResignsInactive` dan `TestScheduledResignInvalidIsCancelled`.
5. **Error 409 deadlock tidak menyembunyikan error lain.** Hanya `40P01` dan `55P03` yang dipetakan; error DB lain tetap seperti sebelumnya. Test: T2 `TestRetryableLockError`.

---

### Task 1: Kunci nomor urut NIP

**Files:**
- Modify: `backend/internal/platform/employeekeys/employeekeys.go`, `backend/internal/modules/hris/{employees.go,postgres_employees.go}`, `backend/internal/modules/configuration/accounts/employees.go`, `backend/internal/modules/recruitment/service_hris.go`, and the employee Excel import commit (`backend/internal/modules/hris/employee_excel.go`).
- Test: Create `backend/internal/platform/employeekeys/employeekeys_integration_test.go`.

**Interfaces:**
- Produces: `func LockNIPSequence(ctx context.Context, q database.Querier) error`. `CheckNewNIP` calls it first, so every caller holds the lock until its own transaction ends.
- Requirement: every caller of `CheckNewNIP`, and the import commit's NIP check, must run the check and the INSERT/UPDATE in one transaction.
  - Callers that pass a pool (`s.db` outside `InTx`) are moved into a transaction.
  - In the ledger, list each caller and whether it already ran in a transaction.

- [ ] **Step 1: Failing test** `TestCheckNewNIPConcurrentSeries`.
  - Insert one committed main employee with NIP `<YYYY>01521`, using a test-only far-future year (e.g. 2090), and clean it up afterwards.
  - tx1: `CheckNewNIP("209001522", "2090-01-05", "permanent")` → nil, then INSERT the employee with that NIP. Keep tx1 open.
  - Goroutine tx2: `CheckNewNIP("209002522", "2090-02-05", "permanent")`.
  - Wait until tx2's backend PID is waiting on a Lock (pg_stat_activity), then commit tx1.
  - tx2 must return a 400 whose message starts with `NIP 209002522: nomor urut terakhir karyawan 522`.
  - Before the fix there is no wait, so the probe times out → FAIL.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Add `LockNIPSequence`, call it in `CheckNewNIP`, and fix the callers' transaction scope. The import commit takes the lock before revalidating NIPs.
- [ ] **Step 4:** Run `go test ./internal/platform/employeekeys ./internal/modules/hris ./internal/modules/configuration/... ./internal/modules/recruitment/... -count=1` → PASS.
- [ ] **Step 5: Commit** `fix(hris): kunci nomor urut NIP saat dibuat bersamaan (EPIC-105)`

### Task 2: Impor toko berurutan + deadlock → 409

**Files:**
- Modify: `backend/internal/modules/hris/branch_code_import.go`, `placements.go` (`runBulkPlacement`, `placementError` ~:343).
- Create: `backend/internal/modules/hris/lock_errors.go`.
- Test: `lock_errors_test.go`, `branch_code_import_integration_test.go`.

**Interfaces:**
- Produces: `func retryableLockError(err error) error`. It returns `httpx.Conflict("Data sedang diubah proses lain — coba lagi sebentar")` for pg codes `40P01` and `55P03`, and otherwise `err` unchanged.

- [ ] **Step 1: Failing tests.**
  - `TestRetryableLockError`: `&pgconn.PgError{Code: "40P01"}` and `55P03` → 409 with that message. `23505`, `nil` and a plain error pass through unchanged.
  - `TestBranchCodeImportSerialised`:
    - On `testutil.DB`, tx1 takes `pg_advisory_xact_lock(hashtext('hris.branch-code-import'))`.
    - A commit-step import through the harness must block while tx1 holds the lock: check that the harness request runs only after tx1 commits.
    - If the harness cannot run outside its own tx, assert instead that `ImportBranchCodes` commit issues the lock: query `pg_locks` for an advisory lock from the harness tx after calling the service's commit inside `h.tx`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement the lock at the start of the commit branch (`in.Step == "commit"`). Wrap the returned error of `ImportBranchCodes` and of the bulk placement transaction with `retryableLockError`.
- [ ] **Step 4:** Run `go test ./internal/modules/hris -run 'BranchCode|Placement|RetryableLock' -count=1` → PASS.
- [ ] **Step 5: Commit** `fix(hris): impor kode toko berurutan dan deadlock dijawab 409 (EPIC-105)`

### Task 3: GRN tanpa kode toko tertanam

**Files:**
- Modify: `backend/internal/modules/procurement/service_grn.go` (`receivingScope` ~:801, `warehouseScopeErrors`), `frontend/src/lib/purchasing/grn-create.ts` (~:127-142).
- Test: a procurement GRN test with a fake `Locations` port (create `grn_scope_test.go` if none exists), and `frontend/src/app/api/purchasing/grn/route.test.ts` / the grn-create test.

**Interfaces:**
- `warehouseScopeErrors["no_business"] = "Gudang tidak terhubung ke toko dan perusahaan yang aktif"`. `receivingScope` returns `badRequest` with it when `WarehouseScope` is nil. `ScopeByCodes` is no longer called from GRN.
- First check whether `/api/purchasing/grn` is served by Go (see `frontend/src/lib/go-routes.generated.json`). If the TS route is dead, still remove the hard-coded codes there, and record that in the ledger.

- [ ] **Step 1: Failing tests.**
  - `TestReceivingScopeNoBusiness`: the fake's `WarehouseScope` returns nil → 400 with the message, and the fake's `ScopeByCodes` is never called.
  - `TestReceivingScopeActive`: `WarehouseScope` returns company/branch → the business is returned unchanged (characterisation).
  - TS: with no warehouse scope, `resolveBusinessScopeByCodes` is not called and the request returns the same message.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement in Go and TS.
- [ ] **Step 4:** Run `go test ./internal/modules/procurement/... -count=1` and the TS test → PASS.
- [ ] **Step 5: Commit** `fix(procurement): GRN tanpa fallback kode toko MIZU-BANDUNG (EPIC-105)`

### Task 4: Audit pembatalan resign terjadwal oleh job

**Files:**
- Modify: `backend/internal/modules/hris/scheduled_resign.go` (cancel paths at ~:73-79 and ~:88-97).
- Test: `scheduled_resign_integration_test.go`.

**Interfaces:**
- Consumes: `writeEmployeeAudit(ctx, q, scheduledResignActor, "employee.resign_schedule_cancel", empID, name, before, after)` (`employee_audit.go:20`).
  - before = `{effective_date, resign_type}`;
  - after = `{cancel_reason}`.
  - The name is read in the same transaction; the label falls back to the NIP when the name is empty.

- [ ] **Step 1: Failing tests.** Extend `TestRunScheduledResignsInactive` and `TestScheduledResignInvalidIsCancelled`: exactly one `audit.audit_log` row with action `employee.resign_schedule_cancel`, `actor_name = 'Sistem (resign terjadwal)'`, and `after->>'cancel_reason'` equal to the stored cancel reason.
- [ ] **Step 2:** Run → FAIL (0 rows).
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `go test ./internal/modules/hris -run 'ScheduledResign|Resign' -count=1` → PASS.
- [ ] **Step 5: Commit** `fix(hris): pembatalan otomatis resign terjadwal tercatat di audit (EPIC-105)`

### Task 5: Impor karyawan menolak siklus atasan

**Files:**
- Modify: `backend/internal/modules/hris/domain/employee_import.go` (the lookups struct ~:87; validation ~:362-376), and the lookup loader in `employee_excel.go`, which fills the existing managers.
- Test: `domain/employee_import_test.go`, `employee_excel_integration_test.go`.

**Interfaces:**
- Lookups gain `ManagerOfNIP map[string]string`: the NIP of each existing employee mapped to their current manager's NIP ("" = none).
- After all rows are validated, build the post-import manager map: existing managers, overlaid by each valid row's manager NIP (`reporting_to` from `lk.ManagerNIP`, or `ManagerNIPNew`). Walk each imported row's chain, capped at 50 steps. A row whose chain returns to itself gets an error on column `NIP Atasan`: `NIP Atasan membentuk siklus: A → B → A`, written with the NIPs in walk order.

- [ ] **Step 1: Failing tests.**
  - `TestValidateEmployeeImportCycleInFile`: rows A (atasan B) and B (atasan A) → both rows get the cycle error.
  - `TestValidateEmployeeImportCycleWithExisting`: an existing B with manager A; the file sets A's manager to B → error on A's row.
  - `TestValidateEmployeeImportSwapIsNotCycle`: existing A→B; the file sets B's manager to A and A's manager to "" (top) → no error.
  - Integration `TestEmployeeImportManagerCycle`: replace-mode file with A↔B → 200 preview with errors, and commit refused; `reporting_to` unchanged.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run `go test ./internal/modules/hris/domain ./internal/modules/hris -run 'EmployeeImport' -count=1` → PASS.
- [ ] **Step 5: Commit** `fix(hris): impor karyawan menolak siklus atasan (EPIC-105)`

### Task 6: Tes flaky

**Files:**
- Modify: `backend/internal/modules/memberportal/collection_integration_test.go` (`collSetup` env and `TestRewardRedeemRateLimit`), `ratelimit_integration_test.go` (`TestIntegrationRedeemBrakeIsSharedAcrossReplicas`), and `backend/internal/platform/ratelimit/ratelimit_test.go` (`TestPruneDropsExpiredKeys`).

- [ ] **Step 1:** Pin the clock. Expose the service in the test env and set `svc.now` to one fixed instant for the whole test, on every replica.
  - Retry-After must then be exactly `60`.
  - Prove it: before the change, insert a `time.Sleep(1100 * time.Millisecond)` between the 10th and 11th attempts and watch it fail (RED). After the change, the test is GREEN with the sleep, then remove the sleep.
- [ ] **Step 2:** Prune: loop Prune + Peek up to 20 times, 50 ms apart, and fail only if the key is still there.
- [ ] **Step 3:** Run each test with `-count=5` → PASS.
- [ ] **Step 4: Commit** `test: stabilkan tes rate limit memberportal dan prune (EPIC-105)`

### Task 7: Gate, review, QA staging, catatan, PR

- [ ] **Gate:**
  - backend: full suite on a fresh DB (gofmt, vet, `go test ./internal/...`);
  - frontend: tsc, eslint, and vitest on the full suite.
- [ ] **Final review** by a fresh reviewer, with this Review Focus. Fix Critical and Important findings test-first.
- [ ] **Staging:** run `df`, back up to `/mnt/hdd1/backup/paddydev/staging-before-e105-<stamp>.dump`, then deploy.
- [ ] **QA on staging,** using temporary sessions, then clean up:
  - Create two employees concurrently with NIPs in different months: one is accepted, the other gets the sequence message.
  - Upload an employee import with A↔B: rejected with the cycle error.
  - A GRN to an active warehouse still works (read-only check via preview or API validation).
  - Verify the resign cancel audit by running the job against a demo schedule.
- [ ] **Epic log, status, memory, and PR** to `main`.
