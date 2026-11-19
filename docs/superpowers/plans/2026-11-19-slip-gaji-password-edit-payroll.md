# EPIC-118 Slip Gaji Ringkas, Password ESS, dan Edit Payroll per Karyawan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR bisa mengedit dan menghitung ulang payroll satu karyawan dari tabel run, slip gaji tampil ringkas tanpa kode R- dan tanpa rincian per toko, dan karyawan harus memasukkan password login sebelum angka atau PDF slipnya terbuka di ESS (slip terbit sejak run Disetujui).

**Architecture:** Backend Go (`apps/backend/internal/modules/payroll`) mendapat endpoint hitung ulang satu karyawan (loop `calculateRun` dipecah jadi fungsi per karyawan) dan kunci slip per sesi login (`hris.payslip_unlocks`, password diperiksa lewat `platform/auth`). Frontend Next (`apps/frontend`) menghapus kode R- dan blok toko/kartu dari tampilan slip, menambah dialog Edit per baris run draft, dan menambah dialog password di ESS Slip Gaji. PDF slip (`payslip_pdf.go`) ikut dirapikan.

**Tech Stack:** Go + pgx + PostgreSQL 16, Next.js (baca `apps/frontend/AGENTS.md` dulu), React Query, Vitest, golang.org/x/crypto/bcrypt.

**Spec:** permintaan user 19-11-2026 dan jawaban AskUserQuestion di sesi yang sama:
- Edit per karyawan: "Komponen + hitung ulang". Dialog per karyawan untuk komponen manual; simpan langsung menghitung ulang karyawan itu saja.
- Password: "Rincian dan unduh". Daftar slip ESS hanya menampilkan periode; angka, rincian, dan PDF terbuka sesudah password login benar; server yang memeriksa.
- Rilis slip ESS: "Sejak Disetujui", yaitu status run `completed` atau `paid`.
- Kode R-: "Semua tampilan". Slip HR/ESS, PDF, judul kolom tabel run, rekap baris slip, label komponen manual. Ekspor CSV tetap memakai kode.
- Slip HR: hapus "Rincian per toko" dan empat kartu Hari Kerja/Hadir/Terlambat/Cuti (Unpaid); info Cuti dibayar dan Cuti tanpa bayaran ada di ringkasan atas.

## Global Constraints

- Repo erp-paddy, worktree `/mnt/hdd1/project/apps/paddydev/.worktrees/erp-epic118`, branch `feat/epic118-slip-gaji` dari `origin/development` (56fab25). Jangan push langsung ke `development`; lewat PR.
- Hormati `.gitignore` erp-paddy: tidak ada docs dan seeder di repo ini. Dokumen epic ada di repo paddydev (`.worktrees/rilis-prod/docs/epics/`).
- Test DB: `CREATE DATABASE epic118_test TEMPLATE epic117_test` di 127.0.0.1:55499; salin `.superpowers/env.sh` dari worktree erp-epic117 ke worktree ini dan ganti nama DB. Go test dijalankan dengan `-p 1` (DB bersama).
- Migration baru di `apps/backend/database/migrations/deltas/`: `20261219090000_payroll_runs_include_thr.sql`, `20261219090100_payslip_unlocks.sql`.
- Teks UI Bahasa Indonesia. Pesan error persis seperti ditulis di task.
- Masa buka slip: 15 menit per sesi login. Batas percobaan password: 5 kali per 15 menit per akun.
- Status run: `draft → processing → completed → paid`. "Disetujui" = `completed`.
- Sesudah menambah route Go: `make -C apps/backend generate` untuk `apps/frontend/src/lib/go-routes.generated.json`, lalu perbarui allowlist `apps/frontend/src/lib/backend-routes.test.ts` bila tesnya meminta.
- Jangan ubah data staging/production.

## Review Focus

