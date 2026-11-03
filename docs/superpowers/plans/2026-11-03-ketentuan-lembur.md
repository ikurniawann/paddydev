# Ketentuan Lembur + Batas 72 Jam (EPIC-085) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lembur ditolak untuk SPV ke atas. Lembur hari kerja harus mulai setelah jam selesai shift. Batas 4 jam/hari, 18 jam/minggu dan 72 jam/periode diblokir. Lembur saat dinas di tanggal merah dibayar 2× tanpa clock-out.

**Architecture:**
- Aturan berupa fungsi domain murni di `hris/domain/overtime_rules.go`.
- Service menyediakan dua helper, `overtimeStartCheck` dan `checkOvertimeCaps`. Keduanya dipanggil dari create, decide (koreksi jam) dan correct.
- Payroll menerima flag `OnTrip` per pengajuan dari adapter.

**Tech Stack:** Go (hris, payroll, app adapter), Next.js/TS (vitest).

**Spec:** `docs/epics/EPIC-085-ketentuan-lembur.md` (R1–R5). Branch ditumpuk di atas EPIC-084.

## Global Constraints
- Pesan error persis seperti spec. Semua penolakan memakai `httpx.BadRequest`.
- Batas: `domain.OvertimeMaxDailyHours = 4`, `OvertimeMaxWeeklyHours = 18`, `OvertimeMaxPeriodHours = 72`. Minggu = Senin–Minggu dari tanggal lembur.
- Total dihitung dari `hris.overtime_requests.hours` dengan status `pending` dan `approved`, tanpa baris yang sedang dikoreksi.
- Tanggal di pesan berformat `02-01`; angka jam memakai koma desimal (`strings.Replace(FormatFloat(x,'f',-1,64), ".", ",", 1)`).
- Tes integrasi: `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e084?sslmode=disable'` (skema sudah berisi delta EPIC-084), `go test -p 1`. Jam tes memakai `h.setNow` (H+1 EPIC-084).
- Trailer commit: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Jangan commit `.swarm`, `.claude-flow`, atau symlink `node_modules`.

## Review Focus
1. **Koreksi jam oleh atasan** yang menaikkan jam melewati batas mingguan: ditolak, dan baris itu sendiri tidak ikut dihitung dua kali. Diuji di Task 2.
2. **Dua shift sehari** (EPIC-080): jam mulai dibandingkan dengan selesai shift kedua. Diuji di Task 1.
3. **Shift overnight** (selesai < mulai): cek jam mulai dilewati. Diuji di Task 1.
4. **Minggu yang melintasi dua periode** (cutoff 20, misalnya Senin 18 s.d. Minggu 24): batas mingguan menjumlah lintas periode, batas periode hanya periode tanggal itu. Diuji di Task 2.
5. **Karyawan naik level** setelah mengajukan: pengajuan lama tetap bisa diputuskan. Diuji di Task 2.

---

### Task 1: Aturan domain lembur

**Files:**
- Create: `backend/internal/modules/hris/domain/overtime_rules.go`
- Test: `backend/internal/modules/hris/domain/overtime_rules_test.go`

**Interfaces:**
- Produces:
  - konstanta di Global Constraints;
  - `func WeekBounds(date string) (monday, sunday string, ok bool)`;
  - `type OvertimeShiftEnd struct{ End string; Overnight bool }` (End "HH:MM");
  - `func OvertimeStartError(start, dayType string, shift *OvertimeShiftEnd, onTrip bool) string`;
  - `func OvertimeCapError(hours, dayUsed, weekUsed, periodUsed float64, weekStart, weekEnd, periodLabel string) string` — pengecekan berurutan hari → minggu → periode.
  - `const OvertimeLeaderRefused = "Lembur tidak berlaku untuk jabatan Supervisor ke atas"`.

- [ ] **Step 1: Failing tests**:
  - `WeekBounds("2026-11-04")` → `2026-11-02`, `2026-11-08`; Minggu `2026-11-08` → Senin `2026-11-02`.
  - `OvertimeStartError`:
    - `("17:30","regular",{End:"18:00"},false)` → "Lembur hari kerja dimulai setelah jam kerja selesai (18:00)";
    - `"18:00"` → "";
    - `rest_day` → "";
    - shift nil → "";
    - `{End:"07:00",Overnight:true}` → "" (Review Focus 3);
    - onTrip → "";
    - shift kedua `{End:"21:30"}` dengan start "19:00" → pesan (21:30) (Review Focus 2; service mengirim selesai shift terakhir).
  - `OvertimeCapError`:
    - `(4.5,0,0,0,…)` → "Lembur maksimal 4 jam per hari";
    - `(2,2,0,0)` → "";
    - `(3,0,16,0,"2026-11-02","2026-11-08","21 Okt – 20 Nov 2026")` → "Lembur minggu 02-11 s.d. 08-11 sudah 16 jam; maksimal 18 jam per minggu (sisa 2 jam)";
    - `(2,0,0,70.5,…)` → "Lembur periode 21 Okt – 20 Nov 2026 sudah 70,5 jam; maksimal 72 jam per periode (sisa 1,5 jam)";
    - `(2,0,16,70)` → "" (tepat batas).
