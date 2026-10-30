# Export Absensi per Toko, Cari & Export Salary, Bug Kontrak & Struktur Gaji — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The attendance export follows the store and late-only filters and carries a Toko column. The Salary page is sorted by name, searchable and exportable. New contracts no longer collide on their number. Adding a salary structure never leaves an employee without an active salary.

**Architecture:** Four independent fixes:
- Go attendance export (filter plus column) with the frontend passing the filters through.
- Salary page UI only (client-side search, sort and xlsx).
- Go contract numbering (max+1 over manual numbers, retry).
- Go salary create (one transaction plus a pre-check for a duplicate effective date).

**Tech Stack:** Go (`backend/internal/modules/{hris,payroll}`, Postgres integration tests); Next.js (Vitest + Testing Library, `@/lib/spreadsheet/exceljs-safe`).

**Spec:** `docs/epics/EPIC-075-export-absensi-salary-kontrak.md` (R1–R9).

## Global Constraints
- Toko column header: `Toko`. Value: `<kode> - <nama>` from `configuration.branches` joined on `hris.attendance.branch_id`, or `-` when there is none. It goes after `Jabatan` in CSV, Excel and PDF.
- `branch_id=none` means attendance without a store (same as `AttendancePage`); `is_late=true` means late only.
- R8 message, verbatim: `Struktur gaji karyawan ini untuk entitas tersebut dengan tanggal efektif <tanggal> sudah ada. Ubah lewat Edit atau pilih tanggal efektif lain.`, where `<tanggal>` is the effective date `YYYY-MM-DD` as sent.
- Contract pattern counted: `^[0-9]+/<TYPE>/[IVXLC]+/<YEAR>$` (TYPE in upper case); at most 5 attempts.
- Go integration tests run with `go test -p 1` (shared DB).

## Review Focus
1. Export with `branch_id` of a store that has no attendance in the period: 404 "No attendance data found", same as today (not an empty file). Test in Task 1.
2. Excel/PDF with a long store name: the column does not break the layout (PDF column widths). Test: the Task 1 PDF still renders (`%PDF`) with a 60-character store name.
3. Salary search on NIP versus name, case-insensitive. Test in Task 3.
4. A contract numbered by hand in another month of the same year (`0570/PKWT/IX/2026`) still counts toward the maximum, because the sequence is per year, not per month. Test in Task 4.
5. A first salary for an employee with no previous version, sent twice in a row on the same date: the second gets the R8 409 and the first stays active. Test in Task 5.

---

### Task 1: Server — export filters + kolom Toko
**Files:** `backend/internal/modules/hris/files.go` (`exportFilter`, `AttendanceExport`), `http_files.go` (`exportAttendance`), `attendance_export.go` (CSV header/rows, `toReportRows`, `attendanceXlsx`, `attendancePDF`, `reportMeta`), `domain/documents.go` (`ReportRow.Branch *string`). Test: `files_integration_test.go` (`TestAttendanceExport`, new `TestAttendanceExportBranchFilter`).
**Produces:** `exportFilter{EmployeeID, StartDate, EndDate, Status, BranchID string; LateOnly bool}`; `reportMeta.BranchLabel *string`. The query adds `LEFT JOIN configuration.branches b ON b.id = a.branch_id`, selects `b.code AS branch_code, b.name AS branch_name`, and handles `branch_id=none` with `a.branch_id IS NULL`.
- [ ] Update `TestAttendanceExport` to expect the `Toko` column (header plus `-` for rows without a store).
- [ ] New `TestAttendanceExportBranchFilter`:
  - Two attendance rows in store A and B, plus one without a store.
  - `branch_id=A` → 1 row, and the Toko column shows `A-code - A-name`.
  - `branch_id=none` → 1 row.
  - `is_late=true` → only the late row.
  - `branch_id` of a store with no data → 404.
  - xlsx with `branch_id=A`: the header contains `Toko`, and the title row contains the store name.
  - PDF with a 60-character store name → body starts with `%PDF`.
- [ ] RED → implement → GREEN.
- [ ] `go test -p 1 ./internal/modules/hris/...`, then commit.

### Task 2: Frontend — export ikut filter
**Files:** `frontend/src/features/hris/attendance/api.ts` (`exportAttendanceCsv` params + `branch_id`, `is_late`), `types.ts` (`AttendanceExportParams`), `components/attendance-recap-tab.tsx` (`handleExport` passes `branch_id: store === ALL_STORES ? undefined : store`, `is_late: lateOnly || undefined`). Test: new `api.export.test.ts`.
- [ ] Test with `fetch` stubbed:
  - `{branch_id: "b1", is_late: true}` → URL contains `branch_id=b1` and `is_late=true`;
  - `branch_id: "all"` or undefined → no `branch_id`.