1. Sesi "Login As" HR ke akun karyawan: membuka slip butuh password karyawan itu, jadi HR tidak bisa membaca slip lewat impersonasi. Tes di Task 7.
2. Akun yang sama login di dua perangkat: membuka slip di perangkat A tidak membuka perangkat B. Tes di Task 7.
3. Masa buka habis (lebih dari 15 menit): daftar kembali terkunci dan PDF ditolak. Tes di Task 7.
4. Hitung ulang satu karyawan pada run yang dihitung dengan THR: THR karyawan itu tetap ada. Tes di Task 4.
5. Hitung ulang satu karyawan menghasilkan total run yang sama dengan hitung ulang seluruh run. Tes di Task 4.

---

### Task 1: Hapus kode R- dari tampilan payroll di frontend

**Files:**
- Modify: `apps/frontend/src/features/hris/payroll/components/slip-lines-view.tsx` (span kode di `LineRow` baris 41; padding cicilan `pl-[4.5rem]` jadi `pl-4`)
- Modify: `apps/frontend/src/features/hris/payroll/components/slip-recap-card.tsx` (kolom "Kode")
- Modify: `apps/frontend/src/features/hris/payroll/components/payroll-detail-page.tsx` (baris 179, 287-289)
- Modify: `apps/frontend/src/lib/payroll/manual-inputs.ts` (label baris 16-24)
- Test: `slip-lines-view.test.tsx`, `slip-recap-card.test.tsx`, `payroll-detail-page.test.tsx`, `apps/frontend/src/lib/payroll/manual-inputs.test.ts` (buat bila belum ada)

**Interfaces:**
- Produces: `SlipLinesView` tetap `({ slip, showSource })`. Atribut `data-slip-line` dan `data-recap-code` tetap ada (tidak terlihat, dipakai tes). Label manual baru: `Insentif`, `Tunjangan lain kena pajak`, `Tunjangan lain tidak kena pajak`, `Denda`, `Potongan lain`.

- [ ] **Step 1: Tulis tes yang gagal**

```ts
// slip-lines-view.test.tsx
it("tidak menampilkan kode rule R- di slip", () => {
  const { container } = render(<SlipLinesView slip={fixtureSlip} showSource />);
  expect(container.textContent).not.toMatch(/R-\d/);
  expect(screen.getByText("Take Home Pay")).toBeInTheDocument();
});
// slip-recap-card.test.tsx (totals memuat R-01 dan R-90c)
it("rekap baris slip tanpa kolom Kode", () => {
  expect(screen.queryByRole("columnheader", { name: "Kode" })).toBeNull();
  expect(container.textContent).not.toMatch(/R-\d/);
});
// payroll-detail-page.test.tsx
it("judul kolom tabel run tanpa kode R-", () => {
  expect(screen.getByRole("columnheader", { name: "Total Pendapatan" })).toBeInTheDocument();
  expect(screen.getByRole("columnheader", { name: "Take Home Pay" })).toBeInTheDocument();
  expect(document.body.textContent).not.toMatch(/R-90/);
});
// manual-inputs.test.ts
it("label komponen manual tanpa kode R-", () => {
  for (const c of MANUAL_INPUT_CODES) expect(c.label).not.toMatch(/R-\d/);
  expect(manualInputLabel("INCENTIVE")).toBe("Insentif");
  expect(manualInputLabel("OTHER_DEDUCTION")).toBe("Potongan lain");
});
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd apps/frontend && pnpm exec vitest run src/features/hris/payroll/components/slip-lines-view.test.tsx src/features/hris/payroll/components/slip-recap-card.test.tsx src/features/hris/payroll/components/payroll-detail-page.test.tsx src/lib/payroll/manual-inputs.test.ts`
Expected: FAIL, teks masih memuat `R-01`/`R-90a` dan label `Insentif (R-06)`.

- [ ] **Step 3: Implementasi** Hapus span kode dan kolom Kode. Ganti judul kolom/kartu jadi "Total Pendapatan", "Total Potongan", "Take Home Pay". Ganti label manual sesuai Interfaces. Tes lama yang mencari teks kode diubah memakai `data-slip-line`/`data-recap-code`. `ui-run-csv.ts` tidak diubah.