- [ ] **Step 2: Run** `go test ./internal/modules/hris/domain/ -run 'WeekBounds|OvertimeStartError|OvertimeCapError'`. Expect FAIL (undefined).
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run.** Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): aturan domain lembur TnC — jam mulai & batas 4/18/72 jam (EPIC-085)`.

### Task 2: Sambungkan ke create / decide / correct

**Files:**
- Modify:
  - `backend/internal/modules/hris/overtime.go` (`CreateOvertime`, `DecideOvertime`)
  - `overtime_correction.go` (`CorrectOvertime`)
  - `overtime_daytype.go` (R4 saran `rest_day` saat dinas)
  - repository interface + store (query baru)
- Test: `backend/internal/modules/hris/overtime_tnc_integration_test.go`. Sesuaikan fixture lama yang melanggar R1–R3 tanpa melonggarkan aturan; catat sebagai ruling.

**Interfaces:**
- Consumes: Task 1.
- Produces (store):
  - `OvertimeShiftEndOn(ctx, employeeID, date string) (*domain.OvertimeShiftEnd, error)`: resolusi `v_employee_schedule` seperti `RosterRows` (`DISTINCT ON` `effective_from DESC, priority` untuk `day_of_week` tanggal itu). Ambil `end_time` shift kedua bila ada, selain itu shift pertama. nil tanpa jadwal atau tanpa shift.
  - `OvertimeHoursUsed(ctx, employeeID, from, to, excludeID string) (float64, error)`.
  - `OnApprovedTrip(ctx, employeeID, date string) (bool, error)`.
- Produces (service):
  - `func (s *Service) overtimeStartCheck(ctx, employeeID, date, start, dayType string) error`;
  - `func (s *Service) checkOvertimeCaps(ctx context.Context, r Repository, employeeID, date string, hours float64, excludeID string) error` — dipanggil di dalam transaksi setelah `LockLeaveEmployee(employeeID)`. Periode dari `periodContaining`.

- [ ] **Step 1: Failing test** `TestOvertimeTnC` (`newFileHarness`, `h.setNow` H+1; employee dengan pola NS Senin–Jumat 09:00–18:00 via `SaveShiftPattern` atau insert langsung seperti tes jadwal yang ada). Kasus:
  - (a) Self level Supervisor (`h.leaderPosition`) → 400 `OvertimeLeaderRefused`. HRD menugaskan karyawan itu → 400 yang sama.
  - (b) Senin 17:00–19:00 → 400 jam selesai (18:00); 18:00–20:00 → 200.
  - (c) Sabtu (tanpa shift) 10:00–12:00 → 200 tanpa cek jam mulai.
  - (d) Hari kerja 18:00–22:30 (4,5 jam) → "Lembur maksimal 4 jam per hari".
  - (e) Empat hari kerja 4 jam dalam minggu yang sama (16 jam), lalu hari kelima 18:00–21:00 (3 jam) → pesan mingguan "sudah 16 jam … (sisa 2 jam)".
  - (f) Batas periode: isi 70 jam pending pada minggu-minggu lain di periode yang sama lewat insert langsung (`source='company'`, `status='pending'`), lalu pengajuan 3 jam → pesan periode.
  - (g) Atasan approve dengan koreksi `end_time` yang membuat minggu > 18 → 400 pesan mingguan; koreksi dalam batas → 200 (Review Focus 1).
  - (h) Koreksi HR (`PATCH`) yang melewati 4 jam → 400.
  - (i) Perjalanan dinas `approved` mencakup tanggal merah aktif; `GET /overtime/day-type` → `rest_day`; pengajuan 08:00–11:00 di tanggal itu → 200 tanpa cek jam mulai.
  - (j) Level dinaikkan ke Supervisor setelah pengajuan dibuat → atasan tetap bisa approve (Review Focus 5).
  - (k) Minggu lintas periode dengan cutoff 20 (`attendance_settings` 20): Senin 16-11 s.d. Minggu 22-11; isi 16 jam di 16–19 Nov, pengajuan 21-11 3 jam → pesan mingguan; total periode tanggal 21 hanya menghitung ≥ 21 Nov (Review Focus 4).
- [ ] **Step 2: Run** `go test -p 1 ./internal/modules/hris/ -run TestOvertimeTnC`. Expect FAIL.
- [ ] **Step 3: Implement.**
  - Urutan di `CreateOvertime`:
    1. level (R1) setelah target terbaca;
    2. self: jam mulai (R2) setelah `dayType` diketahui;
    3. cek batas (R3) di dalam transaksi insert. Bila insert belum dalam transaksi, bungkus lock + cek + insert dengan `s.repo.InTx`.
  - `DecideOvertime`: bila jam dikoreksi, jalankan R2 + R3 (exclude baris itu).
  - `CorrectOvertime`: R2 + R3 untuk tanggal/jam baru (exclude baris itu).
  - `suggestOvertimeDayType`: bila `OnApprovedTrip` dan tanggal itu tanggal merah aktif (semua jenis jadwal) → `rest_day`, known.
- [ ] **Step 4: Run** the test, then `go test -p 1 ./internal/modules/hris/`. Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): lembur ditolak untuk SPV+, mulai setelah jam kerja, batas 4/18/72 jam (EPIC-085)`.

