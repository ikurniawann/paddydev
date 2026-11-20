# Batas, Keterangan, dan Tampilan Lembur Memakai Jam Dibulatkan (EPIC-123) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Batas lembur, aturan keterangan, dan semua tampilan "jam dibayar" memakai jam yang dibulatkan per pengajuan seperti payroll (EPIC-122), dan karyawan melihat sisa kuota sebelum mengirim pengajuan.

**Architecture:** Satu fungsi pembulatan dipakai bersama. Di Go, fungsi pindah ke paket platform `overtimehours` supaya modul `hris` dan `payroll` tidak saling impor. Di frontend, fungsi pindah ke `lib/hris/overtime-rounding.ts`. Batas 18/32 jam dan keterangan > 4 jam memakai jam dibulatkan. Endpoint baru `GET /api/hris/overtime/quota` memberi sisa kuota untuk form ESS.

**Tech Stack:** Go (`apps/backend/internal/...`), Next.js/TypeScript + Vitest (`apps/frontend/src/...`). Repo: `wit-id-indonesia/erp-paddy`, branch dasar `development` (d6d90ec).

**Spec:** [EPIC-123](../../epics/EPIC-123-lembur-batas-dan-tampilan-dibulatkan.md); dasar [EPIC-122](../../epics/EPIC-122-pembulatan-jam-lembur.md).

## Global Constraints

- Pembulatan: menit = round(jam × 60); jam penuh = menit div 60; sisa ≥ 31 menit naik 1 jam, ≤ 30 turun. Satu definisi untuk Go dan satu untuk TypeScript, dengan tabel tes yang sama.
- Minimal 1 jam tetap diperiksa pada jam mentah sebelum pembulatan (60 menit sah, 59 ditolak).
- Batas tetap 18 jam/minggu dan 32 jam/periode. Yang berubah hanya cara menjumlah: tiap pengajuan pending/approved dibulatkan dulu, baru dijumlah.
- Tarif dan pembagi tidak berubah (1,5 / 2 / 173).
- Fallback Next `overtime-repo.ts` tidak menegakkan batas (`/api/hris` dirutekan ke Go); tidak diubah.
- Tanpa migrasi.

## Review Focus

- Pengajuan lama berjam pecahan (1,33; 2,92) ikut dijumlah setelah dibulatkan saat mengecek batas.
- Koreksi HR dan approve dengan koreksi jam memakai jam dibulatkan untuk batas; koreksi yang menurunkan jam tetap boleh walau total di atas batas (perilaku EPIC-109).
- Batas mingguan 18 jam ikut memakai jam dibulatkan (konsisten dengan keputusan 32 jam; ruling, konfirmasi saat review rencana).
- Endpoint kuota hanya mengembalikan data milik karyawan yang login; HR dengan `employee_id` lain mengikuti aturan akses listOvertime.
- Form ESS: tampilan sisa kuota hanya petunjuk; server tetap penentu.

---

### Task 1: Paket pembulatan bersama (Go)

**Files:**
- Create: `apps/backend/internal/platform/overtimehours/round.go`, `round_test.go`
- Modify: `apps/backend/internal/modules/payroll/domain/period.go` (`RoundOvertimeHours` memanggil `overtimehours.Round`)

**Interfaces:**
- Produces: `func Round(hours float64) float64` di paket `overtimehours`.

- [ ] **Step 1:** Tes `TestRound` dengan tabel EPIC-122 (2,92→3; 2,83→3; 1,33→1; 2,50→2; 2,52→3; 1,00→1; 3,00→3; 1,02→1; 1,53→2; 4,33→4; 4,52→5; 15,40→15; 2,40→2).
- [ ] **Step 2:** `go test ./internal/platform/overtimehours/` → FAIL (paket belum ada).
- [ ] **Step 3:** Pindahkan isi `RoundOvertimeHours` ke `overtimehours.Round`; `domain.RoundOvertimeHours` tinggal memanggilnya.
- [ ] **Step 4:** `go test ./internal/platform/overtimehours/ ./internal/modules/payroll/...` → ok.
- [ ] **Step 5:** Commit `refactor(payroll): pembulatan jam lembur di paket bersama (EPIC-123)`.

### Task 2: Batas 18/32 jam dan keterangan > 4 jam memakai jam dibulatkan

**Files:**
- Modify: `apps/backend/internal/modules/hris/domain/overtime_rules.go` (`OvertimeReasonError`, `OvertimeCapError` menerima jam dibulatkan)
- Modify: `apps/backend/internal/modules/hris/overtime_tnc.go` (`OvertimeHoursUsed` mengambil jam per baris lalu menjumlah `overtimehours.Round`; `checkOvertimeCaps` membulatkan `hours` baru)
- Modify: `apps/backend/internal/modules/hris/overtime.go` (pemanggil `OvertimeReasonError` mengirim jam dibulatkan)
- Test: `hris/domain/overtime_rules_test.go`, integrasi `hris/overtime_tnc_integration_test.go`

**Interfaces:**
- Consumes: `overtimehours.Round` (Task 1).
- Produces: `OvertimeHoursUsed` mengembalikan jumlah jam dibulatkan.

