# Checkout Otomatis per Jabatan & 2 Shift dalam Sehari Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Absen terbuka karyawan yang jabatannya (dan, untuk Shopkeeper, tokonya) diatur "checkout otomatis" ditutup sistem di jam selesai shift. Satu hari boleh berisi 2 shift dalam 1 absen gabungan.

**Architecture:**
- Satu tabel aturan `hris.auto_checkout_rules` (jabatan + cabang opsional), diatur lewat API/UI HR dan pengatur jadwal toko.
- Satu job `RunAutoCheckout(ctx, now)` dengan loop 5 menit, mengikuti pola `GapDigestLoop`.
- Dua shift = kolom `shift2_id` di jadwal per tanggal dan pola mingguan, dengan kode `A+B`.
- Clock-in mengisi `scheduled_end` dan `break_minutes` dari kedua shift, sehingga trigger `work_hours` yang sudah ada tetap benar.

**Tech Stack:** Go (hris), PostgreSQL migration, Next.js + Vitest.

**Spec:** `docs/epics/EPIC-080-checkout-otomatis.md` (R1–R10). Dasar: EPIC-079 (`docs/epics/EPIC-079-jam-kerja-sistem-shift.md`), EPIC-071 (pengatur jadwal toko).

## Global Constraints
- Branch ditumpuk di atas `docs/epic079-jam-kerja-shift`. Jangan ubah perilaku EPIC-079 di luar yang disebut R9.
- `clock_out` hasil otomatis = `scheduled_end`, bukan `now()`. Hanya baris dengan `clock_out IS NULL` yang diubah.
- Tanpa notifikasi "Jam Kerja Kurang" untuk baris otomatis. Audit aktor: `Sistem (checkout otomatis)`.
- Kode 2 shift: `KODE1+KODE2`; shift 2 mulai ≥ selesai shift 1 (error bila tidak).
- `break_minutes` absen hari 2 shift = istirahat 1 + istirahat 2 + jeda antar-shift (menit).
- Pengatur jadwal toko hanya boleh mengatur aturan dengan `branch_id` di tokonya.
- Bahasa UI Indonesia. HP 360 px tanpa scroll horizontal.
- Uji integrasi Go: `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e071?sslmode=disable' go test -p 1`. Setelah migrasi baru, jalankan `go run ./cmd/migrate -apply` ke DB uji.

## Review Focus
1. **Absen lintas tengah malam** (shift malam/CS3 sampai 00.00): `scheduled_end` di hari berikutnya; job mencari sampai hari ini − 2. Diuji di Task 4.
2. **Periode absensi sudah ditutup**: job melewati baris itu tanpa error dan tanpa menghentikan baris lain. Diuji di Task 4.
3. **Karyawan pindah jabatan atau toko setelah clock-in**: aturan dievaluasi saat job berjalan, dari jabatan sekarang dan cabang clock-in. Diuji di Task 4.
4. **Clock-in sesudah jam selesai shift** (datang sangat terlambat): tidak ditutup otomatis (`clock_in >= scheduled_end`). Diuji di Task 4.
5. **Kode `A+B` tidak valid di upload xlsx** (shift tidak dikenal, tumpang tindih, `L+SPG1`): error per baris, tidak menyimpan sebagian. Diuji di Task 2.

---

### Task 1: Skema — aturan, penanda, shift kedua

**Files:**
- Create: `backend/database/migrations/deltas/20261104090000_hris_auto_checkout.sql`
- Test: `backend/internal/modules/hris/auto_checkout_schema_integration_test.go`

