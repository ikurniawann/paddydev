# Syarat & Ketentuan Pinjaman dan Kasbon — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Kasbon dan Pinjaman Karyawan mengikuti prosedur 01-11:
- syarat status dan masa kerja, plafon dan cicilan per jenis;
- lampiran wajib (dokumen pendukung / jaminan);
- approval Manager lalu HR untuk keduanya;
- dipotong di payroll periode pengajuan (21–20).

**Architecture:**
- Satu validator domain dengan tabel aturan per jenis, dipakai saat ajukan dan saat HR menyetujui.
- Bulan potong dihitung oleh fungsi domain murni dari tanggal pengajuan WIB, lalu digeser bila run bulan itu sudah selesai.
- Lampiran disimpan di storage privat (pola dokumentasi lembur EPIC-066), di dua kolom array baru `hris.loans`.

**Tech Stack:** Go (`backend/internal/modules/payroll`), Postgres delta migration, Next.js + Vitest.

**Spec:** `docs/epics/EPIC-076-tnc-pinjaman-kasbon.md` (R1–R13).

## Global Constraints
- Aturan per jenis:
  - `kasbon` {label "Kasbon", masa kerja 3 bulan, cicilan maks 2, plafon 50% THP};
  - `loan` {label "Pinjaman Karyawan", 12 bulan, maks 6, 200% THP}.
- Status izin: `contract`, `pkwt`, `permanent` (trim, lower).
- Pesan verbatim ada di spec R2–R5 dan R8.
- Batas periode: tanggal 20, WIB. Run selesai = `completed` atau `paid`.
- Lampiran: PDF/JPG/PNG/WebP ≤ 5 MB, maks 3 per kolom, folder `loan-attachments/<employeeId>/`.
- Tes integrasi Go: `go test -p 1` dengan `TEST_DATABASE_URL`.

## Review Focus
1. Pengajuan 20 Nov 23.30 WIB (= 16.30 UTC) masih periode November. Tes `TestLoanPayrollPeriod`.
2. Pengajuan 21 Des → Januari tahun berikutnya. Tes `TestLoanPayrollPeriod`.
3. Masa kerja tepat di batas (`join_date` 2026-08-01, hari ini 2026-11-01 → 3 bulan, lolos; 2026-08-02 → ditolak). Tes `TestLoanServiceMonths`.
4. Path lampiran milik karyawan lain atau `../` → 400. HR yang mengunggah untuk karyawan X memakai folder X. Tes `TestLoanAttachments`.
5. Pending lama tanpa lampiran atau dengan tenor 12 diputus setelah rilis:
   - aturan TnC divalidasi ulang, sehingga tenor 12 ditolak;
   - lampiran tidak diminta ulang.

   Tes `TestLoanApproveRechecksTnC`.

---

### Task 1: Validator domain
**Files:** `payroll/domain/kasbon.go` (diganti isinya menjadi aturan pinjaman), `domain/kasbon_test.go`, `domain/loans.go`.

**Produces:**
- `type LoanRule struct{ Label string; MinServiceMonths, MaxTenor int; MaxThpPercent float64 }`
- `func LoanRuleFor(loanType string) (LoanRule, bool)`
- `func LoanEligibleStatus(status string) bool`
- `func ServiceMonthsReached(joinDate string, today time.Time, months int) (ok bool, known bool)`
- `type LoanApplicant struct{ Status, JoinDate string }`
- `func ValidateLoanTnC(loanType string, principal float64, tenor int, emp LoanApplicant, today time.Time, thp *float64, activeCount, maxActive float64, showCap bool) string`
- `func LoanPayrollPeriod(submittedAt time.Time) (month, year int)`. Hari (WIB) ≤ 20 → bulan itu, selain itu bulan berikutnya.
- `ValidateKasbon`, `KasbonBlockedStatus`, `KasbonMaxThpPercent`, `ValidateLoanLimits*` dan `FirstInstallmentPeriod` dihapus bila tidak terpakai lagi.

- [ ] Tes tabel `TestValidateLoanTnC`:
  - setiap pesan R2–R5 untuk kedua jenis;
  - batas plafon tepat 50% / 200% lolos;
  - `showCap` menambah "(maks Rp…)".
- [ ] Tes `TestLoanServiceMonths` (Review Focus 3, `join_date` kosong).
- [ ] Tes `TestLoanPayrollPeriod` (Review Focus 1–2).
- [ ] RED → GREEN → commit.

### Task 2: Service — ajukan, setujui, periode, approver
**Files:** `payroll/loans.go`, `kasbon_integration_test.go`, `loan_tnc_integration_test.go` (baru).