- [ ] **Step 4: Jalankan lagi**

Run: perintah Step 2, lalu `cd apps/frontend && pnpm exec vitest run src/features/hris src/lib/payroll`
Expected: PASS semua.

- [ ] **Step 5: Commit**

```bash
git add apps/frontend/src/features/hris/payroll apps/frontend/src/lib/payroll
git commit -m "feat(payroll): sembunyikan kode rule R- di slip, rekap, tabel run, dan komponen manual (EPIC-118)"
```

### Task 2: Slip HR tanpa Rincian per toko dan tanpa empat kartu bawah

**Files:**
- Modify: `apps/frontend/src/features/hris/payroll/components/payslip-page.tsx` (hapus `<SlipStoreBreakdown>` baris 142 dan grid kartu baris 145-162)
- Modify: `apps/frontend/src/features/hris/payroll/components/store-breakdown.tsx` (hapus `SlipStoreBreakdown`; export lain tetap)
- Test: `apps/frontend/src/features/hris/payroll/components/payslip-page.test.tsx`

**Interfaces:**
- Consumes: `AttendanceSummary` di `slip-lines-view.tsx` sudah memuat "Cuti dibayar" dan "Cuti tanpa bayaran" (EPIC-117), tidak diubah.

- [ ] **Step 1: Tulis tes yang gagal**

```tsx
it("slip HR tanpa Rincian per toko dan tanpa kartu Hari Kerja/Hadir/Terlambat/Cuti (Unpaid)", () => {
  // fixture slip_version 2 dengan 2 allocations
  expect(screen.queryByText("Rincian per toko")).toBeNull();
  expect(screen.queryByText("Hari Kerja")).toBeNull();
  expect(screen.queryByText("Cuti (Unpaid)")).toBeNull();
});
it("ringkasan atas memuat cuti dibayar dan tanpa bayaran sebelum Pendapatan", () => {
  const summary = screen.getByText("Ringkasan kehadiran");
  const income = screen.getByText("Pendapatan");
  expect(summary.compareDocumentPosition(income) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(screen.getByText("Cuti dibayar")).toBeInTheDocument();
  expect(screen.getByText("Cuti tanpa bayaran")).toBeInTheDocument();
});
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd apps/frontend && pnpm exec vitest run src/features/hris/payroll/components/payslip-page.test.tsx`
Expected: FAIL pada tes pertama ("Rincian per toko" masih ada).

- [ ] **Step 3: Implementasi** Hapus dua blok, export `SlipStoreBreakdown`, dan import-nya. Slip format lama (`slip == null`) tidak mendapat pengganti, karena staging dan production hanya punya slip versi 2.

- [ ] **Step 4: Jalankan lagi**

Run: perintah Step 2, lalu `pnpm exec vitest run src/features/hris/payroll`
Expected: PASS.

- [ ] **Step 5: Commit** `feat(payroll): slip HR tanpa rincian per toko dan kartu kehadiran bawah (EPIC-118)`

### Task 3: PDF slip tanpa kode R- dan tanpa Rincian per Toko

**Files:**
- Modify: `apps/backend/internal/modules/payroll/payslip_pdf.go` (kolom kode di `row` baris 281-300, prefix `thp.code` baris 377-378, `section("Rincian per Toko")` baris 337, `slipStoreRows` baris 612+, field `showStores`/`stores`, baris 64)
- Test: `apps/backend/internal/modules/payroll/payslip_sections_test.go`

**Interfaces:**
- Produces: `buildPayslipPDF(slip payslipDoc) ([]byte, error)` tetap. `payslipDoc` tanpa `showStores` dan `stores`; query toko untuk PDF dihapus.