**Interfaces:**
- Produces:
  - Tabel `hris.auto_checkout_rules(id uuid pk, position_id uuid not null → hris.positions, branch_id uuid null → configuration.branches, is_active bool not null default true, created_by, updated_by, created_at, updated_at)`, unik pada `(position_id, coalesce(branch_id, '00000000-0000-0000-0000-000000000000'))`.
  - `hris.attendance.clock_out_auto boolean NOT NULL DEFAULT false`.
  - `hris.employee_shift_days.shift2_id` dan `hris.employee_shifts.shift2_id`: `uuid null → hris.shifts`, `CHECK (shift2_id IS NULL OR shift_id IS NOT NULL)`.
  - `hris.v_employee_schedule` ditambah kolom `shift2_id`, ditaruh setelah `priority` supaya pembaca lama tidak bergeser.
  - `public.fn_notify_attendance_changes()` di-replace: cabang UPDATE clock_out dilewati bila `NEW.clock_out_auto`. Selebihnya salin persis dari `bootstrap/00000000001000_functions.sql:476-523`.
  - Seed aturan "semua lokasi" untuk posisi aktif yang `title ~* '(host live|crm)'` (`ON CONFLICT DO NOTHING`).

- [ ] **Step 1: Failing test** `TestAutoCheckoutSchema`:
  - insert aturan duplikat (posisi sama, cabang NULL) → unique violation;
  - `shift2_id` tanpa `shift_id` → check violation;
  - view mengembalikan `shift2_id` untuk baris per tanggal dan mingguan;
  - UPDATE attendance mengisi `clock_out` dengan `clock_out_auto=true` dan work_hours < 7 → 0 baris baru di `public.notifications` bertitel "Jam Kerja Kurang"; dengan `false` → 1 baris.
- [ ] **Step 2: Run, verify FAIL; write migration; apply to test DB; run, verify PASS.**
- [ ] **Step 3: Commit** `feat(hris): skema checkout otomatis dan shift kedua (EPIC-080)`.

### Task 2: Jadwal 2 shift di grid, upload, dan pola mingguan (backend)

**Files:**
- Modify: `backend/internal/modules/hris/domain/schedule_upload.go`.
  - Parse `KODE1+KODE2` menjadi `plannedDay{Shift, Shift2}`.
  - `UploadShift` sudah membawa `BreakMinutes` (EPIC-079).
  - Validasi error: kode tidak dikenal; `L+…`; shift 2 mulai sebelum shift 1 selesai (pakai menit seperti `shiftNetMinutes`, termasuk overnight). Pesan: "Shift kedua harus mulai setelah shift pertama selesai (SPG1 selesai 16.00, SPG2 mulai 11.00)".
  - `sequenceWarnings`: jam bersih hari = jumlah kedua shift; jeda < 8 jam diukur dari selesai shift 2.
- Modify: `backend/internal/modules/hris/schedule_days.go`. Grid response sel memuat `shift2_id`/kode gabungan; penulisan per sel dan upload menyimpan `shift2_id`; template xlsx menulis `A+B`.
- Modify: pola mingguan (`employees.go` + `http_employees.go` pattern save). Tiap hari menerima `shift2_id` opsional dengan validasi yang sama.
- Modify: `backend/internal/modules/hris/files.go:65` `ShiftPattern`. Ikut memilih `shift2_id` beserta jam, istirahat, dan overnight shift 2 (LEFT JOIN kedua).
- Test: `domain/schedule_upload_test.go`, `schedule_cells_test.go` atau test integrasi grid terdekat.

**Interfaces:**
- Produces:
  - `domain.ScheduleRow.Shift2ID *string`.
  - `plannedDay.Shift2 *UploadShift`.
  - Cell code string `"PAGI4+SORE4"` pada API grid (field yang sama dengan kode tunggal).
  - `func ParseScheduleCode(code string) (first, second string, err error)` di domain.

- [ ] **Step 1: Failing tests:**
  - `TestParseScheduleCodeTwoShifts`: `"PAGI4+SORE4"` → (`PAGI4`, `SORE4`); `"L+SPG1"` → error; `"SPG1+"` → error.
  - `TestTwoShiftOverlapRejected`: SPG1+SPG2 → error with the message above.
  - `TestTwoShiftDayCountsBothForFortyHours` (fixture shifts PAGI4 08:00–12:00 and SORE4 17:00–21:00, break 0 → 8 h net per day): off, 5×(PAGI4+SORE4), off (edited) → no 40-hour warning (40 h); off, 4×(PAGI4+SORE4), off (edited) → warning "baru 32 jam".
  - Integration: per-cell save `PAGI4+SORE4` stores `shift2_id`, and the grid GET returns `"PAGI4+SORE4"`; upload row with `SPG1+SPG2` → 422/400 per-row error and nothing saved (Review Focus 5).