- [ ] RED → implement → GREEN → commit.

### Task 3: Salary — urut nama, cari, export Excel
**Files:**
- `frontend/src/features/hris/salary/components/salary-page.tsx`
- create `frontend/src/features/hris/salary/salary-export.ts`, holding `salaryExportRows(rows: EmployeeSalary[], entityName: string): SpreadsheetCellValue[][]` (header + rows, R5 column order) and `downloadSalaryXlsx(rows, entityName, today: string): Promise<void>`. The latter uses `buildXlsxBuffer` → Blob → anchor download.
- Tests: `salary-page.test.tsx` and `salary-export.test.ts`.

**Produces:** `sortSalaries(rows)` (name A–Z via `localeCompare("id")`, then NIP) and `matchesSalarySearch(row, q)` (case-insensitive name/NIP), both exported from `salary-export.ts`.
- [ ] Tests:
  - `sortSalaries` orders "budi", "Andi", "Citra" → Andi, budi, Citra.
  - `matchesSalarySearch` finds by NIP `202501` and by name `AND`.
  - `salaryExportRows` header = `["NIP","Nama","Jabatan","Departemen","Entitas","Gaji Pokok","Tunjangan Tetap","Transport","Makan","Tunjangan Jabatan","Tunjangan Kesehatan","Status PTKP","Efektif"]`, amounts are numbers.
  - Page:
    - the search box `Cari nama atau NIP` filters the table;
    - the table order is A–Z;
    - the `Export Excel` button calls `downloadSalaryXlsx` (mocked) with the rows on screen;
    - an empty search shows "Tidak ada karyawan yang cocok".
- [ ] RED → implement → GREEN → commit.

### Task 4: Nomor kontrak max+1
**Files:** `backend/internal/modules/hris/postgres_contracts.go` (`ContractsThisYear` → `MaxContractSeqThisYear(ctx, contractType string, year int) (int, error)`), `contracts.go` (`insertNumbered` loop with up to 5 attempts, seq = max+1, then +1 per collision), the repository interface. Test: new `contract_number_integration_test.go`.
- [ ] Test `TestContractNumberSkipsTakenNumbers`:
  - Insert directly 3 `ODOO-…/PKWT/…/2026` rows, plus `0004/PKWT/X/2026` and `0570/PKWT/IX/2026` (fixed clock 2026-10-05).
  - `POST /api/hris/employees/{id}/contracts` pkwt → 200, number `0571/PKWT/X/2026`.
  - Then a second POST → `0572/PKWT/X/2026`.
  - PKWTT with no manual number yet → `0001/PKWTT/X/2026`.
- [ ] Test `TestContractNumberRetriesOnCollision`: insert `0001/PKWTT/X/2026` with `created_at` in the previous year (not counted in the max), then POST pkwtt → 200 `0002/PKWTT/X/2026`.
- [ ] RED (today the first test gets 409) → implement → GREEN → commit.

### Task 5: Struktur gaji dalam transaksi + tanggal kembar
**Files:** `backend/internal/modules/payroll/salary.go` (`createSalary`). Test: `entity_integration_test.go`, new `TestSalaryDuplicateEffectiveKeepsActive`.
- [ ] Test:
  - POST salary e.co `2026-01-01` → 200.
  - POST again with the same date → 409 with the R8 message (date `2026-01-01`).
  - The first version is still `is_active`, and the active count is 1.
  - POST with date `2026-02-01` → 200, the old one inactive, the new one active.
  - Also: a forced insert failure (an invalid `pay_basis` value that violates a CHECK) → error, and the old version is still active.
- [ ] Implementation:
  - Pre-check `SELECT 1 … WHERE employee_id=$1 AND company_id=$2 AND effective_date=$3::date` → 409 `httpx.Conflict(msg)`.
  - Deactivate + insert inside `pgx.BeginFunc` / the module's existing tx helper.
  - Drop the "log and continue" on a deactivate error (return it).
- [ ] RED → GREEN → `go test -p 1 ./internal/modules/payroll/...` → commit.

### Task 6: Gerbang, QA, dokumen
- [ ] `go vet`; `go test -p 1` for hris+payroll; frontend `tsc`, `lint`, `pnpm test`.
- [ ] Staging: backup, then deploy the branch.
- [ ] QA staging (demo data only, removed afterwards):
  - export filtered by one store (CSV/Excel) has the Toko column and only that store;
  - Salary search and export run on the demo entity;
  - a new PKWT contract for a demo employee → number = max+1 (no 409);
  - double salary with the same date → R8 message, and the old version stays active.
- [ ] Probe Salary + Rekap at laptop and phone widths.
- [ ] Update the Automation Log and README registry, then open the PR.
