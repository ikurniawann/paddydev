# Jam Kerja Sistem Shift, Libur Nasional & Cuti Bersama Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hari libur, absen terlewat, jenis hari lembur, hitung cuti, peringatan roster, dan payroll membedakan staf Non-Shift (HQ/Operations) dari staf sistem shift, sesuai TnC 02-11.

**Architecture:**
- Satu aturan domain Go, `domain.HolidayDayOff(h Holiday, scheduleType string) bool`, menjadi satu-satunya tempat yang memutuskan "hari ini libur untuk karyawan ini".
- Semua pembaca hari libur (roster, gaps, jenis hari lembur, cuti, payroll) memanggilnya dengan `schedule_type` karyawan.
- Roster per tanggal menjadi sumber hari kerja staf shift, baik untuk cuti maupun payroll, lewat `hris.v_employee_schedule`.
- Satu migrasi menambah `public_holidays.is_day_off` dan mengubah Host Live/Produksi/CRM menjadi Shift.

**Tech Stack:** Go (hris, payroll, app adapters), PostgreSQL migration, Next.js + Vitest (HR Hari Libur, ESS).

**Spec:** `docs/epics/EPIC-079-jam-kerja-sistem-shift.md` (R1–R10). Lihat juga EPIC-077 dan EPIC-078.

## Global Constraints
- Sistem shift = `hris.employees.schedule_type = 'shift'`; Non-Shift = `'non_shift'` (R1).
- Libur Non-Shift: `nasional`, `perusahaan`, dan `cuti_bersama` bila `is_day_off`. Staf shift: tidak ada tanggal merah yang menjadi libur (R2–R3).
- Kolom baru `hris.public_holidays.is_day_off boolean NOT NULL DEFAULT false`. Nilainya hanya bermakna untuk `cuti_bersama`; untuk `nasional` dan `perusahaan` diabaikan (selalu libur).
- Peringatan, bukan blokir: peringatan baru memakai `UploadIssue` dan `ack_warnings` yang sudah ada.
- Teks TnC ditulis apa adanya. Bahasa UI Indonesia.
- Riwayat absensi dan payroll yang sudah closing tidak dihitung ulang.
- Uji integrasi Go: `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e071?sslmode=disable' go test -p 1`. Setelah migrasi baru, jalankan `go run ./cmd/migrate -apply` ke DB uji.

## Review Focus
1. **Karyawan tanpa `schedule_type` yang jelas** (data lama, akun baru): `schedule_type` NOT NULL dengan default `shift`. Karyawan HQ tanpa jadwal tidak boleh tiba-tiba kehilangan libur nasional. Diuji di Task 2: karyawan Non-Shift tanpa jadwal tetap libur di 25-12.
2. **Tanggal dengan dua baris libur** (nasional + cuti bersama di hari yang sama): nasional menang, sehingga libur bagi Non-Shift walau cuti bersama tidak ditetapkan. Diuji di Task 1.
3. **Jadwal per tanggal dan pola mingguan berlaku di hari yang sama**: per tanggal menang di payroll, sama seperti view. Diuji di Task 5.
4. **Shift malam** (MALAM 22.00–06.00, CS3 16.00–00.00) dalam hitungan 40 jam: durasi melewati tengah malam. Diuji di Task 4.
5. **Cuti staf shift di rentang tanpa roster sama sekali**: dihitung per tanggal (R4), tidak menolak "0 hari". Diuji di Task 3.

---

### Task 1: Hari libur "ditetapkan libur" + aturan libur per tipe jadwal

**Files:**
- Create: `backend/database/migrations/deltas/20261103090000_hris_shift_system_holidays.sql` (bagian 1: kolom `is_day_off`)
- Modify: `backend/internal/modules/hris/domain/holidays.go` (`Holiday`, aturan baru), `backend/internal/modules/hris/postgres_leaves.go:75-90` (`ActiveHolidays` membaca `is_day_off`), `backend/internal/modules/hris/holidays.go` (create/update/import menerima `is_day_off`)
- Modify: `frontend/src/features/hris/holidays/components/holiday-form-dialog.tsx`, `holiday-badges.tsx`, `frontend/src/features/hris/holidays/types.ts`
- Test: `backend/internal/modules/hris/domain/holidays_test.go`, `backend/internal/modules/hris/holidays_shift_integration_test.go`, `frontend/src/features/hris/holidays/components/holiday-form-dialog.test.tsx`