- [ ] **Step 2: Run, verify FAIL; implement; run hris tests; PASS.**
- [ ] **Step 3: Commit** `feat(hris): jadwal 2 shift dalam sehari (KODE1+KODE2) (EPIC-080)`.

### Task 3: Clock-in hari 2 shift + tampilan jadwal

**Files:**
- Modify: `backend/internal/modules/hris/attendance_clock.go:140-207`.
  - `shiftSnap` mendapat shift 2 (jam, istirahat, overnight).
  - `scheduled_end` = selesai shift 2 (`ScheduledWindow` shift 2 di tanggal yang sama, +1 hari bila overnight atau bila mulai shift 2 < mulai shift 1).
  - Terlambat tetap dinilai dari shift 1.
  - Isi `break_minutes` = istirahat 1 + istirahat 2 + jeda (menit antara selesai shift 1 dan mulai shift 2). Untuk hari 1 shift isi `break_minutes` = istirahat shift itu, sesuai master. Perilaku lama memakai default tabel 60; nilai master untuk semua shift saat ini 60, kecuali OPSSAT = 0.
- Modify: roster harian dan "Absensi Hari Ini" ESS. Nama shift menjadi "PAGI4 + SORE4" dan jam "08.00–12.00, 17.00–21.00" (cari pembentuk `today_shift` di ESS beranda dan `RosterRows`).
- Test: `backend/internal/modules/hris/auto_checkout_integration_test.go` (dibuat di sini, dipakai Task 4).

- [ ] **Step 1: Failing test** `TestClockInTwoShiftDay`:
  - jadwal per tanggal dengan dua shift yang tidak tumpang tindih: shift A 08:00–12:00 break 0 dan shift B 17:00–21:00 break 0.
  - Clock-in 08:05 → `scheduled_start` 08:00, `scheduled_end` 21:00, `break_minutes` 300, terlambat menurut toleransi shift A.
  - Clock-out 21:00 → `work_hours` 8.
  - Hari 1 shift OPSSAT → `break_minutes` 0.
- [ ] **Step 2: Run, verify FAIL; implement; PASS.**
- [ ] **Step 3: Commit** `feat(hris): clock-in hari 2 shift mengisi jam selesai dan istirahat gabungan (EPIC-080)`.

### Task 4: Job checkout otomatis

**Files:**
- Create: `backend/internal/modules/hris/auto_checkout.go`:
  - `const AutoCheckoutEvery = 5 * time.Minute`;
  - `func (s *Service) RunAutoCheckout(ctx context.Context, now time.Time) (closed int, err error)`;
  - `func (s *Service) AutoCheckoutLoop(ctx context.Context, every time.Duration)`, dengan panic-recover dan log seperti `gapDigestTick`.
- Modify: `backend/internal/modules/hris/module.go:85-89`. Daftarkan `go svc.AutoCheckoutLoop(...)` di blok yang sama (tidak jalan saat `AppEnv == "test"`).
- Test: `auto_checkout_integration_test.go`.

**Interfaces:**
- Consumes: `auto_checkout_rules`, `attendance.clock_out_auto` (Task 1); `scheduled_end` 2 shift (Task 3).