- [ ] **Step 1: Tulis tes yang gagal** `TestPayslipPDFHasNoRuleCodes`: bangun `payslipDoc` versi 2 dengan helper yang sudah ada di file tes, ambil teks PDF dengan helper teks yang dipakai tes section, lalu assert tidak ada `R-01`, `R-90a`, `R-90c`, dan ada `Take Home Pay`. `TestPayslipPDFHasNoStoreSection`: slip dengan dua toko, teks tidak memuat `Rincian per Toko`.

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd apps/backend && source ../../.superpowers/env.sh && go test ./internal/modules/payroll/ -run 'TestPayslipPDFHasNo' -p 1`
Expected: FAIL, teks memuat `R-01`.

- [ ] **Step 3: Implementasi** `row` tidak lagi menulis kode, dan versi 2 memakai lebar label penuh seperti format lama. Label THP tanpa prefix kode. Hapus seluruh jalur toko. Ringkasan kehadiran PDF (Cuti dibayar, Cuti tanpa bayaran) tetap.

- [ ] **Step 4: Jalankan lagi**

Run: `go test ./internal/modules/payroll/ -p 1`
Expected: PASS semua. Tes lama yang mengharapkan kode atau toko di PDF disesuaikan.

- [ ] **Step 5: Commit** `feat(payroll): PDF slip tanpa kode rule dan rincian per toko (EPIC-118)`

### Task 4: Backend hitung ulang satu karyawan

**Files:**
- Create: `apps/backend/database/migrations/deltas/20261219090000_payroll_runs_include_thr.sql` berisi `ALTER TABLE hris.payroll_runs ADD COLUMN IF NOT EXISTS include_thr boolean NOT NULL DEFAULT false;`
- Modify: `apps/backend/internal/modules/payroll/service_runs.go` (`calculateRun` baris 391-545), `postgres_runs.go`, `http_runs.go`, `module.go`
- Test: `apps/backend/internal/modules/payroll/recalc_employee_integration_test.go`

**Interfaces:**
- Produces:
  - `POST /api/hris/payroll/{id}/employees/{employeeId}/calculate` → 200 `{ data: <detail dengan employee>, summary: { skipped: [...] }, message }`. Guard `RequireMenuPrefix(r, iam.HrisCompensation...)` seperti `calculateRun`.
  - `func (s *service) calculateEmployee(ctx context.Context, runID, employeeID string) (*calcResult, error)`
  - `func refreshRunTotals(ctx context.Context, q database.Querier, runID string, now time.Time) error`: jumlah dari `hris.payroll_details` run itu, kolom sama dengan `updateRunTotals`.
  - `calculateRun` menyimpan `include_thr` ke run.
- Error: run bukan draft → 400 `Hanya payroll draft yang bisa dihitung`. Karyawan di luar cakupan run (`PayrollEmployees` periode) → 404 `Karyawan tidak termasuk payroll periode ini`. Karyawan tanpa struktur gaji entitas run → 200 dengan `summary.skipped` beralasan sama seperti `calculateRun`, dan detail lamanya dihapus.

- [ ] **Step 1: Tulis tes yang gagal**

```go
func TestRecalcEmployeeMatchesFullRun(t *testing.T)        // run draft 3 karyawan; INCENTIVE 500000 utk A; calculateEmployee(A);
                                                           // simpan totals; calculateRun penuh; totals run dan detail A sama persis
func TestRecalcEmployeeLeavesOthersUntouched(t *testing.T) // id dan updated_at detail B dan C tidak berubah
func TestRecalcEmployeeKeepsThr(t *testing.T)              // calculateRun include_thr=true; calculateEmployee(A) → thr A > 0 dan sama
func TestRecalcEmployeeRefusesNonDraft(t *testing.T)       // status processing → 400 "Hanya payroll draft yang bisa dihitung"
func TestRecalcEmployeeOutsideRun(t *testing.T)            // karyawan resign sebelum periode → 404 "Karyawan tidak termasuk payroll periode ini"
func TestRecalcEmployeeRewritesAllocations(t *testing.T)   // alokasi toko A dihitung ulang, jumlahnya = gross A
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `go test ./internal/modules/payroll/ -run TestRecalcEmployee -p 1`
Expected: FAIL kompilasi, `calculateEmployee` belum ada.