- [ ] **Step 1:** Tes unit: `OvertimeReasonError(4.33, "")` = "" ; `OvertimeReasonError(4.52, "")` = pesan wajib keterangan.
- [ ] **Step 2:** Tes integrasi `TestOvertimeCapRounded`: dua pengajuan approved 15,40 jam (15j24m → 15 + 15) di periode yang sama, pengajuan ketiga 2,40 jam (2j24m → 2) diterima (total 32; dengan jam mentah 33,2 akan ditolak). Pengajuan keempat 1 jam → 400 "… sudah 32 jam; maksimal 32 jam per periode (sisa 0 jam)".
- [ ] **Step 3:** Jalankan tes → FAIL.
- [ ] **Step 4:** Implementasi. Pesan tolak memakai angka bulat.
- [ ] **Step 5:** `go test ./internal/modules/hris/...` (dengan `TEST_DATABASE_URL`) → ok; sesuaikan ekspektasi lama yang memakai jam pecahan, catat sebagai ruling di ledger.
- [ ] **Step 6:** Commit `fix(hris): batas dan keterangan lembur memakai jam dibulatkan (EPIC-123)`.

### Task 3: Endpoint sisa kuota lembur

**Files:**
- Modify: `apps/backend/internal/modules/hris/routes.go` (`GET /api/hris/overtime/quota`)
- Create: handler di `apps/backend/internal/modules/hris/overtime_quota.go`
- Test: `hris/overtime_quota_integration_test.go`

**Interfaces:**
- Produces: `GET /api/hris/overtime/quota?date=YYYY-MM-DD[&employee_id=]` → `{ data: { week: {start, end, used, max, remaining}, period: {label, start, end, used, max, remaining} } }`, jam dibulatkan.

- [ ] **Step 1:** Tes integrasi: karyawan dengan pengajuan 1,33 + 2,92 jam di minggu dan periode yang sama → week.used 4, period.used 4, period.remaining 28; karyawan tanpa hak HR yang meminta `employee_id` orang lain ditolak seperti listOvertime.
- [ ] **Step 2:** Jalankan → FAIL (route belum ada).
- [ ] **Step 3:** Implementasi memakai `WeekBounds`, `periodContaining`, `OvertimeHoursUsed`.
- [ ] **Step 4:** Tes → ok. Daftarkan route di peta rute frontend bila ada tes `backend-routes`.
- [ ] **Step 5:** Commit `feat(hris): endpoint sisa kuota lembur (EPIC-123)`.

### Task 4: Tampilan jam dibayar dan sisa kuota di frontend

**Files:**
- Create: `apps/frontend/src/lib/hris/overtime-rounding.ts` (+ test); `lib/payroll/period.ts` mengimpor dari sini
- Modify: `apps/frontend/src/lib/hris/overtime-rules.ts` (`paidOvertimeHours` tetap mentah; tambah `roundedPaidOvertimeHours`)
- Modify: `features/hris/ess/components/lembur-page.tsx`, `features/hris/team-approvals/components/overtime-approve-dialog.tsx`, `persetujuan-tim-page.tsx`
- Modify: `features/hris/overtime/api.ts` (`fetchOvertimeQuota`)
- Test: vitest untuk helper dan komponen ESS

- [ ] **Step 1:** Tes: `roundOvertimeHours` dengan tabel yang sama dengan Task 1; form ESS 13.00–14.20 menampilkan "1 jam dibayar"; 13.00–15.31 "3 jam dibayar"; keterangan wajib muncul pada 4j31m, tidak pada 4j20m; kartu kuota menampilkan sisa minggu dan periode dari endpoint Task 3.
- [ ] **Step 2:** Jalankan → FAIL.
- [ ] **Step 3:** Implementasi. Validasi "minimal 1 jam" tetap pada jam mentah. Bila jam dibulatkan melebihi sisa kuota, tampilkan peringatan; pengiriman tetap diputus server.
- [ ] **Step 4:** `vitest run` penuh, `tsc --noEmit`, `eslint` file yang diubah → bersih.
- [ ] **Step 5:** Commit `feat(hris): jam dibayar dibulatkan dan sisa kuota di form lembur (EPIC-123)`.

### Task 5: Gate, PR, staging

- [ ] `go vet ./...`, `go test ./...` (integrasi dengan DB tes), vitest penuh, tsc, eslint.
- [ ] Review akhir seluruh branch oleh reviewer terpisah.
- [ ] PR ke `development`. CI GitHub Actions tertahan billing: lampirkan hasil gate lokal di PR.
- [ ] Deploy staging, QA form ESS dan Persetujuan Tim di staging, catat di Automation Log.

## Rilis (keputusan user: bersama EPIC-118 s.d. 120 dan 122)

1. Gladi di salinan production (container `--internal` + proxy + crawler Playwright, pola rilis ke-2), termasuk hitung ulang run payroll November di salinan dan bandingkan slip (contoh Gantari Garini: lembur hari kerja 6 jam Rp103.006, libur 1 jam Rp22.890).
2. Minta izin jam rilis. Backup DB production.
3. PR `development` → `production`, deploy manual (pola rilis ke-3), pantau sampai "Deploy selesai".
4. HR menghitung ulang run payroll November; cek slip contoh.
5. Catat di Automation Log EPIC-118 s.d. 123.
