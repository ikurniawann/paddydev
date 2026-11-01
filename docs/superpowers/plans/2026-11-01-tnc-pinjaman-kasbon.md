# Syarat & Ketentuan Pinjaman dan Kasbon — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pinjaman dan kasbon mengikuti TnC 01-11: hanya PKWT/PKWTT, pokok ≤ 50% THP, cicilan 1–2 kali, dipotong pada payroll periode pengajuan (21–20).

**Architecture:** Satu validator domain dipakai kedua jenis, menggantikan cabang kasbon/loan saat ajukan dan setujui. Bulan potong ditentukan fungsi domain murni dari tanggal pengajuan WIB; saat disetujui digeser ke bulan berikutnya bila run payroll bulan itu sudah selesai. Form ESS dan HR memakai tenor 1/2 serta menampilkan TnC dan perkiraan bulan potong.

**Tech Stack:** Go (`backend/internal/modules/payroll`, tes unit domain + integrasi Postgres), Next.js (Vitest).

**Spec:** `docs/epics/EPIC-076-tnc-pinjaman-kasbon.md` (R1–R9).

## Global Constraints
- Status yang diizinkan: `contract`, `pkwt`, `permanent` (case-insensitive, trim). Selain itu ditolak.
- Pesan, verbatim:
  - probation: `Karyawan masa percobaan (probation) tidak dapat mengajukan pinjaman atau kasbon`
  - status lain: `Pinjaman dan kasbon hanya untuk karyawan PKWT atau PKWTT`
  - tenor: `Cicilan pinjaman dan kasbon hanya 1 atau 2 kali`
  - plafon: `Pinjaman/kasbon maksimal 50% THP` (+ ` (maks Rp…)` bila `showCap`)
- Batas tanggal periode: **20** (konstanta `LoanPeriodCutoffDay = 20`), WIB.
- Run yang dianggap selesai: status `completed` atau `paid`.
- Pinjaman lama (tenor > 2, berjalan) tidak diubah; tanpa migrasi.
- Tes integrasi Go: `go test -p 1`.

## Review Focus
1. Pengajuan pukul 23.30 WIB tanggal 20 (= 16.30 UTC): masih periode bulan itu. Konversi WIB, bukan UTC. Tes `TestLoanPayrollPeriod` (Task 2).
2. Pengajuan 21 Desember → payroll **Januari tahun berikutnya** (pergantian tahun). Tes `TestLoanPayrollPeriod` (Task 2).
3. Status bertulisan berbeda (`" Permanent "`, `PKWT`) tetap diizinkan; `""` ditolak. Tes `TestLoanEligibleStatus` (Task 1).
4. Pengajuan pending lama tenor 6 disetujui setelah rilis: ditolak dengan pesan tenor (validasi ulang). Tes di Task 2.
5. Run bulan target ada tetapi masih `draft`/`processing`: cicilan tetap bulan itu, tidak digeser. Tes di Task 2.

---

### Task 1: Validator domain bersama
**Files:**
- `backend/internal/modules/payroll/domain/kasbon.go`: ganti `kasbonBlockedStatuses`/`KasbonBlockedStatus` dengan daftar izin; `ValidateKasbon` → `ValidateLoanTnC`.
- Test: `domain/kasbon_test.go`.

**Produces:**
- `func LoanEligibleStatus(status string) bool`
- `func ValidateLoanTnC(principal float64, tenor int, status string, thp *float64, activeCount, maxActive float64, showCap bool) string`. Urutan cek: status → tenor → jumlah aktif → THP ada → plafon.
- `KasbonMaxThpPercent` diganti nama `LoanMaxThpPercent = 50`.

- [ ] `TestLoanEligibleStatus`:
  - `contract`, `pkwt`, `permanent`, `" Permanent "`, `PKWT` → true;
  - `probation`, `intern`, `freelance`, `daily_worker`, `""` → false.
- [ ] `TestValidateLoanTnC`, satu baris per kasus, dengan pesan Global Constraints:
  - `probation` → pesan probation; `daily_worker` → pesan status lain;
  - tenor 3 → pesan tenor;
  - pokok 2.600.000 dengan THP 5.000.000 → pesan plafon (dengan dan tanpa `showCap`);
  - pokok 2.500.000 → `""`;
  - THP nil → pesan struktur gaji.
- [ ] RED → implementasi → GREEN → `go test ./internal/modules/payroll/domain/` → commit.

### Task 2: Ajukan, setujui, dan periode potong
**Files:**
- `backend/internal/modules/payroll/loans.go` (create ±L140–170, approve ±L270–300): kedua jenis memakai `ValidateLoanTnC` dengan `kasbonTHP`.
- `domain/loans.go`: `LoanPayrollPeriod`.
- `postgres_runs.go` atau `loans.go`: `runClosed(ctx, q, month, year) (bool, error)`.
- Test: `kasbon_integration_test.go` (sesuaikan kasus kasbon lama yang memakai blokir), file baru `loan_tnc_integration_test.go`, `domain/payroll_test.go`.