- Create:
  - kedua jenis memakai `ValidateLoanTnC` dengan `kasbonTHP` (rename `loanTHP`);
  - approver Manager disimpan untuk kedua jenis.
- HR approve:
  - validasi ulang;
  - `waitingManager` berlaku untuk kedua jenis;
  - bulan potong = `LoanPayrollPeriod(created_at)`, digeser bila `runClosed`;
  - pesan menyebut bulan.
- Pesan Manager: "Kasbon/Pinjaman Karyawan disetujui Manager — menunggu persetujuan HR".

- [ ] Tes integrasi:
  - probation → 400;
  - masa kerja kurang → 400;
  - plafon → 400;
  - loan tenor 6 dan 200% THP + jaminan → 200 dengan `manager_approver_id` terisi;
  - HR approve sebelum Manager → 400;
  - Manager lalu HR → `first_installment` mengikuti `created_at`;
  - run `completed` → bulan berikutnya, run `draft` → tetap.
- [ ] RED → GREEN → `go test -p 1 ./internal/modules/payroll/...` → commit.

### Task 3: Lampiran
**Files:**
- migrasi `database/migrations/deltas/20261101090000_loans_attachments.sql`:
  - `attachments text[] NOT NULL DEFAULT '{}'` dan `collateral_attachments …`;
  - CHECK `cardinality ≤ 3`.
- `payroll/module.go`: `service.files *storage.Store` dari `Ports.Files`, default `storage.FromEnv()`.
- `payroll/loan_files.go` (baru):
  - upload dan baca;
  - `checkLoanAttachments(ownerID, paths, required bool, emptyMsg string)`.
- `loans.go`: create menerima `attachments` dan `collateral_attachments`; `listLoanApprovals` mengembalikan keduanya.
- Rute baru di `goNativePrefixes`, lalu `make -C backend generate`.

- [ ] Tes integrasi `TestLoanAttachments`:
  - kasbon tanpa dokumen → 400;
  - loan tanpa jaminan → 400;
  - path karyawan lain → 400;
  - kasbon dengan `collateral_attachments` → 400.
- [ ] Tes akses baca:
  - pemohon 200;
  - Manager approver 200;
  - HR 200;
  - karyawan lain 403.
- [ ] RED → GREEN → commit.

### Task 4: Frontend
**Files:**
- `lib/hris/loans-view.ts`:
  - `LOAN_RULES`, `loanTenorOptions(type)`, `loanPayrollPeriodLabel(dateIso)`, `loanTermsText(type)`;
  - `validateLoanForm` per jenis + lampiran;
  - `loanAttachmentHref`.
- `features/hris/loans/components/loan-attachments-field.tsx` (baru, dipakai ESS dan HR; upload ke `/api/hris/loans/attachments`).
- `ess/components/pinjaman-page.tsx`:
  - form: syarat, cicilan, perkiraan bulan potong, dua bidang lampiran;
  - detail tim menampilkan lampiran;
  - teks "Persetujuan Kasbon & Pinjaman Tim".
- `loans/components/loan-form-dialog.tsx` dan `loans-table.tsx`: detail menampilkan dokumen dan jaminan, serta perkiraan/bulan potong.
- Banner Persetujuan Tim: "kasbon/pinjaman tim".
- Label `loan` "Pinjaman Karyawan" di `LOAN_TYPE_OPTIONS` dan `domain/payslip.go` `loanTypeLabels`.

- [ ] Tes Vitest:
  - `loans-view` (periode label 20/21 Nov, 21 Des → "Januari 2027"; validasi per jenis);
  - ESS form (syarat tampil, opsi cicilan pinjaman 1–6, lampiran wajib);
  - detail tim (tautan jaminan).
- [ ] RED → GREEN → `pnpm vitest run` area terkait → commit.

### Task 5: Acuan, gerbang, review, QA
- [ ] Acuan:
  - `status-requirement.md` KP-02/KP-03/PY-02;
  - `matriks-approval.md` (Pinjaman Karyawan: Manager → HR);
  - `pertanyaan-rekomendasi.md`;
  - skill `paddy-hris-acuan` baris Kasbon → Kasbon & Pinjaman Karyawan.
- [ ] Gerbang:
  - `go vet ./...`;
  - `go test -p 1` payroll/app;
  - frontend `tsc`, `lint`, `pnpm test`.
- [ ] Review akhir (fresh reviewer, model terkuat), lalu perbaikan.
- [ ] Staging (data demo, laptop + HP):
  - ajukan kasbon dan pinjaman dengan lampiran;
  - Manager membuka detail lalu menyetujui;
  - HR menyetujui, lalu cek bulan potong;
  - probation dan masa kerja kurang ditolak.

  Data demo dihapus sesudahnya.
- [ ] Automation Log, README registry, PR.
