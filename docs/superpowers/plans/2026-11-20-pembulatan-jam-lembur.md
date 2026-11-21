# Pembulatan Jam Lembur per Pengajuan (EPIC-122) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Payroll membayar lembur per pengajuan dengan jam dibulatkan ke jam penuh: sisa menit > 30 naik, ≤ 30 turun.

**Architecture:** Pembulatan dilakukan per pengajuan di `SplitOvertimeWith` (Go), setelah jam dibatasi absensi aktual dan setelah aturan minimum 1 jam; rumus tarif (`gaji pokok / 173 × 1,5` hari kerja, `× 2` libur/istirahat) tidak berubah. Fallback Next (`splitOvertimeHours`) diberi pembulatan yang sama.

**Tech Stack:** Go (`apps/backend/internal/modules/payroll/domain`), TypeScript/Vitest (`apps/frontend/src/lib/payroll`).

**Spec:** [EPIC-122](../../epics/EPIC-122-pembulatan-jam-lembur.md). Permintaan user 20-11-2026: "perhitungan lembur di payroll belum sesuai dengan rumus ini gaji pokok/173*1,5*jumlah jam lembur (weekday) gaji pokok/173*2*jumlah jam lembur (weekend)" dan "rounding per pengajuan itu mengacu ke rules perhitungan lembur tadi ( roundingnya >31 menit naik ke atas <=30 menit turun kebawah)". Kasus acuan: slip production Gantari Garini (gaji pokok 1.980.000; lembur 18-11 2j55m, 19-11 2j50m hari kerja; 08-11 1j20m Minggu/rest_day).

## Global Constraints

- Pembulatan per pengajuan, bukan per total periode.
- Sisa menit > 30 → naik 1 jam; sisa ≤ 30 → turun (30 menit tepat turun).
- Jam pengajuan disimpan 2 desimal (1j20m = 1,33); konversi ke menit dengan `round(jam × 60)` sebelum dibulatkan.
- Minimum 1 jam (EPIC-059) tetap diperiksa pada jam aktual sebelum pembulatan.
- Tarif dan pembagi tetap dari setting (`overtime_multiplier` 1,5, `overtime_multiplier_holiday` 2, `overtime_hourly_divisor` 173).

## Review Focus

- Jam 2 desimal yang berasal dari menit ganjil (2,92 = 2j55m; 2,83 = 2j50m; 2,52 = 2j31m) harus dibulatkan sesuai menit aslinya, bukan sesuai desimalnya.
- Batas tepat 30 menit (2,50) turun; 31 menit (2,52) naik.
- Jam aktual absensi yang lebih kecil dari pengajuan: pembulatan memakai jam setelah dibatasi.
- Skema `production` dan `daily_worker` ikut memakai jam yang sudah dibulatkan.
- Run payroll November yang sudah dihitung di production perlu dihitung ulang setelah rilis.

---

### Task 1: Pembulatan jam lembur di Go

**Files:**
- Modify: `apps/backend/internal/modules/payroll/domain/period.go` (`SplitOvertimeWith`)
- Test: `apps/backend/internal/modules/payroll/domain/overtime_rounding_test.go` (baru); sesuaikan ekspektasi lama di `payroll_test.go` dan `parity_test.go` yang memakai jam pecahan.

**Interfaces:**
- Produces: `func RoundOvertimeHours(hours float64) float64` di package `domain`.

- [ ] **Step 1: Tulis tes gagal** `TestRoundOvertimeHours` (tabel: 2,92→3; 2,83→3; 1,33→1; 2,50→2; 2,52→3; 1,00→1; 3,00→3; 1,02→1; 1,53→2) dan `TestSplitOvertimeRoundsPerRequest` (pengajuan 2026-11-18 2,92 dan 2026-11-19 2,83 `regular`, 2026-11-08 1,33 `rest_day`, absensi ber-clock-out tanpa `OvertimeHours` → Regular 6, Holiday 1, Total 7), serta `TestCalculateOvertimeGantari` (BaseSalary 1.980.000, OvertimeHours 6, OvertimeHolidayHrs 1, config default → `OvertimeRegularPay` 103.006, `OvertimeHolidayPay` 22.890).
- [ ] **Step 2: Jalankan** `go test ./internal/modules/payroll/domain/ -run 'RoundOvertime|SplitOvertimeRounds|OvertimeGantari'` — Expected: FAIL (`RoundOvertimeHours` belum ada).
- [ ] **Step 3: Implementasi** `RoundOvertimeHours` (menit = round(jam×60); jam penuh = menit div 60; sisa > 30 → +1) dan panggil per pengajuan di `SplitOvertimeWith` setelah cek minimum 1 jam.
- [ ] **Step 4: Jalankan** tes yang sama — Expected: PASS; lalu `go test ./internal/modules/payroll/...` — Expected: ok (ekspektasi lama yang memakai jam pecahan disesuaikan dengan ruling).
- [ ] **Step 5: Commit** `fix(payroll): jam lembur dibulatkan per pengajuan, sisa > 30 menit naik (EPIC-122)`.

### Task 2: Pembulatan yang sama di fallback Next

**Files:**
- Modify: `apps/frontend/src/lib/payroll/period.ts` (`splitOvertimeHours`)
- Test: `apps/frontend/src/lib/payroll/period.test.ts`

**Interfaces:**
- Produces: `export function roundOvertimeHours(hours: number): number`.

- [ ] **Step 1: Tulis tes gagal** `roundOvertimeHours` (tabel yang sama dengan Task 1) dan `splitOvertimeHours` kasus Gantari tanpa hari libur (2,92 + 2,83 → regularHours 6).
- [ ] **Step 2: Jalankan** `pnpm --filter frontend exec vitest run src/lib/payroll/period.test.ts` — Expected: FAIL.
- [ ] **Step 3: Implementasi** `roundOvertimeHours` dan panggil per pengajuan di `splitOvertimeHours`.
- [ ] **Step 4: Jalankan** tes yang sama — Expected: PASS.
- [ ] **Step 5: Commit** `fix(payroll): pembulatan jam lembur di fallback Next (EPIC-122)`.