**Produces:**
- `func LoanPayrollPeriod(submittedAt time.Time) (month, year int)`: tanggal di WIB, hari ≤ 20 → bulan itu, selain itu bulan berikutnya.
- Approve memakai `LoanPayrollPeriod(loan.created_at)`. Bila `runClosed` (ada `payroll_runs` dengan `period_month/period_year` itu dan status `IN ('completed','paid')`) → bulan berikutnya.
- Respons approve menambah `first_installment_label` ("November 2026").
- `FirstInstallmentPeriod` dihapus bila tak terpakai lagi.

- [ ] `TestLoanPayrollPeriod` (WIB):
  - 2026-11-20 23:30 → 11/2026; 2026-11-21 00:05 → 12/2026;
  - 2026-12-21 → 1/2027; 2026-01-01 → 1/2026;
  - waktu masukan berzona UTC `2026-11-20T16:30Z` → 11/2026.
- [ ] `TestLoanTnCCreate` (integrasi):
  - pinjaman `loan` oleh karyawan probation → 400 pesan probation;
  - PKWT tenor 3 → 400 pesan tenor;
  - pokok > 50% THP → 400 pesan plafon;
  - PKWTT valid → 200;
  - kasbon `daily_worker` → 400 pesan status lain.
- [ ] `TestLoanFirstInstallmentFollowsSubmission` (integrasi, jam tetap):
  - pinjaman dibuat dengan `created_at` 2026-11-20 lalu disetujui HR → `first_installment` 11/2026;
  - `created_at` 2026-11-21 → 12/2026;
  - run November status `completed` sudah ada → pengajuan 20 Nov disetujui menjadi 12/2026;
  - run November `draft` → tetap 11/2026.
- [ ] `TestLoanApproveRechecksTnC`: pending lama tenor 6 (insert langsung) → approve HR 400 pesan tenor.
- [ ] Sesuaikan tes kasbon lama yang mengandalkan blokir (`TestKasbonBlockedForProbation` tetap; pesan berubah).
- [ ] RED → GREEN → `go test -p 1 ./internal/modules/payroll/...` → commit.

### Task 3: Form ESS & HR
**Files:**
- `frontend/src/lib/hris/loans-view.ts`: `kasbonTenorOptions` → `loanTenorOptions`, berlaku untuk kedua jenis; validasi form tenor 1/2; tambah `loanPayrollPeriodLabel(dateIso: string): string`.
- `frontend/src/features/hris/ess/components/pinjaman-page.tsx` (form pengajuan).
- `frontend/src/features/hris/loans/components/loan-form-dialog.tsx` (HR).
- Test: `lib/hris/loans-view.test.ts` dan `features/hris/ess/components/pinjaman-page.test.tsx`.

**Produces:** `loanPayrollPeriodLabel("2026-11-21")` → `"Desember 2026"`. Logikanya sama dengan Go `LoanPayrollPeriod`, dengan masukan tanggal WIB `todayWib()`.

- [ ] Tes `loans-view`:
  - `loanPayrollPeriodLabel` untuk 20/21 Nov dan 21 Des (→ "Januari 2027");
  - validasi form jenis `loan` tenor 6 → error "Cicilan pinjaman dan kasbon hanya 1 atau 2 kali".
- [ ] Tes ESS:
  - form menampilkan teks TnC: "Hanya PKWT/PKWTT · maks 50% THP · 1–2 kali cicilan dipotong dari payroll";
  - perkiraan "Dipotong mulai payroll <bulan>" (dengan `todayWib` di-mock);
  - pilihan tenor hanya 1 dan 2 untuk jenis Pinjaman.
- [ ] Form HR: tenor 1/2 dan teks TnC yang sama.
- [ ] Label jenis `loan` menjadi **"Pinjaman Karyawan"** (sebutan user): `LOAN_TYPE_OPTIONS` di `lib/hris/loans-view.ts` dan `loanTypeLabels` di `backend/internal/modules/payroll/domain/payslip.go` (slip). Sesuaikan tes yang memakai label "Pinjaman".
- [ ] RED → GREEN → `pnpm vitest run src/lib/hris src/features/hris/ess src/features/hris/loans` → commit.

### Task 4: Acuan, dokumen, gerbang, QA
- [ ] Perbarui acuan MOM:
  - `docs/hris/acuan-mom-20261019/status-requirement.md`: KP-02 (daftar izin R2), KP-03 (Disepakati, TnC 01-11, ✅), PY-02 (periode potong pinjaman 21–20).
  - `pertanyaan-rekomendasi.md`: TnC dicatat.
  - Skill `.claude/skills/paddy-hris-acuan/SKILL.md`, tabel aturan kunci baris Kasbon, menjadi "Pinjaman & Kasbon: PKWT/PKWTT, maks ½ THP, tenor 1–2, potong payroll periode pengajuan (21–20)".
- [ ] Gerbang: `go vet`; `go test -p 1` untuk payroll; frontend `tsc`, `lint`, `pnpm test`.
- [ ] Review akhir (fresh reviewer).
- [ ] Staging: QA dengan data demo (karyawan probation, PKWT, PKWTT dengan struktur gaji demo):
  - ajukan pinjaman dan kasbon;
  - setujui, lalu cek bulan potong;
  - laptop + HP.
  - Hapus data demo.
- [ ] Automation Log EPIC-076, README registry, PR.