**Interfaces:**
- Produces:
  - `domain.Holiday{…, IsDayOff bool}`.
  - `func HolidayDayOff(h Holiday, scheduleType string) bool`.
  - `func DayOffDates(rows []Holiday, scheduleType string) map[string]bool`.
  - API holiday row field `is_day_off`.

- [ ] **Step 1: Failing domain test** `TestHolidayDayOff`:
  - Non-Shift: `nasional` → true; `perusahaan` → true; `cuti_bersama` with `IsDayOff=false` → false; with `IsDayOff=true` → true.
  - Shift (`"shift"`): all four → false.
  - `DayOffDates` for `[{2026-12-24 cuti_bersama false}, {2026-12-25 nasional}]`, `"non_shift"` → `{"2026-12-25": true}`. For a date holding both nasional and cuti bersama → true.
- [ ] **Step 2: Run, verify FAIL** (`go test ./internal/modules/hris/domain -run TestHolidayDayOff`).
- [ ] **Step 3: Migration part 1 + domain + store**
  - Migration: `ALTER TABLE hris.public_holidays ADD COLUMN IF NOT EXISTS is_day_off boolean NOT NULL DEFAULT false;` plus a column comment.
  - `ActiveHolidays` selects `is_day_off`.
  - `holidays.go` create/update accept an optional boolean `is_day_off`, accepted only for `type = 'cuti_bersama'`; other types return 400 "Hanya cuti bersama yang bisa ditetapkan libur". Audit the change like other holiday edits.
- [ ] **Step 4: Integration test** `TestCutiBersamaDayOffToggle`: PATCH `is_day_off` on a cuti bersama → 200 and stored; on `nasional` → 400 with the message above.
- [ ] **Step 5: UI.**
  - Form shows a switch "Ditetapkan libur oleh perusahaan" only when the type is Cuti Bersama.
  - Badge text: cuti bersama shows "Libur HQ/Operations" when `is_day_off`, else "Hari kerja"; nasional/perusahaan show "Libur HQ/Operations".
  - Vitest: switch hidden for Nasional; visible and sends `is_day_off: true` for Cuti Bersama.
- [ ] **Step 6: Run** the Go tests and vitest above. Expect PASS.
- [ ] **Step 7: Commit** `feat(hris): cuti bersama bisa ditetapkan libur; aturan libur per tipe jadwal (EPIC-079)`.

### Task 2: Kalender absen, absen terlewat, dan jenis hari lembur mengikuti tipe jadwal

**Files:**
- Modify: `backend/internal/modules/hris/attendance.go:178-181` + `postgres_attendance.go:133-137`. The roster gets holidays per employee: pass the `[]Holiday` and use `HolidayDayOff` per row with the row's `schedule_type`; add `e.schedule_type` to `RosterRows`.
- Modify: `backend/internal/modules/hris/attendance_gaps.go:73`, `attendance_gap_summary.go:99`. Build `GapInput.Holidays` with `DayOffDates(holidays, scheduleType)`.
- Modify: `backend/internal/modules/hris/overtime_daytype.go:17-34`. `holiday` = `DayOffDates(...)[date]` for the employee.
- Modify: `backend/internal/modules/hris/domain/schedule_upload.go:375-380`. Remove `holidayIssue` and its callers; the grid header label stays.
- Modify (Next mirror, if the route is still reachable): `frontend/src/lib/hris/daily-roster.ts:44-65`. Same rule by `schedule_type`; check `go-routes.generated.json` first, and leave a file untouched if its route is Go-only.
- Test: `backend/internal/modules/hris/holidays_shift_integration_test.go`

**Interfaces:**
- Consumes: `HolidayDayOff`, `DayOffDates` (Task 1).

- [ ] **Step 1: Failing test** `TestHolidayFollowsScheduleType`:
  - Data: national holiday 2026-12-25; store employee (`shift`) with a per-date SPG1 on 25-12 and no attendance; HQ employee (`non_shift`, NS pattern).
  - Roster `GET /api/hris/attendance/daily-roster?date=2026-12-25`: store row status is not `libur_nasional` (it is `belum_absen`/`tidak_hadir` per the existing rule), HQ row is `libur_nasional`.
  - Gaps for the store employee include 25-12; for HQ they do not.
  - Overtime day type: store with a shift on 25-12 → `weekday`; store on its roster off day → `rest_day`; HQ on 25-12 → `rest_day`.
  - Cuti bersama 24-12 with `is_day_off=false`: HQ gap on 24-12 present. With `is_day_off=true`: absent.
  - A Non-Shift employee with no schedule at all is still on holiday on 25-12 (Review Focus 1).
