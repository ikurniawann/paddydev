# Absensi Tim untuk Atasan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Atasan melihat presensi harian seluruh timnya (langsung dan tidak langsung) di Area Karyawan → Absensi Tim, plus kartu ringkasan di Beranda.

**Architecture:** Server memakai ulang rekap harian HR (`RosterRows` + `buildRoster`) yang dibatasi ke subtree bawahan (`delegateSubtree`). Hasilnya disajikan di `GET /api/hris/me/team-attendance` tanpa foto selfie, ditambah nama atasan per anggota. Frontend memakai ulang `AttendanceMonitoringTab` dalam lingkup "team" di halaman ESS baru. Menu ESS baru di-grant ke semua peran dan hanya tampil untuk akun yang punya bawahan.

**Tech Stack:** Go (`backend/internal/modules/hris`, tes integrasi Postgres), Next.js (Vitest + Testing Library), migrasi SQL delta.

**Spec:** `docs/epics/EPIC-074-absensi-tim-atasan.md` (R1–R8).

## Global Constraints
- Tim = karyawan aktif di subtree `reporting_to` ≤ 10 tingkat, atasan dikecualikan (R1); pakai `delegateSubtree` (`schedule_delegates.go`).
- Respons tim tidak memuat `clock_in_photo_url`/`clock_out_photo_url`: nilainya `null` (R2).
- Hanya baca (R4); tidak ada rute tulis baru.
- Menu: kode `ess.team-attendance`, nama "Absensi Tim", path `/dashboard/me/absensi-tim`, ikon `users`, `order_number` 44, actions `["read"]`, parent `ess`, grant semua role (R6).
- Setelah menambah rute Go: `make -C backend generate` (manifest proxy `frontend/src/lib/go-routes.generated.json`).
- Tes Go integrasi dijalankan `go test -p 1` (DB uji bersama).

## Review Focus
1. Atasan dengan siklus `reporting_to` (A→B→A): daftar berhenti, tidak duplikat, atasan tidak memuat dirinya. Tes `TestTeamAttendanceCycleAndSelf` (Task 1).
2. Karyawan nonaktif di tengah rantai: bawahannya ikut terputus, sama dengan `delegateSubtree`. Tes di `TestTeamAttendanceScope` (Task 1).
3. Akun tanpa data karyawan (admin) membuka halaman: daftar kosong tanpa galat 403/500. Tes `TestTeamAttendanceUnlinked` (Task 1).
4. Super admin yang menjadi bawahan: tetap dikecualikan seperti di roster HR (`superAdmins`). Tercakup oleh pemakaian ulang `RosterRows` (Task 1).
5. Tim besar (≥ 100) di HP: daftar per shift tetap bisa digulir dan tidak meluber. Dicek di QA probe (Task 5).

---

### Task 1: Server — `GET /api/hris/me/team-attendance`

**Files:**
- Modify: `backend/internal/modules/hris/postgres_attendance.go` (`RosterRows` menerima `employeeIDs []string`; nil = semua), `attendance.go` (interface `RosterRows`, `DailyRoster` meneruskan nil)
- Create: `backend/internal/modules/hris/team_attendance.go`
- Modify: `backend/internal/modules/hris/routes.go` (rute), `schedule_days_reader_integration_test.go` (panggilan `RosterRows` + `nil`)
- Test: `backend/internal/modules/hris/team_attendance_integration_test.go`

**Interfaces — Produces:**
- `func (s *store) TeamMembers(ctx context.Context, managerID string) ([]teamMember, error)`: subtree via `delegateSubtree`. `type teamMember struct{ ID, ReportingToName string; Direct bool }`.
- `func (s *Service) TeamAttendance(ctx context.Context, managerID *string, date, today string) (*Row, error)`: bentuk sama dengan `DailyRoster` (`date, day_of_week, is_today, summary, shifts, holidays, employees`). Setiap employee menambah `reporting_to_name` (string) dan `direct` (bool). `attendance.clock_in_photo_url`/`clock_out_photo_url` bernilai null. `managerID == nil` atau tim kosong → `employees: []`, summary nol.
- Rute `GET /api/hris/me/team-attendance?date=YYYY-MM-DD` (`withActor`). Tanggal tidak valid → 400 "Format tanggal tidak valid".