- [ ] **Step 3: Implementasi** Pecah isi loop `calculateRun` menjadi `func (s *service) calcOne(ctx context.Context, env *calcEnv, e EmployeeBrief, out *calcResult) (*domain.Result, error)`. `calcEnv` memuat run, config, period, holidays, company, branchCompany, closures, elsewhere, manual, opts, dan includeThr. `calculateRun` memakai `calcOne` tanpa perubahan perilaku. `calculateEmployee` membangun `calcEnv` yang sama (includeThr dari kolom run), mencari karyawan di `PayrollEmployees`, menghapus detail lama karyawan itu (`DELETE FROM hris.payroll_details WHERE payroll_run_id=$1 AND employee_id=$2`; alokasi ikut terhapus lewat FK), memanggil `calcOne`, lalu `refreshRunTotals`, semuanya dalam satu `database.WithTx`.

- [ ] **Step 4: Jalankan lagi**

Run: `go test ./internal/modules/payroll/ -p 1`
Expected: PASS semua, termasuk tes `calculateRun` lama.

- [ ] **Step 5: Regenerasi route dan commit**

```bash
make -C apps/backend generate
git add apps/backend apps/frontend/src/lib/go-routes.generated.json
git commit -m "feat(payroll): hitung ulang payroll satu karyawan di run draft (EPIC-118)"
```

### Task 5: Dialog Edit per karyawan di tabel run

**Files:**
- Create: `apps/frontend/src/features/hris/payroll/components/employee-payroll-dialog.tsx`
- Modify: `apps/frontend/src/features/hris/payroll/api.ts`, `mutations.ts`, `components/payroll-detail-page.tsx` (kolom Aksi baris 324-350)
- Test: `apps/frontend/src/features/hris/payroll/components/employee-payroll-dialog.test.tsx`, `payroll-detail-page.test.tsx`

**Interfaces:**
- Consumes: endpoint Task 4; `usePayrollInputs(runId)`, `useSavePayrollInput(runId)`, `useDeletePayrollInput(runId)`, `MANUAL_INPUT_CODES`, `parseAmountID`, `ZERO_ALLOWED_CODES`.
- Produces: `recalculateEmployee(runId: string, employeeId: string)` di `api.ts`; `useRecalculateEmployee(runId)` di `mutations.ts` (invalidate query run dan inputs); `<EmployeePayrollDialog runId employee={{ id, full_name, nip }} open onOpenChange />`.

- [ ] **Step 1: Tulis tes yang gagal**

```tsx
it("tombol Edit hanya muncul saat run draft", () => { /* draft → 1 tombol Edit per baris; completed → 0 */ });
it("dialog menampilkan komponen manual karyawan itu saja", () => { /* inputs A dan B; buka A → hanya baris A */ });
it("Simpan & hitung ulang menyimpan komponen lalu menghitung ulang karyawan", async () => {
  // INCENTIVE, nominal "500.000" → PUT inputs {employee_id: A, code: "INCENTIVE", amount: 500000}
  // lalu POST /api/hris/payroll/run-1/employees/A/calculate; toast "Payroll <nama A> dihitung ulang"
});
it("hapus komponen lalu hitung ulang", async () => { /* DELETE inputs lalu POST calculate */ });
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd apps/frontend && pnpm exec vitest run src/features/hris/payroll/components/employee-payroll-dialog.test.tsx src/features/hris/payroll/components/payroll-detail-page.test.tsx`
Expected: FAIL, komponen belum ada.

- [ ] **Step 3: Implementasi** Baris run draft mendapat tombol `Edit` (ikon `PencilSquareIcon`) di samping `Slip`. Dialog berjudul `Edit payroll · <nama>` dan berisi: daftar komponen manual karyawan (label, nominal, catatan, tombol hapus); form tambah (kode, nominal, catatan) tanpa pilihan karyawan; tombol `Simpan & hitung ulang`; tautan `Ubah gaji pokok/tunjangan` ke `/dashboard/hris/salary`. Hapus komponen juga langsung menghitung ulang. Tombol nonaktif selama mutasi berjalan.

