# Saklar Nonaktif PPh 21 & BPJS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pengaturan Payroll punya saklar PPh 21, BPJS Ketenagakerjaan, dan BPJS Kesehatan. Saat nonaktif, komponen itu tidak dihitung dan barisnya tidak tampil di slip.

**Architecture:** Tiga kolom boolean di `hris.payroll_settings` dibaca `domain.BuildConfig` ke `Config`. `domain.Calculate` menggabungkan saklar global dengan kepesertaan per struktur sebelum `CalculateBPJS` dan PPh 21. UI menambah saklar di halaman setting dan menyembunyikan baris bernilai 0 di slip.

**Tech Stack:** Go (payroll), PostgreSQL migration, Next.js + Vitest.

**Spec:** `docs/epics/EPIC-082-pph21-bpjs-nonaktif.md` (R1–R7). Acuan gaji: `docs/hris/acuan-mom-20261019/aturan-payroll.md`.

## Global Constraints
- Kolom: `pph21_enabled`, `bpjs_tk_enabled`, `bpjs_kes_enabled` (boolean NOT NULL DEFAULT true). Migrasi meng-UPDATE baris yang ada menjadi false.
- Nonaktif berarti potongan karyawan **dan** iuran perusahaan 0. Tapera dan Subsidi BPJS Kesehatan tidak berubah.
- Saklar menyala = perhitungan identik dengan sebelum EPIC ini.
- Label UI: "PPh 21 dihitung", "BPJS Ketenagakerjaan dihitung", "BPJS Kesehatan dihitung". Catatan: "Nonaktif: tidak dipotong dari gaji dan tidak ada iuran perusahaan. Berlaku untuk payroll yang dihitung setelah disimpan."
- Run yang sudah disetujui/dibayar tidak dihitung ulang.
- Uji Go: `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e071?sslmode=disable' go test -p 1 ./internal/modules/payroll/...`. Setelah migrasi, jalankan `go run ./cmd/migrate -apply` ke DB uji.

## Review Focus
1. **Desember (Pasal 17 tahunan) dengan PPh 21 nonaktif**: PPh Desember = 0, tanpa "koreksi" dari bulan sebelumnya. Diuji di Task 1.
2. **Run draft lama dihitung ulang setelah saklar dimatikan**: hasil baru memakai saklar; run disetujui tidak tersentuh. Diuji di Task 2.
3. **Saklar menyala lagi**: angka identik dengan sebelum EPIC. Diuji di Task 1 dengan tes perbandingan pada input yang sama.
4. **Karyawan tidak ikut BPJS di struktur tapi saklar menyala**: tetap 0. Saklar tidak "memaksa" kepesertaan. Diuji di Task 1.
5. **Setting lama tanpa kolom baru** (misalnya baris dibuat skrip lama, atau GET sebelum migrasi pada fallback TS): `BuildConfig` default true bila kolom tidak ada, dan baru false dari data migrasi. Diuji di Task 1.

---

### Task 1: Saklar di config + kalkulator

**Files:**
- Create: `backend/database/migrations/deltas/20261106090000_payroll_pph21_bpjs_toggle.sql`
- Modify: `backend/internal/modules/payroll/domain/config.go`. `Config` mendapat `Pph21Enabled, BpjsTkEnabled, BpjsKesEnabled bool`; `DefaultConfig` = true semua; `BuildConfig` membaca kolom (absen → true).
- Modify: `backend/internal/modules/payroll/domain/calculator.go:390`.
  - `CalculateBPJS(base+fixed, in.BpjsTkEnrolled && c.BpjsTkEnabled, in.BpjsKesEnrolled && c.BpjsKesEnabled, in.TaperaEnrolled, c)`.
  - PPh 21 (`:440-472`): bila `!c.Pph21Enabled`, maka `pph`, `pphAnnual`, `taxable`, `dpp`, `terRate` = 0, `category` = "", dan `Pph21Method` di hasil = `"nonaktif"`.
- Test: `backend/internal/modules/payroll/domain/pph21_bpjs_toggle_test.go`

**Interfaces:**
- Produces: `Config.Pph21Enabled`, `Config.BpjsTkEnabled`, `Config.BpjsKesEnabled`; setting keys `pph21_enabled`, `bpjs_tk_enabled`, `bpjs_kes_enabled`.

- [ ] **Step 1: Failing tests** `TestToggleOffZeroesTaxAndBpjs`. Input: base 5.000.000, TK/0, taxable, TK + Kes enrolled, method TER, `BpjsKesSubsidyAmount` 100.000 with service > 1 year.
  - Config all off: `Pph21Deduction`, `BpjsTkJhtDeduction`, `BpjsTkJpDeduction`, `BpjsKesDeduction` = 0; `TotalEmployerContribution` = 0; `Pph21Method == "nonaktif"`; `HealthAllowance == 100000`; `NetSalary == GrossSalary − (other deductions)`.
  - Only `BpjsKesEnabled`: Kes deduction = 1% of min(base, cap); TK = 0; PPh = 0.
  - December period with PPh off → 0.
  - `TestToggleOnMatchesDefault`: `Calculate(in, DefaultConfig())` equals `Calculate(in, cfgWithAllTogglesTrue)` field by field. Use `reflect.DeepEqual` on `Result`.
  - Enrolled false + toggle true → still 0.
  - `TestBuildConfigToggleDefaults`: `BuildConfig(Row{}, Row{})` → all true; `{"pph21_enabled": false}` → `Pph21Enabled` false.