- [ ] **Tes gagal** `TestTeamAttendanceScope`:
  - Atasan A punya koordinator K (langsung) dan staf S (melapor ke K). X di luar tim.
  - S clock-in hari itu (insert `hris.attendance` dengan foto).
  - `GET …?date=2026-10-05` sebagai A → employees = {K, S}.
  - S: `direct` false, `reporting_to_name` = nama K, `attendance.clock_in_photo_url` null, status `hadir`/`terlambat`.
  - K: `direct` true. X tidak ada.
  - Karyawan nonaktif N di antara A dan bawahan N2 → N2 tidak ada.
- [ ] **Tes gagal** `TestTeamAttendanceCycleAndSelf`: A→B→A (reporting_to saling menunjuk). Sebagai A → employees = {B}, A tidak termuat, tanpa duplikat.
- [ ] **Tes gagal** `TestTeamAttendanceUnlinked`: akun tanpa karyawan → 200, `employees` []; `?date=salah` → 400.
- [ ] Jalankan: `go test -p 1 ./internal/modules/hris/ -run TestTeamAttendance` → FAIL (404 route).
- [ ] Implementasi.
  - `RosterRows` + `employeeIDs`: tambah `AND ($4::uuid[] IS NULL OR e.id = ANY($4::uuid[]))`.
  - `TeamAttendance`: `TeamMembers` → ids → kerangka `DailyRoster` dengan ids → hapus foto, tambahkan atasan.
- [ ] Hijau. Jalankan `go vet`, `go test -p 1 ./internal/modules/hris/...`, lalu `make -C backend generate` (manifest berubah: commit). Commit.

### Task 2: Menu ESS "Absensi Tim" + visibilitas

**Files:**
- Create: `backend/database/migrations/deltas/20261030120000_ess_team_attendance_menu.sql` (pola `20261029120000_schedule_delegates.sql` bagian menu: insert/upsert menu, `module='ess'`, `level=2`, parent `ess`, grant semua role dari `permission_context`)
- Modify: `backend/database/seeders/iam-menus.sql` (baris menu + daftar ESS)
- Create: `frontend/src/lib/iam/team.ts` + `team.test.ts`
- Modify: `frontend/src/app/dashboard/(dashboard)/layout.tsx`, `frontend/src/app/dashboard/pos/layout.tsx`

**Interfaces — Produces:**
- `export const ABSENSI_TIM_PATH = "/dashboard/me/absensi-tim"`.
- `export const hasActiveTeam = cache(async (userId: string): Promise<boolean>)`: karyawan aktif ber-`user_id` ini (id terkecil, sama dengan `isScheduleDelegate`) punya ≥ 1 bawahan aktif langsung (`reporting_to = id AND id <> e.id`). Bawahan langsung cukup, karena subtree kosong ⇔ tidak ada bawahan langsung aktif. Gagal dibaca → false.

- [ ] Tes `team.test.ts` (mock `./pg-client` seperti `schedule-delegate.test.ts`): `{ok:true}` → true; `{ok:false}` → false; throw → false; IAM DB tidak dikonfigurasi → false.
- [ ] Implementasi. Kedua layout: `if (!teamManager) navItems = dropNavHref(navItems, ABSENSI_TIM_PATH)`, dipanggil paralel di `Promise.all` yang sama.
- [ ] Migrasi diterapkan di DB uji (`TEST_DATABASE_URL`) lewat `make -C backend migrate` (atau jalankan filenya dengan psql). Tes Go `TestMigrationEssTeamAttendanceMenu`: menu ada, parent `ess`, dan setiap role punya grant.
- [ ] Commit.