- [ ] **Step 2: Run, verify FAIL.**
- [ ] **Step 3: Implement** the four call sites; delete `holidayIssue` and update its existing tests (expect no "libur nasional" warning).
- [ ] **Step 4: Run** the hris package tests. Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): tanggal merah tidak meliburkan staf shift; cuti bersama ikut penetapan (EPIC-079)`.

### Task 3: Hitung hari cuti staf shift dari roster

**Files:**
- Modify: `backend/internal/modules/hris/domain/holidays.go`. Add `func DescribeRosterLeaveDays(startISO, endISO string, schedule []ScheduleRow) int`.
- Modify: `backend/internal/modules/hris/leaves.go:175-179`.
  - Load `schedule_type`.
  - Shift: load the employee's `v_employee_schedule` rows via the existing `ShiftPattern` repo method (`files.go:65`) and count with `DescribeRosterLeaveDays`.
  - Non-Shift: `DescribeLeaveDays` with holidays filtered through `HolidayDayOff` (a non-day-off cuti bersama counts as a normal day).
- Create: `GET /api/hris/me/leave-days?start=&end=` → `{days, excluded}` (`excluded` = names of holidays skipped, empty for shift staff). Register it in `routes.go` next to the other `/me` routes, run `make generate`, and add it to `goNativePrefixes` in `frontend/src/lib/backend-routes.test.ts`.
- Modify: `frontend/src/features/hris/ess/components/cuti-request-dialog.tsx:17,60`. The preview uses the endpoint (TanStack query keyed by start/end) instead of TS `describeLeaveDays`.
- Test: `backend/internal/modules/hris/domain/holidays_test.go`, `holidays_shift_integration_test.go`, `cuti-request-dialog.test.tsx`

- [ ] **Step 1: Failing domain test** `TestDescribeRosterLeaveDays`. Dates are 2026-12-21..25.
  - Rows: SPG1 on 21, 22, 24, 25, per-date off (`ShiftID nil`) on 23 → 4.
  - Same range with no rows → 5 (R4: unrostered days count).
  - Weekend dates with a shift count.
- [ ] **Step 2: Failing integration test** `TestShiftLeaveCountsRoster`:
  - POST leave 21–25 Dec for the store employee → `total_days = 4`.
  - HQ employee 21–25 Dec → 4 (25 is a national holiday).
  - HQ 24–24 with cuti bersama `is_day_off=false` → 1, and with `true` → 1 (deducted, as today).
  - `GET /api/hris/me/leave-days` returns the same numbers.
- [ ] **Step 3: Run, verify FAIL; implement; run, verify PASS.**
- [ ] **Step 4: Vitest:** the dialog shows "4 hari kerja" from the mocked endpoint, and shows a loading state while it loads.
- [ ] **Step 5: Commit** `feat(hris): cuti staf shift dihitung dari roster (EPIC-079)`.

### Task 4: Peringatan 40 jam di roster + pola mingguan

**Files:**
- Modify: `backend/internal/modules/hris/domain/schedule_upload.go`.
  - `UploadShift` gains `BreakMinutes int`, filled where `Shifts` is built from `loadShifts` (`schedule_days.go:189-215`).
  - Add `const minHoursBeforeDayOff = 40`.
  - Add `func shiftNetMinutes(s UploadShift) int` (end−start, plus 24 h when `Overnight || end <= start`, minus `BreakMinutes`).
  - In `sequenceWarnings`, when a streak ends with a day off whose previous day off is inside `days`, and the streak's net minutes are < 40×60 and some day in the streak or the day off is edited, warn: `"%s libur %s sebelum 40 jam kerja (baru %d jam sejak libur %s)"` (name, `FormatDateID(off)`, hours rounded down, `FormatDateID(prevOff)`), column = `ScheduleDateHeader(off)`.
- Modify: `frontend/src/features/hris/schedules/components/pattern-dialog.tsx`. For shift employees, show an inline amber note when all 7 days have a shift ("Belum ada hari libur — sistem shift wajib libur 1 hari setiap minggu") and the weekly net-hours total ("Total ±42 jam kerja/minggu"). Compute net hours with the shift list already in the grid response (`break_minutes`).
- Test: `backend/internal/modules/hris/domain/schedule_upload_test.go`, `pattern-dialog.test.tsx`

- [ ] **Step 1: Failing tests** `TestFortyHourDayOffWarning`:
  - off, 4×SPG1, off (edited) → exactly one warning containing "sebelum 40 jam kerja (baru 28 jam".
  - off, 6×SPG1, off → none.
  - off, 6×MALAM (22–06, break 60) then off → none (Review Focus 4: 7 h each).
  - Streak without a previous off in the window → none.
  - The existing "masuk 7 hari berturut-turut" test still passes.
- [ ] **Step 2: Run, verify FAIL; implement; run, verify PASS.**
- [ ] **Step 3: Vitest** for the pattern dialog: 7 filled days show the note; 6 + libur do not; the total shows "42 jam".
- [ ] **Step 4: Commit** `feat(hris): peringatan roster libur sebelum 40 jam kerja (EPIC-079)`.

### Task 5: Payroll membaca roster dan libur per tipe jadwal

**Files:**
- Modify: `backend/internal/modules/payroll/domain/period.go:59-117`.
  - `ShiftRow` gains `Priority int`.
  - `ResolveScheduleRow` picks the latest `EffectiveFrom`, then the lowest `Priority` on ties.
- Modify: `backend/internal/app/adapters_payroll.go:285` and `adapters_payroll_kpi.go:68`.
  - Read `SELECT day_of_week, shift_id, effective_from, effective_to, priority FROM hris.v_employee_schedule …` (per-date rows have `effective_from = effective_to = work_date`, `priority = 0`).
  - Add `ScheduleType string` to payroll `domain.Employee` (`payroll/domain/inputs.go:9`), filled by `PayrollEmployees`.
- Modify: `backend/internal/modules/payroll/ports.go:113-114` + `adapters_payroll.go:359-371`. `Holidays` returns `map[string]bool` of **Non-Shift day-off dates** (R2 via `is_day_off`; nasional/perusahaan always true).
- Modify: `backend/internal/modules/payroll/service_runs.go:414`. Per employee, pass `nil` holidays when `schedule_type = 'shift'`, else the map.
- Create: `backend/database/sql/sim-epic079/payroll_divisor.sql`, a read-only comparison per employee of scheduled days (weekly only vs view) for a period, used for the simulation report.
- Test: `backend/internal/modules/payroll/domain/period_test.go`, `backend/internal/app/adapters_payroll_integration_test.go` (or the nearest existing adapter test file)

**Interfaces:**
- Consumes: `is_day_off` column (Task 1).

- [ ] **Step 1: Failing domain tests.**
  - `TestResolveScheduleRowPrefersPerDate`: a weekly row (dow 4, NS, from 2026-11-05) and a per-date row (dow 4, nil, from = to = 2026-11-05, priority 0) → the per-date row wins on 2026-11-05.
  - `TestScheduledDaysCountsRoster`: per-date rows only, 26 shifts in Nov → 26.
- [ ] **Step 2: Failing adapter test** `TestPayrollScheduleReadsRoster`:
  - Store employee with only `employee_shift_days` rows → schedule rows non-empty, and `ScheduledDays` over the period equals the rostered days.
  - `schedule_type` is returned.
  - Holidays map excludes a cuti bersama with `is_day_off=false`.
- [ ] **Step 3: Implement; run** `go test ./internal/modules/payroll/... ./internal/app/...`. Expect PASS.
- [ ] **Step 4: Overtime test** `TestShiftOvertimeOnNationalHolidayIsWeekday` (payroll domain `SplitOvertimeWith` with `holidays=nil` and a shift scheduled on 2026-12-25) → hours in the weekday bucket. Same input with HQ holidays → holiday bucket.
- [ ] **Step 5: Simulation.**
  - Run `payroll_divisor.sql` on the staging copy for November 2026.
  - Write `.share/payroll/sim-epic079-<date>.md`: employees whose divisor changes, before/after, count by department.
  - Paste the summary into the epic Automation Log.
- [ ] **Step 6: Commit** `fix(payroll): hari kerja dari roster per tanggal; libur per tipe jadwal (EPIC-079)`.

### Task 6: Host Live, Produksi, CRM menjadi Shift

**Files:**
- Modify: `backend/database/migrations/deltas/20261103090000_hris_shift_system_holidays.sql` (part 2).
  - Replace `hris.work_hours_rule_targets` so its title exclusion regex also contains `host live|production|crm`.
  - Replace `hris.employee_department_fixed_week` to skip employees whose title matches the same shift-title regex.
  - Then, for active `non_shift` employees whose `positions.title ~* '(host live|production|crm)'`:
    - set `schedule_type = 'shift'`;
    - delete weekly rows with `effective_from >= today`;
    - end running weekly rows `today - 1`;
    - insert an `employee.schedule_type` audit (actor `Sistem (sistem shift EPIC-079)`, reason `Sistem shift: Host Live/Produksi/CRM (EPIC-079)`);
    - `SELECT count(*)`.
- Test: `backend/internal/modules/hris/shift_system_integration_test.go`

- [ ] **Step 1: Failing test** `TestShiftSystemTitlesBecomeShift`. The test calls the conversion as a function `hris.apply_shift_system_titles(p_from date, p_actor text) RETURNS integer` that the migration defines and invokes once.
  - Host Live Staff (Marketing, NS week), Production Staff (Warehouse, OPS week), CRM Supervisor → `shift`, no open weekly rows from `p_from`, 3 audits.
  - Finance staff on NS unchanged.
  - Then `work_hours_rule_targets(p_from)` excludes all three.
  - Moving the Production Staff from Warehouse to Marketing adds no fixed week.
- [ ] **Step 2: Run, verify FAIL; implement; apply the migration to the test DB; run, verify PASS.**
- [ ] **Step 3: Commit** `feat(hris): Host Live, Produksi, CRM masuk sistem shift (EPIC-079)`.

### Task 7: ESS jam kerja sistem shift + acuan

**Files:**
- Modify: `frontend/src/lib/hris/work-hours.ts`.
  - Add the group `{ name: "Karyawan Sistem Shift", scope: "Retail Operasional, Host Live, CRM dan Produksi.", rows: [["Hari", "Senin – Minggu (libur 1 hari setiap minggu)"], ["Shift 1", "08.00 – 16.00 WIB"], ["Shift 2", "11.00 – 19.00 WIB"], ["Shift 3", "13.30 – 21.30 WIB"]] }`.
  - Add `notes: string[]` holding, verbatim, the unit-kerja sentence, the "Karyawan wajib menaati…" sentence, and both holiday paragraphs.
- Modify: `frontend/src/features/hris/lateness/components/lateness-policy.tsx`. Render the third group and the notes; the grid becomes `sm:grid-cols-2 lg:grid-cols-3`; HP stays one column.
- Modify docs:
  - `docs/hris/acuan-mom-20261019/status-requirement.md` (AB-07 evidence + holiday rule);
  - `pertanyaan-rekomendasi.md` (row TnC-SHIFT 02-11 with the 4 answers);
  - `.claude/skills/paddy-hris-acuan/SKILL.md` (rows "Sistem shift" and "Hari libur");
  - `docs/epics/README.md` (EPIC-079 status).
- Test: `frontend/src/features/hris/lateness/components/my-lateness-card.test.tsx`

- [ ] **Step 1: Failing vitest:** the open panel has a region "Karyawan Sistem Shift" containing "13.30 – 21.30 WIB", and the text "tidak mengikuti hari libur nasional".
- [ ] **Step 2: Implement; run vitest; PASS.**
- [ ] **Step 3: Commit** `feat(hris): ESS jam kerja sistem shift + acuan hari libur (EPIC-079)`.

### Task 8: Gates, staging, QA

- [ ] Go: `gofmt -l`, `go vet ./...`, `go test -p 1 ./internal/...`. Frontend: `npx tsc --noEmit`, eslint, `npx vitest run`. All green.
- [ ] Check disk first (`df -h /`, ≥ 10 GB free, else `docker builder prune -af`). Back up the staging DB, then deploy the branch (`tools/deploy-paddydev.sh` from `.worktrees/deploy-e071`, `umask 022`).
- [ ] QA staging with QA demo data (triggers off, removed afterwards), HP 360 px + laptop:
  - store crew rostered on 25-12;
  - HQ on 25-12 and on 24-12 with the toggle off and on;
  - shift-staff leave 21–25 Dec;
  - a 40-hour roster warning;
  - ESS panel.

  Require 0 console errors, 0 API ≥ 400, and no horizontal overflow.
- [ ] Epic Automation Log, memory, and PR (to `paddydev`). Merge and production deploy only with explicit user approval, following `alur-migrasi-production` (rehearsal on a prod copy showing the converted names + payroll simulation first).