- [ ] **Step 2: Run, verify FAIL.**
- [ ] **Step 3: Implement.** Migration: `ALTER TABLE hris.payroll_settings ADD COLUMN IF NOT EXISTS …` for the 3 columns, `COMMENT ON COLUMN` in Indonesian, then `UPDATE hris.payroll_settings SET pph21_enabled = false, bpjs_tk_enabled = false, bpjs_kes_enabled = false;`. Apply to the test DB.
- [ ] **Step 4: Run** `go test ./internal/modules/payroll/...` (domain and integration). Expect PASS. The existing parity/acuan tests must stay green: they build config from defaults or explicit rows. If an integration test reads the live-like settings row and now sees false, set the toggles to true in that test's fixture and say so in the report.
- [ ] **Step 5: Commit** `feat(payroll): saklar PPh 21, BPJS TK, BPJS Kesehatan di kalkulasi (EPIC-082)`.

### Task 2: API setting + audit + run integration

**Files:**
- Modify: `backend/internal/modules/payroll/settings.go:48-75`. Add `{key: "pph21_enabled", kind: "bool"}, {key: "bpjs_tk_enabled", kind: "bool"}, {key: "bpjs_kes_enabled", kind: "bool"}` to `settingsFields`. Check that `saveSettings` audits changes the same way as other fields; if setting changes are not audited today, add an audit row `payroll_settings.update` with before/after of changed keys.
- Test: `backend/internal/modules/payroll/pph21_bpjs_toggle_integration_test.go`

- [ ] **Step 1: Failing integration test** `TestPayrollToggleSettingsAndRun`:
  - `PUT` settings with `pph21_enabled: false, bpjs_tk_enabled: false, bpjs_kes_enabled: false` → 200, `GET` returns them.
  - Create a draft run for one employee (reuse the nearest existing run-creation helper in this package) → item `pph21_deduction`, BPJS deductions and employer contributions = 0.
  - Switch all on and recalculate the draft → values > 0 and equal to a run computed before the change in the same test.
  - An approved run created before the switch keeps its stored numbers after the switch (no recalculation).
- [ ] **Step 2: Run, verify FAIL; implement; PASS.**
- [ ] **Step 3: Commit** `feat(payroll): simpan saklar PPh 21/BPJS di Pengaturan Payroll (EPIC-082)`.

### Task 3: UI Pengaturan Payroll + slip

**Files:**
- Modify: `frontend/src/features/hris/payroll/components/payroll-settings-page.tsx` (form type/payload in the same feature's `api.ts`/types).
  - 3 switches (the switch/checkbox control this page already uses for boolean settings such as `thr_prorate`; if none, the shared `Switch` from `@/components/ui`) above the BPJS and PPh 21 sections, with the exact labels and note from Global Constraints.
  - Rate inputs of a disabled part are `disabled` (values kept).
- Modify: `frontend/src/features/hris/payroll/components/payslip-page.tsx:189-193`, `frontend/src/features/hris/ess/components/payslip-detail-dialog.tsx`, `frontend/src/features/hris/payroll/components/payroll-detail-page.tsx`. Hide PPh 21 / BPJS TK (JHT, JP) / BPJS Kesehatan rows (and employer-contribution blocks) whose value is 0. `payslip_pdf.go` already hides zero rows; if its employer-contribution section prints with total 0, hide it too.
- Modify: `frontend/src/features/hris/salary/components/new-salary-page.tsx`, `edit-salary-page.tsx`. Next to the BPJS TK / BPJS Kes / taxable controls, show the muted note "Dinonaktifkan di Pengaturan Payroll" when the matching global toggle is off (read the settings query already used elsewhere; do not add an endpoint).
- Test: the nearest existing test files for these components (create `payroll-settings-page.test.tsx` and `payslip-page.test.tsx` if missing).

- [ ] **Step 1: Failing vitest:**
  - settings page renders the 3 switches with the exact labels; toggling and saving sends `pph21_enabled: false`; the BPJS rate inputs are disabled when `bpjs_tk_enabled` is false.
  - payslip page with `pph21_deduction: 0, bpjs_* : 0` shows no "PPh 21" / "BPJS" text; with values > 0 it shows them.
  - ESS payslip dialog: same.
- [ ] **Step 2: Implement; vitest + tsc + eslint; PASS.** HP 360 px: switches wrap, no horizontal scroll (class-based reasoning; staging QA confirms).
- [ ] **Step 3: Commit** `feat(payroll): UI saklar PPh 21/BPJS dan slip tanpa baris nonaktif (EPIC-082)`.

### Task 4: Acuan, gates, staging

- [ ] Docs:
  - `docs/hris/acuan-mom-20261019/aturan-payroll.md`: PPh 21 row + "Setting" table, with the 3 toggles and live values false.
  - `status-requirement.md`: PY row for PPh 21/BPJS.
  - `pertanyaan-rekomendasi.md`: row "PPH-BPJS (02-11)" with the 4 answers.
  - `.claude/skills/paddy-hris-acuan/SKILL.md`: payroll rule line.
  - `docs/epics/README.md`: EPIC-082 row; epic status `review`.
- [ ] Gates:
  - Go: gofmt, `go vet ./...`, `go test ./internal/... -count=1` (parallel, as CI).
  - Frontend: tsc, eslint, `npx vitest run`.
- [ ] Staging: back up the DB, then deploy the branch. Simulate the November 2026 draft run before/after on staging and record the totals in the Automation Log: total PPh 21, total BPJS karyawan, total iuran perusahaan, total THP.
- [ ] PR to `main`. Production only with explicit user approval (`alur-migrasi-production`). The rehearsal shows the run totals before/after on a prod copy.