Algorithm (one SQL UPDATE … FROM per run, inside a transaction):
- Candidates: `a.clock_out IS NULL AND a.scheduled_end IS NOT NULL AND a.scheduled_end <= $now AND a.clock_in < a.scheduled_end AND a.date >= ($now WIB date − 2)`.
- Joined to `hris.employees e` (`e.job_title_id`) and an active rule `r` (`r.position_id = e.job_title_id AND (r.branch_id IS NULL OR r.branch_id = a.branch_id)`).
- Excluding rows whose date is in a closed attendance period: use the same predicate as the `attendance_closed_period_guard` function. Do not set `allow_closed_period_write`.
- SET `clock_out = a.scheduled_end`, `clock_out_auto = true`, `clock_out_geofence = 'unchecked'`, `notes = concat_ws(' · ', nullif(a.notes,''), 'Checkout otomatis pukul ' || to_char(a.scheduled_end AT TIME ZONE 'Asia/Jakarta','HH24.MI') || ' (jam selesai shift)')`, `RETURNING a.id, a.employee_id, a.scheduled_end`.
- One `audit.audit_log` row per closed row: action `attendance.auto_checkout`, actor_name `Sistem (checkout otomatis)`, entity `hris.attendance`, after `{clock_out, clock_out_auto:true}`.

- [ ] **Step 1: Failing test** `TestRunAutoCheckout`:
  - Host Live (rule all branches), clock-in 07:58, scheduled_end 16:00; run at 15:59 → 0 closed; run at 16:05 → 1 closed with clock_out 16:00, `clock_out_auto`, note text, audit row, and no "Jam Kerja Kurang" notification.
  - Shopkeeper rule only for branch A: row at A closed, row at B untouched.
  - Manual clock-out 15:30 untouched.
  - Clock-in 16:30 after scheduled_end 16:00 untouched (Review Focus 4).
  - Row in a closed period untouched and other rows still closed (Review Focus 2).
  - Overnight CS3 16:00–00:00 dated yesterday closed at 00:00 (Review Focus 1).
  - Rule switched off → untouched.
  - Employee whose job title changed to a ruled title after clock-in → closed (Review Focus 3).
  - Second run → 0 (idempotent).
- [ ] **Step 2: Run, verify FAIL; implement; PASS.** Also run `TestGapDigest*` to confirm the loop registration did not change them.
- [ ] **Step 3: Commit** `feat(hris): checkout otomatis di jam selesai shift (EPIC-080)`.

### Task 5: API + halaman aturan checkout otomatis (HR + pengatur jadwal toko)

**Files:**
- Create: `backend/internal/modules/hris/auto_checkout_rules.go` + routes in `routes.go`.
  - `GET /api/hris/auto-checkout-rules`. HR (menu kepegawaian) sees all. A delegate sees rules for their branches plus all-location rules, the latter read-only (flag `editable`). Rows carry `position_title`, `branch_name` and `editable`.
  - `POST /api/hris/auto-checkout-rules {position_id, branch_id|null}`.
  - `PATCH /api/hris/auto-checkout-rules/{id} {is_active}`.
  - `DELETE /api/hris/auto-checkout-rules/{id}`.
  - Delegate access via the existing delegate wrapper (`loadDelegateAccess`, `storeScope`). A delegate with `branch_id` null or outside their branches gets 403 "Hanya untuk toko yang Anda atur".
  - Audit `attendance.auto_checkout_rule.create|update|delete` with before/after.
  - Run `make generate` and add the path to `goNativePrefixes` in `frontend/src/lib/backend-routes.test.ts`.
- Create: `frontend/src/features/hris/auto-checkout/` (api, queries, `auto-checkout-page.tsx`).
  - Table: Jabatan · Lokasi ("Semua lokasi" / toko) · Aktif (switch) · Hapus.
  - Dialog "Tambah aturan": pemilih jabatan; pemilih lokasi, terbatas toko pengatur untuk delegate.
  - Short note: "Absen karyawan yang belum clock-out ditutup otomatis di jam selesai shift."
  - Page under HRIS › Kepegawaian (HR) and inside the store schedule area for delegates. Follow how EPIC-071 exposes delegate pages (`frontend/src/features/hris/schedule-delegates`, menu config).
