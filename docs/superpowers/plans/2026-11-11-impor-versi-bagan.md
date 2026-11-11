# Impor Versi Bagan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR membuat versi bagan baru dari template Excel (unduh → ubah → pratinjau → simpan sebagai draft) dan bisa menghapus draft yang salah.

**Architecture:**
- **Template** ditulis dari node versi yang sedang dilihat.
- **Unggahan** dibaca oleh parser domain murni (validasi baris R3, ditambah `ValidateOrgChart` untuk BR) dan dibandingkan dengan versi dasar (`DiffOrgCharts`). Perkiraan keadaan posisi memakai `MatchOrgChart` + `OrgVacancies` (EPIC-102).
- **Alur** mengikuti EPIC-100 (Kode & Status Toko): `step=preview|error_file|commit`, sidik SHA-256, centang peringatan, satu transaksi + audit.
- **Frontend** menambah tombol di `VersionBar` dan dialog impor yang meniru `branch-code-dialog.tsx`.

**Tech Stack:** Go 1.26 (pgx, `internal/platform/xlsx`, `audit`), PostgreSQL 16, Next.js 16.4.2 + React Query + vitest.

**Spec:** `docs/epics/EPIC-103-impor-versi-bagan.md` (R1–R10). Dibangun di atas EPIC-102 (`docs/epics/EPIC-102-posisi-kosong-approve-versi.md`, branch `feat/epic102-vacancy-versi`, PR #97).

## Global Constraints

- Branch `feat/epic103-impor-bagan` (worktree `.worktrees/epic103`), bertumpuk di atas `feat/epic102-vacancy-versi`. PR kelak ber-base PR #97, atau `main` setelah #97 merge.
- Tidak ada perubahan pada karyawan, jabatan, toko, `reporting_to`, `hris.org_title_aliases`, atau approval (R1). Versi yang diimpor selalu `draft` (R7).
- Template: `menu hris.organization` (baca). Impor dan hapus draft: `h.withAction("import", iam.HrisOrganization, …)` (R9).
- Nama sheet `Bagan`. Header persis: `Kode, Kode Atasan, Jabatan, Posisi, Kode Divisi, Divisi, Level, Jenis Kerja, Vendor, Region, Toko, Status, Perlu Konfirmasi` (R2).
- Semua teks untuk pengguna dalam bahasa Indonesia. Pesan error dipakai persis seperti tertulis di rencana ini.
- Batas: file `.xlsx` maks. 16 MB, maks. 1000 baris node, label maks. 40 karakter.
- **Migrasi baru**, jangan mengubah migrasi EPIC-102 yang sudah diterapkan di staging. Nomor `20261116090200`.
- **Tes Go:**
  - Jalankan di DB uji yang baru dimigrasi: `TEST_DATABASE_URL=postgres://postgres@127.0.0.1:55499/<db>?sslmode=disable` (container `paddydev-sdd-testdb`; `go` ada di `~/.local/go/bin`).
  - Tes IAM yang bergantung seed di-skip di DB kosong. Buktikan RED/GREEN-nya di DB hasil restore backup staging, seperti EPIC-102.
- **Frontend:** worktree belum punya `node_modules`. Buat symlink ke `.worktrees/org-chart/frontend/node_modules` (lockfile sama) dan pastikan `git status` tetap bersih. Komponen baru hanya client component; tidak ada API Next.js yang disentuh.
- Production tidak disentuh. Deploy butuh izin terpisah (memory `alur-migrasi-production`).

## Review Focus

1. **Round-trip:** template yang diunduh lalu diunggah tanpa diubah harus memberi 0 error dan selisih kosong. Teks seperti `L0`, `001`, dan kode toko harus tetap teks. Test: T3 `TestOrgChartImportRoundTrip`.
2. **Label bentrok:** label yang sudah ada, termasuk beda huruf besar/kecil ("draft 6"), ditolak. Dua simpan berlabel sama tidak menghasilkan dua versi. Test: T3 `TestOrgChartImportLabel`.
3. **File atau label berubah setelah pratinjau:** simpan ditolak 409 dan tidak ada yang tertulis. Test: T3 `TestOrgChartImportFingerprint`.
4. **Hapus draft:** tidak pernah menghapus versi disetujui atau arsip. Setelah menghapus versi yang sedang dilihat, pemilih kembali ke versi bawaan. Test: T3 `TestOrgChartDeleteDraft`, T4 `version-bar.test.tsx` "hapus draft".
5. **Node yang masih punya bawahan dihapus dari file:** bawahannya dilaporkan error BR-03 di baris masing-masing, tidak diam-diam jadi yatim. Test: T1 `TestParseOrgChartUploadRemovedParent`.

---

### Task 1: Domain — parser sheet Bagan, validasi baris, selisih

**Files:**
- Create: `backend/internal/modules/hris/domain/org_chart_upload.go`
- Test: `backend/internal/modules/hris/domain/org_chart_upload_test.go`

**Interfaces:**
- Consumes: `OrgNode`, `ValidateOrgChart`, `UploadIssue`, `NormalizeHeader` (existing).
- Produces:
  ```go
  const OrgChartSheet = "Bagan"
  const MaxOrgChartRows = 1000
  var OrgChartHeaders = []string{"Kode", "Kode Atasan", "Jabatan", "Posisi", "Kode Divisi", "Divisi", "Level",
      "Jenis Kerja", "Vendor", "Region", "Toko", "Status", "Perlu Konfirmasi"}

  type OrgUploadNode struct {
      OrgNode
      DivisionName string
      NeedsConfirm bool
      Line         int // 1-based sheet row
  }
  type OrgUploadResult struct {
      Nodes            []OrgUploadNode
      Errors, Warnings []UploadIssue
      HeaderLine       int
  }
  // problem != "" when the sheet is not the template (no "Kode" header) or has no rows / too many rows.
  func ParseOrgChartUpload(matrix [][]string, activeStores []string) (res OrgUploadResult, problem string)

  func EmploymentTypeLabel(code string) string // PERMANENT_OR_CONTRACT→"Tetap/Kontrak", INTERN→"Magang", FREELANCE→"Freelance", OUTSOURCE→"Outsource", ""→""
  func NodeStatusLabel(status string) string   // active→"Aktif", planned→"Rencana", vacant→"Kosong", frozen→"Dibekukan"

  type OrgNodeChange struct {
      Code   string   `json:"code"`
      Title  string   `json:"title"`
      Fields []string `json:"fields"` // subset, in this order: "Jabatan","Atasan","Posisi","Divisi","Level","Jenis Kerja","Region","Toko","Status"
  }
  type OrgChartDiff struct {
      Added   []string        `json:"added"`   // sorted codes
      Removed []string        `json:"removed"` // sorted codes
      Changed []OrgNodeChange `json:"changed"` // sorted by code
  }
  func DiffOrgCharts(base, next []OrgNode) OrgChartDiff // never nil slices
  ```

- [ ] **Step 1: Write the failing tests.** Build matrices inline as `[][]string` (header row + rows).
  - `TestParseOrgChartUploadValid`:
    - Three rows: `EXE-001` (puncak, CEO, Ya, EXE, Executive, L0, Tetap/Kontrak); `EXE-002` (atasan EXE-001, General Manager, L1); `SUP-001` (atasan EXE-001, Support, Posisi `Tidak`, level kosong).
    - Plus a blank row in between, and a fourth row: `RTL-010` (atasan EXE-002, Shopkeeper, RTL, Retail Operational, L7, Freelance, toko `ppga; PPGI`, Status `Rencana`, Perlu Konfirmasi `Ya`).
    - Expect: 4 nodes; 0 errors; `RTL-010` has `StoreCodes == []string{"PPGA","PPGI"}`, `Status == "planned"`, `NeedsConfirm`, `EmploymentType == "FREELANCE"`, and `Line == 6`; `SUP-001.IsPosition == false`.
    - With `activeStores = []string{"PPGA"}`: exactly one warning, line 6, column `Toko`, message `Toko PPGI tidak ada sebagai toko aktif di HRIS`.
  - `TestParseOrgChartUploadRowErrors`: one issue per row, asserted by `(Line, Column, Message)`:
    - duplicate Kode → `Kode EXE-002 dipakai lebih dari sekali (baris 3)`;
    - Kode `ex 1` → `Format kode tidak valid`;
    - Jabatan empty → `Jabatan wajib diisi`;
    - Posisi `Mungkin` → `Isi Posisi dengan Ya atau Tidak`;
    - Level `L9` → `Level harus L0 sampai L8`;
    - Jenis Kerja `Kontrakan` → `Jenis kerja tidak dikenal`;
    - Status `Hilang` → `Status harus Aktif, Rencana, Kosong, atau Dibekukan`;
    - Toko `PP GA` → `Kode toko PP GA tidak valid`;
    - Kode Divisi empty → `Kode divisi wajib diisi`.
    - Divisi empty → `Divisi wajib diisi`;
    - Jabatan of 151 characters → `Jabatan maksimal 150 karakter`.
  - `TestParseOrgChartUploadBRIssues`: two roots plus a level inversion. Expect a BR-01 error with `Line == 0`, `Column == "Kode Atasan"`, and the `ValidateOrgChart` message. Expect a BR-04 error on the child's line, column `Level`. BR issues use the node's line and the column of the field the rule checks: BR-01/02/03 → Kode Atasan, BR-04 → Level, BR-05 → Toko, BR-07 → Jabatan, BR-09 → Level, BR-10 → Vendor.
  - `TestParseOrgChartUploadRemovedParent`: `RTL-053` is absent but `RTL-054` names it as Kode Atasan → BR-03 error on RTL-054's line.
  - `TestParseOrgChartUploadProblems`:
    - no `Kode` header → problem `Judul kolom "Kode" tidak ditemukan — pakai template dari halaman Struktur Org`;
    - header only → `File tidak berisi baris bagan`;
    - 1001 rows → `Maksimal 1000 baris per file`.
  - `TestDiffOrgCharts`: base `A`, `B` (title "Staf"), `C`; next `A` unchanged, `B` with title "Staf Senior" and parent changed, `D` new → `Added == ["D"]`, `Removed == ["C"]`, `Changed == [{B, "Staf Senior", ["Jabatan","Atasan"]}]`. `DiffOrgCharts(nil, nil)` returns empty, non-nil slices.
- [ ] **Step 2: Run them to see them fail.** `cd backend && go test ./internal/modules/hris/domain -run 'OrgChartUpload|DiffOrgCharts' -count=1`. Expect a compile error: `undefined: ParseOrgChartUpload`.
- [ ] **Step 3: Implement** in `org_chart_upload.go`. Header detection: the first row whose first cell normalises to `kode` (same as EPIC-100). Cell reads trim spaces. Kode and Kode Atasan are upper-cased. Toko is split on `,` and `;`, upper-cased, and deduplicated in order. A row with every cell empty is skipped. Run `ValidateOrgChart` on the nodes that passed row checks, and map each `OrgIssue` to an `UploadIssue` by node code → line.
- [ ] **Step 4: Run the domain package.** `go test ./internal/modules/hris/domain -count=1` → PASS, existing tests included.
- [ ] **Step 5: Commit.** `feat(hris): baca template bagan, validasi baris, selisih versi (EPIC-103)`

### Task 2: Template Excel + aksi `import`

**Files:**
- Create: `backend/internal/modules/hris/org_chart_import.go` (template part only)
- Create: `backend/database/migrations/deltas/20261116090200_org_chart_import_action.sql`
- Modify: `backend/database/seeders/iam-menus.sql` (`hris.organization.org-chart` actions `["read","approve","import"]`)
- Modify: `backend/internal/modules/hris/routes.go`, `frontend/src/lib/go-routes.generated.json`
- Test: `backend/internal/modules/hris/org_chart_import_integration_test.go`, `backend/internal/platform/auth/hris_staff_roles_integration_test.go`

**Interfaces:**
- Consumes: `orgVersionOf`, `orgNodes` (`backend/internal/modules/hris/org_chart.go`); `EmploymentTypeLabel`, `NodeStatusLabel`, `OrgChartHeaders` (T1); `writeFile`, `exportFile`, `xlsx.Build`.
- Produces:
  - `func orgChartTemplate(v orgVersion, nodes []orgNodeView) (*exportFile, error)`. File name `Template Bagan <label>.xlsx`, sheet `Bagan` first and `Petunjuk` second.
  - `GET /api/hris/org-chart/template?version=` → `h.withMenu(iam.HrisOrganization, h.orgChartTemplateFile)`.
  - The migration:
    - adds `import` to the leaf's `permission_context`;
    - grants `import` to existing grant rows of `super_admin`, `admin`, `hrd`, `admin_hris` on that leaf;
    - adds `import` to `excluded_actions` of the `hris.organization.org-chart` mask rows of `hris_non_payroll` and `hris_non_rekrutmen_kpi`, using the inherit-then-add insert from `20261116090100` so it is idempotent;
    - runs the guarded `iam.sync_hris_staff_roles()`.

- [ ] **Step 1: Write failing tests.**
  - `TestOrgChartTemplate` (hris):
    - `hr` (menu `iam.HrisOrganization`) gets 200 with `Content-Type == xlsx.ContentType` and filename `Template Bagan Draft 6.xlsx`.
    - `ParseMatrix(PreferSheet "Bagan")` has header == `domain.OrgChartHeaders`, 150 data rows, and the first data row `EXE-001`, empty Kode Atasan, `CEO`, `Ya`, `EXE`, `Executive`, `L0`, `Tetap/Kontrak`.
    - The `Petunjuk` sheet contains `Rencana` and `Dibekukan`.
    - `?version=Draft%2099` → 404 `Versi bagan tidak ditemukan`.
    - A user without the menu → 403.
  - `TestOrgChartImportRoles` (auth, seed-dependent via `syncTx`):
    - The leaf offers `import`.
    - `admin_hris` and `hris_manager` hold it.
    - `hris_non_payroll` and `hris_non_rekrutmen_kpi` keep `read` without `import`, `approve`, or `delete`.
    - Also update `TestHrisStaffRolesDeriveFromAdminHris` to drop `import` on that leaf for the two staff roles.
- [ ] **Step 2: Run to see them fail.** `go test ./internal/modules/hris -run TestOrgChartTemplate -count=1` → 404. For the auth test, restore `/mnt/hdd1/backup/paddydev/staging-before-e102b-20261111-115350.dump` into a fresh test DB, apply migrations up to the EPIC-102 ones, then run `go test ./internal/platform/auth -run 'OrgChartImportRoles' -count=1` → FAIL (no `import`).
- [ ] **Step 3: Implement** the template, the route, the migration, and the seeder change. Every cell is written as a string, including Level and codes. Store codes are joined with `", "`.
- [ ] **Step 4: Verify.**
  - Apply the migration on the restored DB and re-run the auth tests → `TestOrgChartImportRoles` and `TestHrisStaffRolesDeriveFromAdminHris` PASS. `TestIAMGuards` fails on prod data because `gym.packages` was deleted there; that is known and unrelated.
  - Dry-run the IAM part on the restored DB inside `BEGIN … ROLLBACK`, with a mask hiding `hris.organization` for `hris_non_payroll` → the menu stays hidden.
  - Regenerate routes: `cd backend && go run ./cmd/api -routes > ../frontend/src/lib/go-routes.generated.json`, then `cd frontend && npx vitest run src/lib/backend-routes.test.ts`.
- [ ] **Step 5: Commit.** `feat(hris): template Excel bagan + aksi import (EPIC-103)`

### Task 3: API impor + hapus draft

**Files:**
- Modify: `backend/internal/modules/hris/org_chart_import.go`
- Modify: `backend/internal/modules/hris/routes.go`, `frontend/src/lib/go-routes.generated.json`
- Test: `backend/internal/modules/hris/org_chart_import_integration_test.go`

**Interfaces:**
- Consumes:
  - `ParseOrgChartUpload`, `DiffOrgCharts` (T1);
  - `orgPeople`, `domainNodes`, `MatchOrgChart`, `OrgVacancies`, `GroupVacancies`, `errorWorkbook`, `audit.Write`;
  - `orgVacancySummary` (EPIC-102 `org_chart_vacancy.go`);
  - the multipart pattern of `importBranchCodes` (`branch_code_import.go:345`).
- Produces:
  - `POST /api/hris/org-chart/import` (multipart) via `h.withAction("import", iam.HrisOrganization, h.orgChartImport)`.
    - Fields: `file`, `label`, `base` (version label, optional), `step` = `preview|error_file|commit`, `fingerprint`, `ack_warnings`.
    - Preview `data`:
      ```
      { label, base: {label}, summary: {nodes, positions, errors, warnings},
        errors: [UploadIssue], warnings: [UploadIssue], diff: OrgChartDiff,
        vacancy: orgVacancySummary, fingerprint }
      ```
    - Commit `data`: `{ version: orgVersion }`, message `Versi <label> disimpan sebagai draft (<n> node)`.
    - `error_file` returns `Error - <nama file>` built by `errorWorkbook(matrix, HeaderLine-1, errors+warnings, "Bagan")`.
  - Error messages, exactly:
    - `Unggah file .xlsx dari template bagan` (wrong file or no file);
    - `File terlalu besar — maksimal 16 MB`;
    - `Langkah import tidak dikenal`;
    - `Label versi wajib diisi`;
    - `Label versi maksimal 40 karakter`;
    - `Label versi sudah dipakai` (400 at preview and commit; 409 on unique violation at commit);
    - `File masih punya error — perbaiki lalu unggah ulang`;
    - `Centang "Saya sudah memeriksa peringatan" sebelum menyimpan`;
    - `File atau label berubah sejak pratinjau — pratinjau ulang` (409);
    - the T1 header problems as 400.
  - Fingerprint = hex SHA-256 of `data | lower(trim(label)) | base version id`.
  - Commit runs in one `InTx`:
    1. insert the version (`status 'draft'`, `source_file` = file name);
    2. insert nodes with `sort_order` = 1-based upload order, `raw_title` = title, `level_confidence` = `'manual'`, empty strings as NULL for nullable columns;
    3. write audit `hris.org_chart.version.import` (R10).
  - `DELETE /api/hris/org-chart/versions/{id}` via `h.withAction("import", iam.HrisOrganization, h.orgChartDeleteDraft)`.
    - Non-UUID or unknown → 404 `Versi bagan tidak ditemukan`.
    - Status ≠ draft → 409 `Hanya versi draft yang bisa dihapus`.
    - Otherwise lock `FOR UPDATE`, delete (nodes cascade), write audit `hris.org_chart.version.delete` → 200 `data: {id, label}`, message `Versi <label> dihapus`.

- [ ] **Step 1: Write failing tests.** Build upload files by downloading the T2 template and editing its matrix with a helper `orgTemplateMatrix(t, h, staff) [][]string` plus `orgUpload(t, h, staff, fields, matrix)`, modelled on `uploadBranchCodes`.
  - `TestOrgChartImportRoundTrip`: the unchanged template of Draft 6 with label `Draft 7` → 200, `errors` empty, `diff` with 0 added, removed and changed, `summary.nodes == 150`, `summary.positions == 149`, `vacancy.positions == 149`, and a fingerprint of 64 hex characters.
  - `TestOrgChartImportCommit`:
    - Edit the matrix: retitle `RTL-054` to `Store Leader Senior`; append `RTL-900` (atasan `RTL-053`, Shopkeeper, RTL, Retail Operational, L7, toko `PPMR`); delete the row of a leaf without children (pick one from Draft 6, e.g. `RTL-056`, after checking it has no children).
    - Preview → `diff == {added [RTL-900], removed [RTL-056], changed [{RTL-054 … ["Jabatan"]}]}`.
    - Commit with that fingerprint and `ack_warnings=true` → 200, message `Versi Draft 7 disimpan sebagai draft (150 node)`.
    - `GET /versions` lists Draft 7 as `draft` with 150 nodes. The `RTL-900` row has the right parent, and `sort_order` puts it last.
    - Exactly one audit row `hris.org_chart.version.import`.
    - Draft 6 node count is unchanged.
    - `POST /versions/{draft7}/approve` (EPIC-102) → 200.
  - `TestOrgChartImportErrors`:
    - A matrix with a duplicate code and a second root → preview 200 with errors that carry line numbers.
    - Commit → 400 `File masih punya error — perbaiki lalu unggah ulang`.
    - `step=error_file` → xlsx whose `Bagan` sheet contains `Keterangan Error`.
  - `TestOrgChartImportLabel`:
    - empty → `Label versi wajib diisi`;
    - 41 characters → `Label versi maksimal 40 karakter`;
    - `draft 6` → `Label versi sudah dipakai`;
    - commit `Draft 7` twice with the same fingerprint → the second is 400 `Label versi sudah dipakai`, and there is one Draft 7 row.
  - `TestOrgChartImportFingerprint`: preview as `Draft 7`, then commit as `Draft 8` with the old fingerprint → 409 `File atau label berubah sejak pratinjau — pratinjau ulang`, and no version row. Draft 6's BR-04 warnings plus the test DB's store warnings mean commit without `ack_warnings` → 400 with the centang message.
  - `TestOrgChartImportAccess`:
    - `staffGrants` read-only on `hris.organization.org-chart` → import 403, delete 403, template 200;
    - `readImport` (`read`, `import`) → preview 200.
  - `TestOrgChartDeleteDraft`:
    - Import Draft 7, then delete it → 200 `Versi Draft 7 dihapus`; nodes gone; one audit `hris.org_chart.version.delete`.
    - Approve Draft 6, then delete Draft 6 → 409.
    - Delete a random UUID → 404.
- [ ] **Step 2: Run to see them fail.** `go test ./internal/modules/hris -run 'OrgChartImport|OrgChartDeleteDraft' -count=1` → 404 route not found.
- [ ] **Step 3: Implement** the handler pair and service in `org_chart_import.go`. The size limit and file-type check follow `importBranchCodes`. The label check queries `lower(label) = lower($1)` inside the transaction before inserting.
- [ ] **Step 4: Verify.** Regenerate routes as in T2, then `go test ./internal/modules/hris/... -count=1` → PASS with no regressions.
- [ ] **Step 5: Commit.** `feat(hris): impor versi bagan (pratinjau, file error, simpan draft) + hapus draft (EPIC-103)`

### Task 4: Frontend — template, dialog impor, hapus draft

**Files:**
- Modify: `frontend/src/features/hris/org-chart/{api.ts,types.ts}`
- Create: `frontend/src/features/hris/org-chart/components/import-version-dialog.tsx`, `import-version-dialog.test.tsx`
- Modify: `frontend/src/features/hris/org-chart/components/version-bar.tsx`, `version-bar.test.tsx`

**Interfaces:**
- Consumes: T2/T3 APIs; `shownVersion`, `VersionBar` (EPIC-102); `ConfirmDialog` (`@/components/ui/confirm-dialog`); the structure of `features/hris/placements/components/branch-code-dialog.tsx`.
- Produces:
  - `types.ts`:
    - `ChartUploadIssue = {line, column, name, message}`;
    - `ChartDiff = {added: string[]; removed: string[]; changed: {code, title, fields: string[]}[]}`;
    - `ChartImportPreview = {label, base: {label}, summary: {nodes, positions, errors, warnings}, errors, warnings, diff, vacancy: ChartVacancyCounts, fingerprint}`.
  - `api.ts`:
    - `orgChartTemplateUrl(version: string): string`;
    - `importOrgChartFile(input: {file: File; label: string; base: string; step: "preview" | "commit"; fingerprint?: string; ack_warnings?: boolean}): Promise<{data: ChartImportPreview | {version: ChartVersion}; message?: string}>`. It throws `Error(message)` on non-2xx; the 403 text is mapped like `approveErrorMessage`, i.e. `Anda tidak punya hak mengimpor versi bagan. Minta admin menambahkan aksi Import pada menu Struktur Org.`
    - `downloadOrgChartImportErrors(input): Promise<Blob>`;
    - `deleteOrgChartVersion(id: string)` via `apiDelete`.
  - `ImportVersionDialog({ open, onOpenChange, base, onImported }: { …; base: string; onImported: (label: string) => void })`:
    - Fields: label input `Label versi` (placeholder `Draft 7`), file input `File template (.xlsx)`.
    - Buttons: **Pratinjau** (disabled without label or file), **Unduh file error** (when errors > 0), checkbox `Saya sudah memeriksa peringatan` (when warnings > 0), **Simpan sebagai draft** (disabled while errors > 0, unchecked warnings, or busy).
    - Preview body:
      - counts `<n> node · <p> posisi`;
      - the error and warning lists (`Baris <line> · <column>: <message>`, line 0 shown as `Bagan`);
      - diff lines `Ditambah <n>`, `Dihapus <n>`, `Diubah <n>` with up to 20 codes each;
      - vacancy counts `Terisi`, `Kosong`, `Mungkin terisi`, `Rencana`.
    - Changing the file or the label clears the preview.
    - On commit: toast the server message, invalidate `orgChartQueryKeys.all`, call `onImported(label)`, close.
  - `VersionBar` gains:
    - link **Unduh template** → `orgChartTemplateUrl(shown.label)`;
    - button **Impor versi** → dialog with `base = shown.label`;
    - for a draft shown version, button **Hapus draft** → `ConfirmDialog` titled `Hapus <label>?`. Confirm calls `deleteOrgChartVersion(id)`, toasts `Versi <label> dihapus`, invalidates, and calls `onChange("")`.

- [ ] **Step 1: Write failing tests.**
  - `import-version-dialog.test.tsx` (mock `../api` and `sonner`, as in `version-bar.test.tsx`):
    - Pratinjau is disabled until both label and file are set.
    - The preview shows `150 node · 149 posisi`, `Ditambah 1`, `RTL-900`, `Baris 152 · Toko: Toko PPMR tidak ada sebagai toko aktif di HRIS`.
    - Simpan is disabled until the warning checkbox is ticked. Clicking it calls `importOrgChartFile` with `{step:"commit", fingerprint:"fp", ack_warnings:true, label:"Draft 7", base:"Draft 6"}` and then `onImported("Draft 7")`.
    - With errors: Simpan is disabled; **Unduh file error** calls `downloadOrgChartImportErrors`.
    - Changing the label after a preview hides the preview.
  - `version-bar.test.tsx` additions:
    - **Unduh template** href contains `/api/hris/org-chart/template` and `version=Draft+6`.
    - **Hapus draft** is shown for the draft only. Confirming calls `deleteOrgChartVersion("v1")` and `onChange("")`.
    - The approved version has no **Hapus draft**.
- [ ] **Step 2: Run to see them fail.** `cd frontend && npx vitest run src/features/hris/org-chart` → missing module or elements.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Gate the frontend.** Run `npx vitest run` (full suite), `NODE_OPTIONS=--max-old-space-size=8192 npx tsc --noEmit`, and `npx eslint src/features/hris/org-chart` → all green.
- [ ] **Step 5: Commit.** `feat(hris): unduh template, impor versi, dan hapus draft bagan (EPIC-103)`

### Task 5: Gate, review, QA staging, epic log

**Files:**
- Modify: `docs/epics/EPIC-103-impor-versi-bagan.md`, `docs/epics/README.md` (add the EPIC-103 row in T5 if it is not there yet; status `review`)

- [ ] **Step 1: Full backend gate** on a freshly migrated test DB: `gofmt -l ./internal ./cmd` (empty), `go vet ./...`, `go test ./internal/... -count=1`. Record the counts.
- [ ] **Step 2: Final whole-branch review** by a fresh reviewer, with the Review Focus above. Fix Critical and Important findings test-first.
- [ ] **Step 3: Staging.** Run `df -h /` first. Back up the staging DB to `/mnt/hdd1/backup/paddydev/staging-before-e103-<stamp>.dump` (inside `(umask 077; …)`), then `umask 022 && bash tools/deploy-paddydev.sh`.
- [ ] **Step 4: QA on staging** using temporary sessions and a temporary read-only role/user (pattern of EPIC-102 QA; staging only):
  1. Download the Draft 6 template, edit it with openpyxl (retitle, add, and remove a node), preview, check the diff, and save as `QA E103 Draft`.
  2. The version appears as Draft. Approve it → 200, then restore. A read-only user gets 403 on import.
  3. Delete the QA draft → 200; deleting Draft 6 after approving it → 409.
  4. Playwright desktop and HP: dialog, preview, no horizontal scroll.
  5. Afterwards Draft 6 is `draft`, and the QA version, role, user, and sessions are removed.
- [ ] **Step 5: Epic log, status, memory.** Check off T1–T5, then write the gate counts, QA results, staging image, and rulings to the Automation Log. Commit `docs: EPIC-103 selesai, hasil gate dan QA staging`.