### Task 3: Halaman Absensi Tim

**Files:**
- Modify: `frontend/src/features/hris/attendance/{api.ts,queries.ts,query-keys.ts,types.ts}`: `fetchTeamAttendance(date)`, `useTeamAttendance(date, enabled = true)`, key `attendanceQueryKeys.team(date)`. `RosterEmployee` + `reporting_to_name?: string; direct?: boolean`.
- Modify: `frontend/src/features/hris/attendance/components/attendance-monitoring-tab.tsx`: prop `scope?: "hr" | "team"` (default "hr"). "team" memakai `useTeamAttendance` dan teks kosong "Belum ada anggota tim."; hook HR tidak aktif.
- Modify: `frontend/src/features/hris/attendance/components/roster-row.tsx`: bila `emp.direct === false && emp.reporting_to_name`, tampilkan baris kecil `Atasan: {nama}`.
- Create: `frontend/src/features/hris/ess/components/absensi-tim-page.tsx` (`EssAbsensiTimPage`: judul "Absensi Tim", deskripsi R1/R4, `<AttendanceMonitoringTab scope="team" />`) + test; export di `features/hris/ess/index.ts`.
- Create: `frontend/src/app/dashboard/(dashboard)/me/absensi-tim/page.tsx` (pola `me/jadwal-toko/page.tsx`).

- [ ] Tes `absensi-tim-page.test.tsx` (mock `fetchTeamAttendance`):
  - Dua anggota (langsung dan tidak langsung): nama keduanya tampil, dan "Atasan: Koordinator K" tampil untuk yang tidak langsung.
  - `fetchDailyRoster` tidak dipanggil.
  - Data kosong menampilkan "Belum ada anggota tim."
- [ ] Tes `attendance-monitoring-tab` yang ada tetap hijau (scope default "hr").
- [ ] Implementasi, hijau, commit.

### Task 4: Kartu Beranda "Absensi Tim hari ini"

**Files:**
- Create: `frontend/src/features/hris/ess/components/team-attendance-card.tsx` + test
- Modify: `frontend/src/features/hris/ess/components/beranda-page.tsx` (render setelah `JadwalTokoShortcut`; panggil `useTeamAttendance(todayWib())` di atas halaman agar paralel)

**Interfaces — Produces:** `TeamAttendanceCard()`: data hari ini; `employees.length === 0` atau loading/error → null. Link `/dashboard/me/absensi-tim` berlabel "Absensi Tim". Teks `{hadir+terlambat} hadir · {terlambat} terlambat · {belum_absen} belum absen` dari `summary`.

- [ ] Tes:
  - Summary `{hadir:3, terlambat:1, belum_absen:2}` dengan employees 6 menampilkan "4 hadir · 1 terlambat · 2 belum absen", dan href benar.
  - Employees kosong → container kosong.
- [ ] Implementasi, hijau, commit.

### Task 5: Gerbang, QA, dokumen
- [ ] `go vet`, `go test -p 1 ./internal/modules/hris/...`, frontend `tsc`, `lint`, `pnpm test`.
- [ ] Staging: backup, deploy branch (migrasi menu ikut).
- [ ] Seed demo:
  - atasan A (akun employee) dengan koordinator K dan 2 staf di bawah K, ditambah 1 staf yang clock-in hari ini;
  - karyawan biasa E (akun).
- [ ] Probe laptop 1280 + HP 390:
  - A melihat menu (sidebar dan sheet Lainnya) dan kartu Beranda;
  - halaman Absensi Tim memuat K + 2 staf, dengan "Atasan: K";
  - E tidak melihat menu/kartu, dan halamannya kosong;
  - tidak ada yang meluber, 0 error konsol, 0 API ≥ 400.
- [ ] Hapus demo.
- [ ] Automation Log EPIC-074 + README registry, lalu PR.