- [ ] **Step 4: Jalankan lagi**

Run: perintah Step 2, lalu `pnpm exec vitest run src/features/hris/payroll && pnpm exec tsc --noEmit -p . && pnpm exec eslint src/features/hris/payroll`
Expected: PASS, tanpa error tipe dan lint.

- [ ] **Step 5: Commit** `feat(payroll): edit komponen dan hitung ulang per karyawan dari tabel run (EPIC-118)`

### Task 6: Slip terbit di ESS sejak run Disetujui

**Files:**
- Modify: `apps/backend/internal/modules/payroll/domain/access.go`, `payslips.go` (baris 72-81), `payslip_pdf.go`
- Modify: `apps/frontend/src/features/hris/ess/components/slip-gaji-page.tsx` (teks "sudah dibayarkan")
- Test: `apps/backend/internal/modules/payroll/domain/access_test.go`, `apps/backend/internal/modules/payroll/payslip_release_integration_test.go`

**Interfaces:**
- Produces: `func PayslipReleased(status string) bool`, true untuk `completed` dan `paid`.

- [ ] **Step 1: Tulis tes yang gagal**

```go
func TestPayslipReleased(t *testing.T)                     // draft false, processing false, completed true, paid true
func TestMyPayslipsListsCompletedRun(t *testing.T)         // run completed → employee_id=me memuat 1 baris slip itu (terkunci atau tidak)
func TestOwnPayslipPDFRefusedBeforeApproval(t *testing.T)  // run processing → karyawan 404 "Slip gaji tidak ditemukan"; HR Penggajian 200
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `go test ./internal/modules/payroll/... -run 'TestPayslipReleased|TestMyPayslipsListsCompletedRun|TestOwnPayslipPDFRefusedBeforeApproval' -p 1`
Expected: FAIL.

- [ ] **Step 3: Implementasi** Filter me-view memakai `domain.PayslipReleased`. PDF untuk pemohon tanpa `PayslipFullAccess` membalas 404 `Slip gaji tidak ditemukan` bila run belum terbit. Teks ESS: "Slip gaji Anda dari periode yang sudah disetujui" dan "Slip muncul di sini setelah payroll periode berjalan disetujui HR." Notifikasi WA tetap hanya untuk `paid`.

- [ ] **Step 4: Jalankan lagi** `go test ./internal/modules/payroll/... -p 1` dan `pnpm exec vitest run src/features/hris/ess`. Expected: PASS.

- [ ] **Step 5: Commit** `feat(payroll): slip ESS terbit sejak run disetujui (EPIC-118)`

### Task 7: Kunci slip ESS dengan password login (backend)

**Files:**
- Create: `apps/backend/database/migrations/deltas/20261219090100_payslip_unlocks.sql`, `apps/backend/internal/modules/payroll/payslip_unlock.go`
- Modify: `apps/backend/internal/platform/auth/auth.go`, `payroll/payslips.go`, `payroll/payslip_pdf.go`, `payroll/module.go`
- Modify: `apps/frontend/src/lib/payroll/payslips.ts`, `apps/frontend/src/app/api/hris/payslips/[id]/pdf/route.ts` (fallback Next)
- Test: `apps/backend/internal/modules/payroll/payslip_unlock_integration_test.go`, `apps/backend/internal/platform/auth/auth_integration_test.go`, `apps/frontend/src/lib/payroll/payslips.test.ts`

**Interfaces:**
- Migration:

```sql
CREATE TABLE IF NOT EXISTS hris.payslip_unlocks (
  session_token_hash text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
```

- `func (s *Service) VerifyPassword(ctx context.Context, userID, password string) (bool, error)` di `platform/auth`: membaca `COALESCE(password_hash,'')` dari `auth.users`. Hash kosong tidak pernah cocok, dan bcrypt hanya membaca 72 byte pertama seperti `identity.verifyPassword`.
- `POST /api/hris/payslips/unlock` body `{ password: string }`:
  - 200 `{ data: { unlocked_until: <ISO> } }`: upsert baris untuk `auth.HashToken(auth.ReadSessionToken(r))` dan hapus baris kedaluwarsa.
  - 400 `Password salah` bila tidak cocok.
  - 400 `Buka slip gaji hanya dari sesi login` bila tanpa cookie sesi (Bearer API token).
  - 429 `Terlalu banyak percobaan password, coba lagi 15 menit lagi` mulai percobaan ke-6 dalam 15 menit (`ratelimit`, kunci `payslip_unlock_<userID>`, setiap percobaan dihitung).
- `func (s *service) payslipUnlocked(ctx context.Context, r *http.Request, userID string) (bool, error)`.
- `GET /api/hris/payslips` me-view tanpa unlock aktif: tiap baris hanya `{ id, payroll_run_id, payroll_run: { id, run_name, period_month, period_year, status, paid_at, company_name }, locked: true }`. Dengan unlock: baris penuh plus `locked: false`. Daftar HR penuh tidak berubah.
- `GET /api/hris/payslips/{id}/pdf` untuk pemohon tanpa `PayslipFullAccess` dan tanpa unlock aktif: 403 `Masukkan password akun Anda untuk membuka slip gaji`.
- Fallback Next: `listPayslips` me-view mengembalikan `[]`, dan route PDF menolak karyawan dengan 403 berpesan sama, supaya kunci tidak bisa dilewati saat `BACKEND_URL` kosong.

- [ ] **Step 1: Tulis tes yang gagal**

```go
func TestVerifyPassword(t *testing.T)                     // benar true, salah false, hash kosong false
func TestMyPayslipsLockedWithoutUnlock(t *testing.T)      // locked:true, tanpa net_salary, gross_salary, slip
func TestUnlockOpensPayslipsForSession(t *testing.T)      // unlock benar → list penuh locked:false, PDF 200
func TestUnlockWrongPassword(t *testing.T)                // 400 "Password salah", list tetap terkunci
func TestUnlockRateLimited(t *testing.T)                  // percobaan ke-6 dalam 15 menit → 429
func TestUnlockBoundToSession(t *testing.T)               // sesi A unlock; sesi B akun sama tetap locked
func TestUnlockExpires(t *testing.T)                      // jam svc maju 16 menit → locked lagi, PDF 403
func TestUnlockLoginAsNeedsEmployeePassword(t *testing.T) // sesi impersonasi: password HR → 400; password karyawan → 200
func TestUnlockRefusesAPIToken(t *testing.T)              // Bearer token → 400 "Buka slip gaji hanya dari sesi login"
func TestOwnPDFLockedWithoutUnlock(t *testing.T)          // karyawan → 403 pesan kunci; HR Penggajian unduh slip orang lain → 200 tanpa unlock
```

```ts
// payslips.test.ts
it("fallback Next tidak membuka slip me-view", async () => {
  expect(await listPayslips(client, employeeActor, { employee_id: "me" })).toEqual([]);
});
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `go test ./internal/platform/auth/ ./internal/modules/payroll/ -run 'TestVerifyPassword|Unlock|Locked' -p 1`
Expected: FAIL kompilasi.

- [ ] **Step 3: Implementasi** sesuai Interfaces. Waktu diambil dari `s.now()` supaya tes kedaluwarsa bisa memajukan jam.

- [ ] **Step 4: Jalankan lagi**

Run: `go test ./internal/platform/auth/ ./internal/modules/payroll/... -p 1` dan `cd apps/frontend && pnpm exec vitest run src/lib/payroll`
Expected: PASS semua.

- [ ] **Step 5: Regenerasi route dan commit**

```bash
make -C apps/backend generate
git add apps/backend apps/frontend/src/lib apps/frontend/src/app/api/hris/payslips
git commit -m "feat(payroll): slip gaji ESS terkunci password login per sesi (EPIC-118)"
```

### Task 8: ESS Slip Gaji dengan dialog password

**Files:**
- Create: `apps/frontend/src/features/hris/ess/components/payslip-unlock-dialog.tsx`
- Modify: `apps/frontend/src/features/hris/ess/api.ts`, `queries.ts`, `types.ts`, `components/slip-gaji-page.tsx`, `components/payslip-detail-dialog.tsx`
- Test: `apps/frontend/src/features/hris/ess/components/slip-gaji-page.test.tsx`, `payslip-unlock-dialog.test.tsx`

**Interfaces:**
- Consumes: endpoint Task 7.
- Produces: `unlockPayslips(password: string) => Promise<{ unlocked_until: string }>`; `useUnlockPayslips()` (sukses → invalidate `useMyPayslips`); `EssPayslip.locked?: boolean`.

- [ ] **Step 1: Tulis tes yang gagal**

```tsx
it("slip terkunci hanya menampilkan periode dan tombol Buka slip", () => {
  // data [{ id, payroll_run: {...}, locked: true }]
  expect(screen.getByText("November 2026")).toBeInTheDocument();
  expect(screen.queryByText(/Rp/)).toBeNull();
  expect(screen.getByRole("button", { name: "Buka slip" })).toBeInTheDocument();
});
it("password benar membuka slip", async () => { /* POST unlock {password:"rahasia"} → refetch → THP tampil */ });
it("password salah menampilkan pesan server", async () => { /* 400 → "Password salah" di dialog, input tetap */ });
it("input bertipe password dan kosong lagi sesudah dialog ditutup", () => {});
```

- [ ] **Step 2: Jalankan, pastikan gagal**

Run: `cd apps/frontend && pnpm exec vitest run src/features/hris/ess/components/slip-gaji-page.test.tsx src/features/hris/ess/components/payslip-unlock-dialog.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implementasi** Kartu slip terkunci berisi periode, ikon gembok, teks "Masukkan password untuk melihat", dan tombol `Buka slip` yang membuka dialog. Dialog berjudul `Buka slip gaji`, keterangan "Masukkan password akun Anda. Slip terbuka 15 menit di perangkat ini.", input `type="password"` dengan `autoComplete="current-password"`, tombol `Buka`. Sesudah sukses, dialog tertutup dan daftar memuat angka; klik kartu membuka `PayslipDetailDialog` seperti sekarang. Di detail, ganti `<a download>` dengan tombol yang `fetch` PDF: status 403 membuka dialog password lagi, status 200 mengunduh blob.

- [ ] **Step 4: Jalankan lagi**

Run: perintah Step 2, lalu `pnpm exec vitest run src/features/hris && pnpm exec tsc --noEmit -p . && pnpm exec eslint src/features/hris/ess`
Expected: PASS.

- [ ] **Step 5: Commit** `feat(ess): slip gaji minta password sebelum dibuka atau diunduh (EPIC-118)`

### Task 9: Gerbang akhir dan dokumen epic

**Files:**
- Create (repo paddydev, `.worktrees/rilis-prod`): `docs/epics/EPIC-118-slip-gaji-password-edit-payroll.md`; Modify: `docs/epics/README.md`

- [ ] **Step 1: Suite penuh** `cd apps/backend && go build ./... && go vet ./internal/modules/payroll/... ./internal/platform/auth/... && go test ./internal/modules/payroll/... ./internal/platform/auth/... -p 1`; `cd apps/frontend && pnpm exec tsc --noEmit -p . && pnpm exec eslint src/features/hris src/lib/payroll && pnpm test`. Expected: PASS. Kegagalan yang bukan dari perubahan ini dicatat dengan nama tesnya.
- [ ] **Step 2: Migration** Terapkan dua migration ke `epic118_test` lewat runner (`-apply` lalu `-verify`). Expected: 0 drift.
- [ ] **Step 3: Epic doc** Status `review`, tujuan, empat keputusan user 19-11, daftar commit, Automation Log; tautkan rencana ini.
- [ ] **Step 4: Commit dokumen** di repo paddydev: `docs: EPIC-118 slip gaji ringkas, password ESS, edit payroll per karyawan`.