### Task 3: Payroll — lembur dinas tanpa clock-out

**Files:**
- Modify:
  - `backend/internal/modules/payroll/domain/inputs.go` (`OvertimeRequest.OnTrip bool`)
  - `period.go` (`SplitOvertimeWith`)
  - `backend/internal/app/adapters_payroll.go:305` (kolom `on_trip` via `EXISTS` ke `hris.business_trips` `approved` yang mencakup tanggal)
- Test: `backend/internal/modules/payroll/domain/period_test.go` (atau file baru `overtime_trip_test.go`).

- [ ] **Step 1: Failing test** `TestSplitOvertimeTripSkipsClockout`:
  - requireClockout true, tanpa absensi;
  - request `{Date:"2026-11-10", Hours:3, DayType:"rest_day", OnTrip:true}` → holiday 3;
  - request yang sama tanpa OnTrip → 0.
- [ ] **Step 2: Run** `go test ./internal/modules/payroll/domain/ -run TestSplitOvertimeTrip`. Expect FAIL.
- [ ] **Step 3: Implement.** Adapter: `SELECT date::text, hours::text, day_type, EXISTS (SELECT 1 FROM hris.business_trips t WHERE t.employee_id = o.employee_id AND t.status = 'approved' AND o.date BETWEEN t.start_date AND t.end_date) AS on_trip FROM hris.overtime_requests o …`.
- [ ] **Step 4: Run** `go test -p 1 ./internal/modules/payroll/... ./internal/app/...`. Expect PASS.
- [ ] **Step 5: Commit** `feat(payroll): lembur saat perjalanan dinas dibayar tanpa syarat clock-out (EPIC-085)`.

### Task 4: Frontend — halaman Lembur ESS

**Files:**
- Modify:
  - `frontend/src/features/hris/ess/components/lembur-page.tsx`
  - `frontend/src/lib/hris/overtime-rules.ts`
- Test:
  - `frontend/src/lib/hris/overtime-rules.test.ts`
  - `frontend/src/features/hris/ess/components/lembur-page.test.tsx`

**Interfaces:**
- Produces: `export const OVERTIME_RULES_HINT = "Lembur hari kerja dimulai setelah jam kerja selesai · maks. 4 jam/hari, 18 jam/minggu, 72 jam/periode"` di `overtime-rules.ts`.

- [ ] **Step 1: Failing tests**:
  - hint konstanta tampil di form;
  - bila `useLeaveQuota(month).data.leader_eligible === true`, halaman menampilkan "Lembur tidak berlaku untuk jabatan Supervisor ke atas" dan tidak merender tombol ajukan. Mock query mengikuti pola tes halaman yang ada.
- [ ] **Step 2: Run** `npx vitest run src/lib/hris/overtime-rules.test.ts src/features/hris/ess/components/lembur-page.test.tsx`. Expect FAIL.
- [ ] **Step 3: Implement.** Pakai `useLeaveQuota` dari `@/features/hris/leaves/queries`, yang sudah dipakai form cuti ESS untuk `leader_eligible`.
- [ ] **Step 4: Run** the vitest files above, then `npx tsc --noEmit` and `npx eslint --quiet` on the changed files. Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): halaman Lembur ESS menampilkan aturan dan menolak SPV+ (EPIC-085)`.

### Task 5: Review akhir, staging, dokumen, PR

- [ ] Final review (reviewer terpisah) → perbaikan.
- [ ] Backup staging lalu deploy branch.
  - Staging punya cutoff 20 sejak EPIC-084, jadi periode = 21–20.
- [ ] QA dengan akun demo (dihapus sesudahnya; audit append-only jangan disentuh):
  - SPV ditolak;
  - jam mulai sebelum selesai shift ditolak;
  - batas 4/18 jam;
  - dinas di tanggal merah lolos dan disarankan Weekend/Libur.
- [ ] Docs:
  - Automation Log epic;
  - `status-requirement.md` (AP-07 baru, AP-02/AP-03 catatan);
  - `pertanyaan-rekomendasi.md` (K-23 dijawab 03-11);
  - `aturan-payroll.md` (batas dan dinas);
  - `docs/epics/README.md`;
  - skill `paddy-hris-acuan`.
- [ ] PR ke `main` (base `main`; PR ini memuat commit EPIC-084 sampai #69 di-merge; sebutkan di body). Merge dan production hanya dengan izin user.
