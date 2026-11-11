# Rapikan Bagan Organisasi Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (user chose inline). Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Menyelesaikan 10 temuan minor EPIC-102/103 yang ditunda, tanpa fitur baru.

**Architecture:** Perubahan kecil dan terpisah di `org_chart_approve.go`, `org_chart_import.go`, `org_chart_vacancy.go`, `org_chart.go` (tree), `platform/xlsx` (format kolom), satu migrasi indeks, satu endpoint akses, dan frontend `features/hris/org-chart`.

**Tech Stack:** Go 1.26 (pgx, excelize), PostgreSQL 16, Next.js 16.4.2 + React Query + vitest.

**Spec:** `docs/epics/EPIC-104-rapikan-bagan.md` (R1–R9).

## Global Constraints

- Branch `feat/epic104-rapikan-bagan` (worktree `.worktrees/epic104`) di atas `feat/epic103-impor-bagan` (PR #98).
- Pesan error dipakai persis seperti di spec. Semua teks untuk pengguna dalam bahasa Indonesia.
- Migrasi baru `20261116090300_org_chart_label_lower_unique.sql`. Migrasi lama tidak diubah.
- **Tes Go:** jalankan di DB uji yang baru dimigrasi (`paddydev-sdd-testdb`, port 55499; `go` di `~/.local/go/bin`). Tes IAM yang bergantung seed dijalankan di DB hasil restore backup staging.
- **Frontend:** `node_modules` adalah symlink. Komponen client saja; tidak ada API Next.js yang disentuh.
- Production tidak disentuh.

## Review Focus

1. **Approve bersamaan.** Dua approve versi berbeda pada saat yang sama hanya menghasilkan satu versi `approved`; yang kalah mendapat pesan R2 dan versinya tetap draft. Test: T1 `TestOrgChartApproveConcurrent`.
2. **Label beda huruf besar/kecil dari jalur lain** (SQL langsung, atau balapan) ditolak oleh indeks. Test: T2 `TestOrgChartLabelLowerUnique`.
3. **Impor 1000 node** (batas atas) tersimpan utuh dalam satu INSERT, tanpa melewati batas parameter Postgres. Test: T2 `TestOrgChartImportMaxRows`.
4. **"Tanpa region" vs region kosong.** `region=-` hanya mengembalikan node tanpa region, dan `region=` (kosong) tetap berarti semua. Test: T4 `TestOrgChartRegionNone`.
5. **Akun tanpa hak** tidak melihat tombol Setujui/Impor/Hapus. Tombol tetap tersembunyi selama data hak belum termuat. Test: T5 `version-bar.test.tsx` "tanpa hak".

---

### Task 1: Pesan kalah balapan + tes dua transaksi

**Files:** Modify `backend/internal/modules/hris/org_chart_approve.go`, `org_chart_approve_integration_test.go`.

**Interfaces:**
- Produces: `func approveOrgVersion(ctx context.Context, q database.Querier, r *http.Request, u *auth.User, id, validFrom string, reason *string) (orgVersion, error)`, the transaction body extracted from `orgChartApprove` so the test can run it on two pool transactions. Unique violation (`23505`) → `httpx.Conflict("Versi lain baru saja disetujui — muat ulang lalu coba lagi")`.

- [ ] **Step 1: Failing test** `TestOrgChartApproveConcurrent`. Use `testutil.DB(t)`. Commit two versions `QA Race A` and `QA Race B`, each with one root node, in their own committed transaction, and delete them in `t.Cleanup`. Begin tx1 and tx2 on the pool. tx1 runs `approveOrgVersion(A)` and is not committed yet. A goroutine runs tx2 `approveOrgVersion(B)`, which blocks on the unique index. Commit tx1. tx2 returns a `*httpx.Error` with status 409 and the R2 message. Then exactly one row is `approved`, and it is A.

  The test must not touch the seeded Draft 6 status: set Draft 6 to `archived` inside tx1 and restore it in cleanup, or skip when any approved row exists.
- [ ] **Step 2:** Run it → FAIL. The function is undefined, or the message is the old one.
- [ ] **Step 3:** Extract and change the message. Remove `_ = iam.HrisOrganization` and its import from the test.
- [ ] **Step 4:** `go test ./internal/modules/hris -run 'OrgChartApprove|DefaultVersion' -count=1` → PASS.
- [ ] **Step 5: Commit** `fix(hris): pesan 409 kalah balapan setujui versi + tes dua transaksi (EPIC-104)`

### Task 2: Label unik `lower` + simpan node sekaligus + tes role ketat

**Files:** Create `backend/database/migrations/deltas/20261116090300_org_chart_label_lower_unique.sql`. Modify `org_chart_import.go`, `org_chart_import_integration_test.go`, `backend/internal/platform/auth/hris_staff_roles_integration_test.go`.

**Interfaces:**
- Migration: `CREATE UNIQUE INDEX IF NOT EXISTS uq_org_chart_versions_label_lower ON hris.org_chart_versions (lower(label));`
- Produces: `func insertOrgNodes(ctx context.Context, q database.Querier, versionID string, nodes []domain.OrgUploadNode) error`, one statement with `len(nodes)` VALUES rows.

- [ ] **Step 1: Failing tests.**
  - `TestOrgChartLabelLowerUnique`: inserting version `draft 6` directly → SQLSTATE `23505`.
  - `TestOrgChartImportMaxRows`: build a 1000-row matrix:
    - one root `EXE-001` (L0);
    - 999 children `EXE-X001…EXE-X999`, parent `EXE-001`, title `Staf <n>`, division EXE, level L7.

    Preview, then commit as `Draft Besar` → 200, message `Versi Draft Besar disimpan sebagai draft (1000 node)`, and a node count of 1000 with `sort_order` 1..1000.
  - Role tests: `TestOrgChartImportRoles` and `TestOrgChartApproveRoles` fail when `admin_hris` or `hris_manager` has no row for the leaf.
- [ ] **Step 2:** Run. `LabelLowerUnique` → FAIL (no 23505). `ImportMaxRows` passes with the per-row insert, which is the guard for the refactor; note it in the ledger.
- [ ] **Step 3:** Add the migration and `insertOrgNodes`, and tighten the role tests.
- [ ] **Step 4:** Run the hris `OrgChart` tests and the auth role tests on a restored staging copy → PASS.
- [ ] **Step 5: Commit** `fix(hris): label versi unik tanpa beda huruf + simpan node sekaligus (EPIC-104)`

### Task 3: Template berformat Teks

**Files:** Modify `backend/internal/platform/xlsx/write.go`, `xlsx_test.go`, `backend/internal/modules/hris/org_chart_import.go`, `org_chart_import_integration_test.go`.

**Interfaces:**
- Produces: `SheetSpec.TextColumns int`. The first N columns get number format 49 (`@`) via `NewStyle` + `SetColStyle`, applied before cells are written.

- [ ] **Step 1: Failing tests.**
  - `TestBuildTextColumns` (xlsx): Build with `TextColumns: 2`, then open with excelize. `GetColStyle` of A and B resolves to a style whose `NumFmt == 49`; C does not.
  - `TestOrgChartTemplate`: additionally asserts that column A of sheet Bagan has number format 49.
- [ ] **Step 2:** Run → FAIL (unknown field).
- [ ] **Step 3:** Implement, and set `TextColumns: len(domain.OrgChartHeaders)` for the Bagan sheet.
- [ ] **Step 4:** `go test ./internal/platform/xlsx ./internal/modules/hris -run 'TextColumns|OrgChartTemplate|OrgChartImport' -count=1` → PASS.
- [ ] **Step 5: Commit** `fix(hris): kolom template bagan berformat Teks (EPIC-104)`

### Task 4: Filter "Tanpa region"

**Files:** Modify `backend/internal/modules/hris/org_chart.go` (tree filter), `org_chart_vacancy.go`, `org_chart_vacancy_integration_test.go`, and frontend `components/chart-vacancy.tsx`, `chart-tree.tsx` with their tests.

**Interfaces:**
- Produces: `func regionMatches(filter, region string) bool`. `""` → true; `"-"` → `region == ""`; otherwise equal.
- Frontend constant `NO_REGION = "-"`, option label `Tanpa region`.

- [ ] **Step 1: Failing tests.**
  - `TestOrgChartRegionNone`: `vacancy?region=-` → every group has `region == ""`, and positions = the sum of the `""` groups of the unfiltered call. `tree?region=-` → every node has region null.
  - Vitest: in chart-vacancy, the Region select has an option `Tanpa region` when an empty-region group exists, and selecting it calls the API with `region: "-"`. Same for chart-tree.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run the Go and vitest org-chart tests → PASS.
- [ ] **Step 5: Commit** `feat(hris): filter Tanpa region di Bagan dan Posisi Kosong (EPIC-104)`

### Task 5: Tombol mengikuti hak + tanggal WIB

**Files:** Modify `org_chart.go` (`GET /api/hris/org-chart/access`), `routes.go`, `go-routes.generated.json`, frontend `api.ts`, `query-keys.ts`, `types.ts`, `components/version-bar.tsx`, `version-bar.test.tsx`.

**Interfaces:**
- `GET /api/hris/org-chart/access` via `withMenu(iam.HrisOrganization, …)` → `data: {approve: bool, import: bool}` from `h.auth.HasMenuAction`.
- Frontend: `fetchOrgChartAccess()`, query key `access()`, type `ChartAccess`. `wibToday()` replaces `localToday()`, using `Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Jakarta" })`.

- [ ] **Step 1: Failing tests.**
  - `TestOrgChartAccess`:
    - a reader with `{read}` → `{approve:false, import:false}`;
    - `{read, approve}` → `{true, false}`;
    - `{read, approve, import}` → `{true, true}`;
    - no menu → 403.
  - Vitest:
    - "tanpa hak": access `{approve:false, import:false}` → no Setujui/Impor/Hapus buttons, but Unduh template is visible;
    - access pending → no buttons;
    - `{approve:true, import:false}` → Setujui only.
    - WIB: with `vi.setSystemTime(new Date("2026-11-11T18:30:00Z"))`, the dialog date is `2026-11-12`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement and regenerate the routes.
- [ ] **Step 4:** Run the full vitest suite, tsc and eslint, plus `go test ./internal/modules/hris -run OrgChart` → green.
- [ ] **Step 5: Commit** `feat(hris): tombol bagan mengikuti hak akses + tanggal bawaan WIB (EPIC-104)`

### Task 6: Gate, review, QA staging, catatan

- [ ] Full backend gate on a fresh DB; frontend tsc, eslint and vitest.
- [ ] Final whole-branch review by a fresh reviewer. Fix Critical and Important findings test-first.
- [ ] Staging: run `df`, back up to `/mnt/hdd1/backup/paddydev/staging-before-e104-<stamp>.dump`, then deploy.
- [ ] QA:
  - access endpoint for the hrd account and a temporary read-only account;
  - region `-`;
  - the template opens with Text columns;
  - Playwright: the read-only account sees no Setujui/Impor/Hapus buttons;
  - clean up afterwards.
- [ ] Epic log, status `review`, memory. Commit `docs: EPIC-104 selesai, hasil gate dan QA staging`.