- Test: `auto_checkout_rules_integration_test.go`, `auto-checkout-page.test.tsx`.

- [ ] **Step 1: Failing tests:**
  - HR: create, list, toggle, delete; duplicate → 409 "Aturan untuk jabatan dan lokasi ini sudah ada".
  - Delegate: create for own branch → 201; for another branch or all locations → 403; PATCH an all-location rule → 403; list shows the all-location rule with `editable:false`.
  - Each write leaves one audit row.
  - Vitest: table renders rows; the switch sends PATCH; a delegate sees a disabled switch for the all-location row; at 360 px the table becomes stacked cards (follow existing responsive table pattern in hris).
- [ ] **Step 2: Run, verify FAIL; implement; PASS.**
- [ ] **Step 3: Commit** `feat(hris): pengaturan checkout otomatis per jabatan dan toko (EPIC-080)`.

### Task 6: UI jadwal 2 shift + penanda checkout otomatis

**Files:**
- Modify: `frontend/src/features/hris/schedules/components/shift-picker.tsx`, `schedule-grid.tsx`, `pattern-dialog.tsx`, `schedule-upload-dialog.tsx` (help text).
  - The shift picker offers "Tambah shift kedua" after a shift is chosen.
  - The cell shows `PAGI4+SORE4`.
  - The pattern dialog has an optional second select per day.
  - The 40-hour/total-hours note counts both shifts.
- Modify: `frontend/src/components/hris/AttendanceDayDialog.tsx`, `AttendanceDayCell.tsx`, and the HR Monitoring/Rekap attendance tables (grep for `clock_out` rendering in `frontend/src/features/hris/attendance*`). Show badge "Checkout otomatis" when `clock_out_auto`. Make sure the Go list/detail endpoints return `clock_out_auto`; add it to the select lists if missing.
- Test: `pattern-dialog.test.tsx`, `schedule-grid.test.tsx`, `AttendanceDayCell.test.tsx` (or dialog test).

- [ ] **Step 1: Failing vitest:**
  - picker builds `PAGI4+SORE4` and the grid sends that code;
  - pattern dialog sends `shift2_id`;
  - a day with `clock_out_auto` shows "Checkout otomatis".
- [ ] **Step 2: Implement; vitest + tsc + eslint; PASS.**
- [ ] **Step 3: Commit** `feat(hris): UI jadwal 2 shift dan penanda checkout otomatis (EPIC-080)`.

### Task 7: Acuan, gates, staging, QA

- [ ] Docs:
  - `docs/hris/acuan-mom-20261019/status-requirement.md` (AB rows: checkout otomatis, 2 shift);
  - `pertanyaan-rekomendasi.md` (row CO-OTOMATIS 02-11 with the 3 answers + R4 my call);
  - `.claude/skills/paddy-hris-acuan/SKILL.md` (row "Checkout otomatis");
  - `docs/epics/README.md` (EPIC-080 row, status `review`); epic status `review`.
- [ ] Gates:
  - Go: `gofmt -l`, `go vet ./...`, `go test -p 1 ./internal/...`.
  - Frontend: `npx tsc --noEmit`, eslint, `npx vitest run`.
- [ ] Staging. Check disk (≥ 10 GB) first, then back up the staging DB, then deploy the branch (`tools/deploy-paddydev.sh`, `umask 022`).
- [ ] QA staging with QA080 demo data (triggers off; deleted afterwards), HP 360 px + laptop:
  - aturan Host Live dan Shopkeeper per toko;
  - jadwal `A+B`;
  - clock-in, lalu jalankan `RunAutoCheckout` lewat waktu (atau tunggu loop);
  - penanda di kalender;
  - pengatur jadwal toko hanya tokonya.
- [ ] Epic Automation Log, memory, PR to `paddydev` (base `docs/epic079-jam-kerja-shift`, or `main` once #63 is merged). Merge and production only with explicit user approval. Production rehearsal on a prod copy lists the seeded Host Live/CRM rules and the open rows the first run would close.
