# HRIS Absensi — Tindak Lanjut (EPIC-067) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Menutup tiga tindak lanjut EPIC-065 yang tertunda:
1. Export Rekap absensi memuat label kurang jam dan jam minimum.
2. HR menerima ringkasan harian absen terlewat (notifikasi in-app pukul 09.00 WIB dan kartu di HRIS → Absensi).
3. Keputusan HR atas pengajuan absensi, lembur cadangan, cuti cadangan, dan Perjalanan Dinas memakai aksi `approve` di menu daun.

Butir keempat, "HR mengajukan absen atas nama karyawan", **tidak dikerjakan**. HR sudah bisa membuat absen manual dan koreksi langsung.

**Architecture:**
- **Export:** `AttendanceExport` (Go, `files.go`) ikut membaca `a.is_short_hours` dan `a.min_work_hours`. Keduanya adalah snapshot yang sudah diisi trigger EPIC-065, jadi tidak perlu dihitung ulang. Kolom baru ditambahkan di CSV dan XLSX, dan PDF menambahkan penanda di kolom Status. Fallback Next (`/api/hris/attendance/export`) ikut disamakan.
- **Ringkasan absen terlewat:**
  - Pemuat baru bersifat set-based untuk satu tanggal dan semua karyawan aktif. Isinya sama dengan sumber `MyAttendanceGaps`: jadwal, absensi, libur, cuti disetujui, dinas disetujui, pengajuan menunggu, dan rentang kerja. Hasilnya diumpankan ke `domain.AttendanceGaps` per karyawan, sehingga aturannya tetap satu.
  - Endpoint `GET /api/hris/attendance/gap-summary` menyajikan data untuk kartu.
  - Loop latar belakang di modul `hris` memeriksa setiap 5 menit. Mulai pukul 09.00 WIB, loop mengirim satu ringkasan untuk "kemarin". Keunikan per tanggal dijaga oleh tabel penanda `hris.attendance_gap_digests` (PK `date`), sehingga restart atau dua instance API tidak mengirim dua kali.
- **Approve:**
  - Migrasi menawarkan dan memberi aksi `approve` pada daun `hris.workforce.attendance`, `hris.workforce.leaves`, dan `hris.kepegawaian.overtime` kepada role `super_admin`, `admin`, `hrd`, `admin_hris`, lalu menjalankan `iam.sync_hris_staff_roles()` agar role turunan ikut.
  - `Actor` membawa daftar daun yang punya `approve`, dan semua jalur "HR" pada keempat keputusan memakai `a.CanApprove(leaf)` sebagai pengganti `a.IsHRD`.
  - Skrip cek hanya-baca membandingkan siapa yang bisa memutus sebelum dan sesudah migrasi.

**Tech Stack:** Go 1.26 (`backend/internal/modules/hris`, `platform/iam`, `platform/auth`), PostgreSQL 16/17, Next.js versi repo (16.4.2; baca `frontend/node_modules/next/dist/docs/` sebelum menyentuh halaman atau route), TanStack Query, vitest + testing-library.

**Spec:** brief user 26-10-2026 (sesi ini), "HRIS Absensi — tindak lanjut". Bahannya: daftar tindak lanjut di Automation Log `docs/epics/EPIC-065-absensi-lanjutan.md` dan `docs/hris/acuan-mom-20261019/status-requirement.md` (AB-04..AB-06, LA-01). Epic: `docs/epics/EPIC-067-absensi-tindak-lanjut.md`.

## Ruling

- **R1: Butir 4 tidak dikerjakan.** HR memakai absen manual atau koreksi langsung (`attendance_adjust.go`) yang sudah ada. Keputusan ini dicatat di epic.
- **R2: Isi kolom export.**
  - CSV menyisipkan dua kolom setelah `Keterlambatan (menit)` dan sebelum `Catatan`:
    - `Kurang Jam`: `Ya` bila `is_short_hours`, selain itu `-`.
    - `Jam Minimum (jam)`: angka dengan format yang sama dengan `Jam Kerja (jam)`, atau `-` bila kosong.
  - XLSX menyisipkan `Kurang Jam` dan `Jam Minimum` sebelum `Catatan`.
  - PDF tidak menambah kolom karena lebarnya sudah penuh. Kolom Status baris kurang jam diberi akhiran ` · kurang jam`.
  - Penanda kosong memakai `-` (ASCII), sama dengan sel kosong lain di file ini.
- **R3: Fallback Next ikut diubah.** `frontend/src/app/api/hris/attendance/export/route.ts` beserta lib-nya (`@/lib/hris/attendance-report`, `@/lib/hris/attendance-repo`) harus menghasilkan header dan sel yang sama dengan Go untuk CSV dan XLSX.
- **R4: Sumber ringkasan sama dengan `AttendanceGaps`.**
  - Ringkasan dihitung untuk satu tanggal `D` dengan `domain.AttendanceGaps(GapInput{Start: D, End: D, …})` per karyawan, lalu disaring `withinEmployment`.
  - Karyawan yang dihitung: `hris.employees.is_active = true` dengan `join_date <= D` dan (`end_date` kosong atau `end_date >= D`). Akun super_admin dikecualikan, sama dengan `RosterRows`.
  - Status periode absensi tidak memengaruhi isi ringkasan. Ringkasan hanya informasi, dan kartu menyebut bila periode `D` sudah ditutup.
- **R5: Karyawan tanpa jadwal.**
  - Definisi: `domain.ResolveScheduleRow(schedule, D) == -1`, yaitu tidak ada baris jadwal yang berlaku pada `D`. Baris hari libur jadwal tetap dihitung sebagai berjadwal.
  - Mereka tidak dihitung sebagai `missing`, dan jumlahnya ditampilkan terpisah (`unscheduled`).
  - Bila mereka punya baris absensi tanpa jam pulang, `no_clock_out` tetap muncul. Ini sama dengan ESS.
- **R6: Jadwal notifikasi.**
  - `RunGapDigest(ctx, now)` hanya bekerja bila `now` sudah ≥ 09.00 WIB. Tanggalnya `D = TodayWIB(now) − 1 hari`.
  - Langkahnya berjalan dalam satu transaksi:
    1. `INSERT INTO hris.attendance_gap_digests (date) … ON CONFLICT DO NOTHING RETURNING date`. Bila tidak ada baris yang kembali, berhenti.
    2. Hitung ringkasan.
    3. Bila `missing + no_clock_out > 0`, sisipkan notifikasi.
    4. Perbarui baris penanda dengan jumlah dan banyaknya penerima.
  - Bila API mati sepanjang jam 09.00, ringkasan dikirim saat loop pertama berjalan di hari yang sama. Hari yang terlewat tidak dikirim susulan, karena kartu tetap bisa menampilkannya.
- **R7: Penerima dan isi notifikasi.**
  - Penerimanya sama dengan notifikasi HR yang sudah ada (`public.notify_hrd`): semua `configuration.users` dengan `role = 'hrd'`. Go menyisipkan langsung ke `public.notifications`, tanpa lewat `notify_hrd` yang menelan error, agar jumlah penerima bisa dicatat.
  - Isi notifikasi:
    - `type = 'reminder'`
    - `link = '/dashboard/hris/attendance'`
    - Judul: `Ringkasan Absen Terlewat`
    - Pesan: `Kemarin (<Min, 25 Okt 2026>): <m> karyawan belum absen, <n> belum absen pulang.`
    - Bila `unscheduled > 0`, tambahkan ` <u> karyawan tanpa jadwal tidak dihitung.`
    - Nama hari memakai `dayShort` (`domain/schedule_upload.go:78`) dan tanggal memakai `ShortDateID` (`domain/attendance_period.go:127`).
  - Tidak ada WhatsApp.
- **R8: Loop latar belakang.**
  - `New` di `hris/module.go` menjalankan `go svc.GapDigestLoop(context.Background(), GapDigestEvery)` dengan `GapDigestEvery = 5 * time.Minute`, **kecuali** `deps.Config.AppEnv == "test"`. Harness test memakai `New`, dan test memanggil `RunGapDigest` langsung.
  - Loop mencatat error ke log dan tidak pernah panic.
- **R9: Endpoint ringkasan.**
  - Rute: `GET /api/hris/attendance/gap-summary?date=YYYY-MM-DD`, dengan wrapper `withMenu(attendanceManagers, …)` (menu Absensi).
  - Bila `date` kosong, dipakai kemarin (WIB). Format salah memberi 400 `Tanggal tidak valid`. Tanggal setelah hari ini (WIB) memberi 400 `Tanggal tidak boleh di masa depan`.
  - Rute ini hanya ada di Go: tambahkan ke `goNativePrefixes` dan jalankan `make -C backend generate`.
- **R10: Daun untuk `approve`.**

  | Keputusan | Daun |
  |---|---|
  | Pengajuan absensi (EPIC-065) | `hris.workforce.attendance` (`iam.LeafAbsensi`) |
  | Perjalanan Dinas, jalur HR | `hris.workforce.attendance`. Tab HR-nya ada di halaman Absensi; tidak dibuat menu baru |
  | Cuti, HR cadangan/override | `hris.workforce.leaves` (`iam.LeafCuti`) |
  | Lembur, HR cadangan/override | `hris.kepegawaian.overtime` (konstanta baru `iam.LeafLembur`) |

- **R11: `approve` menggantikan `IsHRD` pada jalur HR keputusan.**
  - Yang memakai `a.CanApprove(<daun>)`:
    - `DecideAttendanceRequest`, `ListAttendanceRequests`, badge pengajuan absensi di `NavBadges`, dan akses foto bukti pengajuan absensi (P6 EPIC-065).
    - `DecideOvertime`: `CanDecideOvertime`, `DecideAs`, penetapan `day_type`, dan `CanCorrectOvertimeTimes` di jalur keputusan.
    - `DecideLeave` (`DecideAs`).
    - Semua kuasa HR Perjalanan Dinas: `ListTrips` lihat semua, `CreateTrip` atas nama karyawan, `DecideTrip`, dan cancel oleh HR.
  - Atasan langsung tetap memutus lembur, cuti, dan Perjalanan Dinas bawahannya tanpa menu. Larangan memutus milik sendiri tetap berlaku walau akun punya `approve`.
  - `IsHRD` dan `IsHR` tetap dipakai untuk fitur lain yang bukan keputusan; jangan dihapus.
  - Wrapper rute tidak berubah (`withActor` / `withMenu`). Pemeriksaan `approve` terjadi di service lewat `Actor`, karena atasan langsung tanpa menu juga memakai rute yang sama.
- **R12: Grant `approve`.**
  - Migrasi `20261027090100_iam_hris_approve.sql`:
    1. Menambahkan `"approve"` ke `permission_context.actions` menu `hris.workforce.attendance` dan `hris.workforce.leaves`. `hris.kepegawaian.overtime` sudah memilikinya.
    2. Menambahkan `"approve"` ke `granted_actions` baris `iam.role_menu_permissions` **yang sudah ada** untuk role `super_admin`, `admin`, `hrd`, dan `admin_hris` pada ketiga daun. Baris baru tidak dibuat, karena memberi `approve` tanpa akses menu tidak berguna.
    3. Menjalankan `iam.sync_hris_staff_roles()` dengan penjaga yang sama seperti `20261022090000_iam_hris_delete_masks.sql`.
  - Seeder ikut disamakan: `seeders/iam-menus.sql` (`permission_context`), `seeders/iam-role-permissions.sql`, dan test `seeders/paddy-hris-admins.test.js` bila ekspektasinya berubah.
- **R13: Bukti tidak ada akun yang kehilangan akses.**
  - Skrip hanya-baca `backend/database/sql/checks/2026-10-26_epic067_approve_access.sql` mendaftar akun aktif dengan kolom `before_*` dan `after_*` untuk empat keputusan.
  - **Sebelum:** `role IN ('super_admin','admin','hrd')`. Untuk pengajuan absensi, syaratnya ditambah punya menu prefiks `hris.workforce.attendance`.
  - **Sesudah:** punya `approve` pada daun lewat role IAM yang di-resolve. Logikanya sama dengan `auth.ResolveRoleIDs` + `GrantedMenuActions` + `iam.HasGrantedAction`: `iam.user_roles`, bila tidak ada maka role dengan `code = users.role`; grant `is_active`; menu aktif dan tidak terhapus; kode daun sama atau turunannya.
  - Bagian akhir skrip menghasilkan dua daftar, `lost` (sebelum ya, sesudah tidak) dan `gained`.
  - Gerbang: `lost` kosong di DB test (integration test), di staging sebelum dan sesudah migrasi, dan di gladi salinan production. Bila `lost` tidak kosong di staging atau production, berhenti dan laporkan ke user. Jangan menambah grant ke role non-HR untuk menutupnya.
- **R14: Frontend memakai kapabilitas dari server.**
  - Endpoint baru `GET /api/hris/me/approve-access` (`withActor`) mengembalikan `{ "attendance": bool, "leaves": bool, "overtime": bool }` dari `Actor`. Rute ini hanya di Go: masukkan ke `goNativePrefixes`.
  - Tab `pengajuan` dan `dinas` di `attendance-page.tsx` tampil bila `attendance` true, menggantikan `canManageTrips(user?.role)`.
  - Tombol Setujui/Tolak di halaman HR Cuti (`/dashboard/hris/leaves`) dan Lembur (`overtime-table.tsx`) disembunyikan bila flag daunnya false. Antrean ESS Persetujuan Tim (atasan) tidak berubah.
- **R15: Fallback Next untuk keputusan.**
  - Bila ada route Next untuk keputusan cuti, lembur, atau Perjalanan Dinas yang masih bisa melayani saat `BACKEND_URL` kosong, jalur HR-nya disamakan dengan helper aksi IAM Next (`userHasIamAction` atau setaranya) pada daun yang sama.
  - Bila route itu tidak ada atau memang hanya di Go, catat di laporan task tanpa membuat route baru.

## Global Constraints

- Bahasa UI, pesan error, dan dokumen: Indonesia. Pesan yang dikunci di ruling ditulis persis.
- Zona waktu WIB (`Asia/Jakarta`). Untuk tanggal domain, pakai `domain.TodayWIB` / `domain.WIBOffset`, bukan `time.Local`.
- Migrasi baru: `backend/database/migrations/deltas/20261027090000_attendance_gap_digests.sql` dan `20261027090100_iam_hris_approve.sql`. Keduanya idempoten (`IF NOT EXISTS`, `NOT … ? 'approve'`).
- Route Go baru → `make -C backend generate` dan entri `goNativePrefixes` di `frontend/src/lib/backend-routes.test.ts` dengan komentar `// EPIC-067 …`.
- Test Go integrasi berjalan terhadap `TEST_DATABASE_URL`. Gerbangnya DB segar ala CI `paddy_test_e067` (55499) dengan 0 kegagalan. `paddy_test` lama sudah melenceng (28 kegagalan 403 di baseline `main` `3b8559b`).
- Tidak menyentuh DB live/production. Tidak mengubah data karyawan.
- TDD: test gagal dulu, baru implementasi. Jangan melonggarkan atau menghapus assert yang ada kecuali ekspektasinya memang berubah karena ruling (sebutkan ruling-nya di commit).
- Commit per task, pesan berbahasa Inggris konvensional (`feat(hris): … (EPIC-067)`), diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Dua instance API atau restart sekitar pukul 09.00.** Tepat satu ringkasan per tanggal (PK penanda). Test: dua `RunGapDigest` berurutan untuk `now` yang sama → notifikasi hanya satu set (Task 3).
2. **Karyawan di tepi rentang kerja dan hari non-kerja.** Masuk hari ini, keluar kemarin-lusa, jadwal hari libur, dan libur nasional tidak dihitung `missing`. Cuti dan dinas yang disetujui, serta pengajuan absensi yang menunggu, juga tidak dihitung (Task 2).
3. **Akun ber-role aplikasi `hrd` yang `iam.user_roles`-nya role non-HR.** Akun ini kehilangan jalur HR setelah R11. Test: skrip R13 pada fixture mendeteksinya sebagai `lost`, dan fixture standar (hrd tanpa user_roles, admin_hris, super_admin, admin) memberi `lost` kosong (Task 5).
4. **Self-approval dengan `approve`.** HR yang punya `approve` tetap ditolak saat memutus pengajuannya sendiri di keempat keputusan. Atasan langsung tanpa menu tetap bisa memutus bawahannya (Task 6).
5. **Export baris tanpa cabang atau tanpa aturan jam minimum.** Sel `-`, bukan `0` atau kosong. Baris kurang jam pada XLSX tetap sejajar header (Task 1).

---

### Task 1: Kolom kurang jam & jam minimum di export Rekap (Go + fallback Next)

**Files:**
- Modify: `backend/internal/modules/hris/files.go` (`AttendanceExport` SELECT), `backend/internal/modules/hris/attendance_export.go` (`attendanceCSVHeader`, baris CSV, header/kolom XLSX + lebar kolom + merge header, Status PDF), `backend/internal/modules/hris/domain/documents.go` (`ReportRow`: `IsShortHours bool`, `MinWorkHours *float64`)
- Modify: `frontend/src/lib/hris/attendance-report.ts`, `frontend/src/lib/hris/attendance-repo.ts` (sesuaikan nama file sebenarnya yang dipakai `frontend/src/app/api/hris/attendance/export/route.ts`)
- Test: `backend/internal/modules/hris/files_integration_test.go` (`TestAttendanceExport`), test vitest lib laporan absensi yang sudah ada (atau baru di sebelahnya)

**Interfaces:**
- Produces: `domain.ReportRow.IsShortHours bool`, `domain.ReportRow.MinWorkHours *float64`.

- [ ] **Step 1: Write the failing test (Go).** Perluas `TestAttendanceExport`:
  - Fixture baris `2099-06-02` di-UPDATE `SET min_work_hours = 9, is_short_hours = true` setelah insert. Trigger mempertahankan label bila jam/cabang tidak berubah; bila trigger menimpanya, beri cabang ber-`min_work_hours` 9 pada fixture.
  - CSV: header memuat `"Keterlambatan (menit)","Kurang Jam","Jam Minimum (jam)","Catatan"` (urutan persis). Baris 2099-06-02 memuat `"5","Ya","9","kata ""kutip"""` atau format angka sesuai helper `Jam Kerja`. Baris 2099-06-01 memuat `"-","-"` pada kedua kolom baru.
  - XLSX: baris header berisi `Kurang Jam`, `Jam Minimum` tepat sebelum `Catatan`, dan baris data memuat `Ya`.
  - PDF: teks memuat `kurang jam`.
- [ ] **Step 2: Run, expect FAIL.** Run: `cd backend && TEST_DATABASE_URL=postgres://postgres@127.0.0.1:55499/paddy_test_e067?sslmode=disable go test ./internal/modules/hris -run TestAttendanceExport -count=1`.
- [ ] **Step 3: Implement (Go)** sesuai R2.
- [ ] **Step 4: Run, expect PASS.**
- [ ] **Step 5: Write the failing vitest** untuk builder CSV/XLSX fallback Next, dengan header dan sel yang sama seperti Step 1 (R3). Run `cd frontend && npx vitest run <file>` → FAIL.
- [ ] **Step 6: Implement fallback Next** (SELECT `is_short_hours, min_work_hours`, header, sel). Run → PASS.
- [ ] **Step 7: Commit** `feat(hris): kurang jam & jam minimum di export Rekap absensi (EPIC-067)`.

### Task 2: Ringkasan absen terlewat per tanggal (migrasi penanda + pemuat set-based + endpoint)

**Files:**
- Create: `backend/database/migrations/deltas/20261027090000_attendance_gap_digests.sql`
- Create: `backend/internal/modules/hris/attendance_gap_summary.go` (service, store, handler)
- Modify: `backend/internal/modules/hris/routes.go` (rute R9), `frontend/src/lib/go-routes.generated.json` (generate), `frontend/src/lib/backend-routes.test.ts` (`goNativePrefixes`: `"/api/hris/attendance/gap-summary"`)
- Test: `backend/internal/modules/hris/attendance_gap_summary_integration_test.go`

**Interfaces:**
- Consumes: `domain.AttendanceGaps`, `domain.GapInput`, `domain.ResolveScheduleRow`, `withinEmployment` (`attendance_gaps.go`).
- Produces:
  - `type GapSummaryEmployee struct { EmployeeID, Name string; NIP, Department *string; Kind string; ClockIn *string }` (json `employee_id,name,nip,department,kind,clock_in`)
  - `type GapSummary struct { Date string; Missing, NoClockOut, Unscheduled int; PeriodClosed bool; Employees []GapSummaryEmployee }` (json `date,missing,no_clock_out,unscheduled,period_closed,employees`). `Employees` diurutkan per kind (`missing` lalu `no_clock_out`) kemudian nama.
  - `func (s *Service) AttendanceGapSummary(ctx context.Context, q database.Querier /* atau pola repo modul */, date string) (*GapSummary, error)`: dipakai Task 3 di dalam transaksi. Ikuti pola repo/InTx modul; yang penting Task 3 bisa memanggilnya dalam tx yang sama.
  - Tabel `hris.attendance_gap_digests(date date PRIMARY KEY, missing int, no_clock_out int, unscheduled int, recipients int, created_at timestamptz NOT NULL DEFAULT now(), sent_at timestamptz)`. Grant/RLS mengikuti pola `20261025090000_attendance_requests.sql`.

- [ ] **Step 1: Write the failing test** `TestAttendanceGapSummary`. Fixture tanggal `D` (pakai tanggal tetap jauh di depan atau di belakang agar tidak bentrok data lain, dan isolasi lewat harness yang sudah ada). Karyawan aktif dengan kondisi berikut:

  | Karyawan | Kondisi | Hasil yang diharapkan |
  |---|---|---|
  | A | berjadwal shift, tanpa absensi | missing |
  | B | berjadwal, absen masuk tanpa pulang | no_clock_out |
  | C | berjadwal, absen lengkap | tidak muncul |
  | E | berjadwal, cuti disetujui | tidak muncul |
  | F | berjadwal, dinas disetujui | tidak muncul |
  | G | berjadwal, pengajuan absensi menunggu | tidak muncul |
  | H | `join_date` = D+1 | tidak muncul |
  | I | `end_date` = D−1, masih `is_active` | tidak muncul |
  | J | berjadwal hari libur (baris jadwal tanpa shift) | tidak muncul, bukan unscheduled |
  | K | tanpa jadwal sama sekali | unscheduled = 1, tidak missing |

  Assert jumlah persis. Tambahkan juga:
  - Libur nasional pada `D` → missing 0.
  - Endpoint: HR dengan menu Absensi → 200 dan JSON sesuai. Staff tanpa menu → 403. `date=2026-13-01` → 400 `Tanggal tidak valid`. Besok WIB → 400 `Tanggal tidak boleh di masa depan`. Tanpa `date` → `date` = kemarin WIB.
  - Periode `D` ditutup → `period_closed: true`.
- [ ] **Step 2: Run, expect FAIL** (`-run TestAttendanceGapSummary`).
- [ ] **Step 3: Write migration** dan terapkan ke DB segar:
  ```bash
  MIGRATE_DATABASE_URL=postgres://postgres@127.0.0.1:55499/paddy_test_e067?sslmode=disable go run ./cmd/migrate -apply
  MIGRATE_DATABASE_URL=… go run ./cmd/migrate -verify
  ```
  Hasil yang diharapkan: diterapkan, lalu verify bersih.
- [ ] **Step 4: Implement** pemuat set-based. Satu query per jenis data untuk semua karyawan yang memenuhi R4: baris jadwal (`hris.v_employee_schedule` + shift, bentuk `domain.ScheduleRow`), absensi `D`, cuti disetujui yang mencakup `D`, dinas disetujui yang mencakup `D`, pengajuan menunggu `D`, dan rentang kerja. Libur memakai `ActiveHolidays(D, D)`. Kelompokkan per karyawan, panggil `domain.AttendanceGaps` + `withinEmployment`, dan hitung `unscheduled` sesuai R5. Jangan memanggil pemuat per karyawan dalam loop. Setelah itu buat handler, rute, `make -C backend generate`, dan `goNativePrefixes`.
- [ ] **Step 5: Run, expect PASS.** Jalankan juga `go test ./internal/modules/hris -run 'TestAttendanceGaps' -count=1` (ESS tidak berubah) dan `cd frontend && npx vitest run src/lib/backend-routes.test.ts`.
- [ ] **Step 6: Commit** `feat(hris): ringkasan absen terlewat per tanggal untuk HR (EPIC-067)`.

### Task 3: Notifikasi harian 09.00 WIB

**Files:**
- Create: `backend/internal/modules/hris/attendance_gap_digest.go` (`RunGapDigest`, `GapDigestLoop`, `GapDigestEvery`)
- Modify: `backend/internal/modules/hris/module.go` (R8), `backend/internal/modules/hris/domain/attendance_request.go` atau file domain baru `domain/gap_digest.go` (`GapDigestNotice`)
- Test: `backend/internal/modules/hris/attendance_gap_digest_integration_test.go`, `backend/internal/modules/hris/domain/gap_digest_test.go`

**Interfaces:**
- Consumes: `AttendanceGapSummary` (Task 2), tabel penanda (Task 2).
- Produces:
  - `func GapDigestNotice(date string, missing, noClockOut, unscheduled int) (title, message string)` (domain, pesan persis R7)
  - `type GapDigestResult struct { Date string; Sent bool; Recipients int; Summary *GapSummary }`
  - `func (s *Service) RunGapDigest(ctx context.Context, now time.Time) (*GapDigestResult, error)`. Hasilnya nil bila `now` < 09.00 WIB atau tanggal itu sudah diproses.
  - `func (s *Service) GapDigestLoop(ctx context.Context, every time.Duration)`.

- [ ] **Step 1: Write the failing tests.**
  - Domain `TestGapDigestNotice`: `("2026-10-25", 5, 2, 3)` → judul `Ringkasan Absen Terlewat`, pesan `Kemarin (Min, 25 Okt 2026): 5 karyawan belum absen, 2 belum absen pulang. 3 karyawan tanpa jadwal tidak dihitung.`. Bila `unscheduled = 0`, kalimat terakhir tidak ada. 25-10-2026 adalah hari Minggu (`dayShort` + `ShortDateID`).
  - Integrasi `TestGapDigestRun`:
    - (a) `now` = D+1 08.59 WIB → nil, tanpa baris penanda.
    - (b) `now` = D+1 09.00 WIB dengan fixture 1 missing → satu notifikasi per user `role='hrd'` (buat dua user hrd dan satu user non-hrd; non-hrd tidak menerima). `type='reminder'`, `link='/dashboard/hris/attendance'`. Baris penanda berisi jumlah dan `recipients=2`.
    - (c) panggilan kedua dengan `now` sama → nil, jumlah notifikasi tetap.
    - (d) tanggal lain tanpa celah → baris penanda ada, `sent_at` NULL, tanpa notifikasi.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement** sesuai R6–R8. Semua langkah berjalan dalam satu transaksi. Loop memakai `time.NewTicker(every)` dan juga menjalankan satu kali saat start. Waktu diambil dari `s.now()`.
- [ ] **Step 4: Run, expect PASS.** Jalankan juga `go vet ./...`.
- [ ] **Step 5: Commit** `feat(hris): notifikasi harian ringkasan absen terlewat 09.00 WIB (EPIC-067)`.

### Task 4: Kartu ringkasan di HRIS → Absensi

**Files:**
- Modify: `frontend/src/features/hris/attendance/{api.ts,queries.ts,query-keys.ts,types.ts,index.ts}`
- Create: `frontend/src/features/hris/attendance/components/attendance-gap-summary-card.tsx` (+ `.test.tsx`)
- Modify: `frontend/src/features/hris/attendance/components/attendance-page.tsx` (kartu di atas `<Tabs>`)

**Interfaces:**
- Consumes: `GET /api/hris/attendance/gap-summary` (Task 2).
- Produces: `useAttendanceGapSummary(date?: string)`, tipe `AttendanceGapSummary` (snake_case sesuai JSON).

- [ ] **Step 1: Write the failing test** `attendance-gap-summary-card.test.tsx` (mock fetch / query sesuai pola test fitur ini):
  - Menampilkan `Absen terlewat kemarin`, `5 belum absen`, `2 belum absen pulang`, dan `3 karyawan tanpa jadwal (tidak dihitung)`.
  - Tombol `Lihat daftar` membuka daftar nama + NIP + jenis (`Belum absen` / `Belum absen pulang` + jam masuk).
  - `missing + no_clock_out = 0` → `Tidak ada absen terlewat`.
  - `period_closed` → teks `Periode sudah ditutup`.
  - Status error → pesan error, tidak crash.
  - Input tanggal (default kemarin) mengubah query.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement.** Gunakan `Card` dan gaya kartu amber `AttendanceGapsCard` sebagai acuan visual; label hari memakai format tanggal Indonesia yang sudah ada. Baca panduan Next di `node_modules/next/dist/docs/` bila menyentuh file `app/`.
- [ ] **Step 4: Run, expect PASS.** Jalankan juga `npx tsc --noEmit` dan eslint pada file yang disentuh.
- [ ] **Step 5: Commit** `feat(hris): kartu ringkasan absen terlewat di halaman Absensi (EPIC-067)`.

### Task 5: Grant `approve`, `Actor.CanApprove`, endpoint akses, dan skrip cek sebelum/sesudah

**Files:**
- Create: `backend/database/migrations/deltas/20261027090100_iam_hris_approve.sql`
- Create: `backend/database/sql/checks/2026-10-26_epic067_approve_access.sql`
- Modify: `backend/internal/platform/iam/hris.go` (`LeafLembur = "hris.kepegawaian.overtime"`, `HrisApproveLeaves = []string{LeafAbsensi, LeafCuti, LeafLembur}`)
- Modify: `backend/internal/modules/hris/service.go` (`Actor.Approves []string`, `func (a *Actor) CanApprove(leaf string) bool`), `backend/internal/modules/hris/transport.go` (`func (h handlers) actorFor(ctx context.Context, u *auth.User) (*Actor, error)`: `svc.Actor` + isi `Approves` via `h.auth.ResolveRoleIDs` + `GrantedMenuActions` + `iam.HasGrantedAction` per daun; `actor(r)` memakainya; semua `h.svc.Actor(` di transport HRIS diganti `h.actorFor(`)
- Create: rute `GET /api/hris/me/approve-access` (R14) + `goNativePrefixes` + generate
- Modify: `backend/database/seeders/iam-menus.sql`, `backend/database/seeders/iam-role-permissions.sql`, `backend/database/seeders/paddy-hris-admins.test.js` (R12)
- Test: `backend/internal/modules/hris/approve_access_integration_test.go`

**Interfaces:**
- Produces: `iam.LeafLembur`, `iam.HrisApproveLeaves`, `Actor.CanApprove(leaf string) bool`, JSON `{attendance, leaves, overtime}`, skrip cek R13 yang menghasilkan kolom `user_id, email, role, before_attendance, after_attendance, before_trip, after_trip, before_leave, after_leave, before_overtime, after_overtime` dan dua query akhir `lost` / `gained`.

- [ ] **Step 1: Write the failing test** `TestApproveAccess`:
  - (a) Setelah migrasi, `permission_context` attendance dan leaves memuat `approve`. Role `super_admin`, `admin`, `hrd`, dan `admin_hris` yang punya baris di daun memuat `approve`. Setelah `iam.sync_hris_staff_roles()`, `hris_manager`, `hris_non_payroll`, dan `hris_non_rekrutmen_kpi` juga memuat `approve` pada daun yang mereka pegang.
  - (b) `/api/hris/me/approve-access` → semua true untuk: user `role='hrd'` tanpa `iam.user_roles`, user `admin_hris`, super_admin, dan admin. Semua false untuk staff `pos` dan `hiring_manager`.
  - (c) Jalankan skrip cek dalam test (baca file SQL, eksekusi query `lost`) pada fixture standar → 0 baris. Fixture tambahan: user `role='hrd'` dengan `iam.user_roles` = role non-HR → muncul di `lost` (bukti skrip mendeteksi; fixture ini dihapus atau di-rollback).
  - (d) Migrasi idempoten: menjalankan isi SQL dua kali tidak menggandakan `approve`.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement** migrasi (R12), terapkan, lalu `-verify` di `paddy_test_e067`. Implement juga konstanta, `Actor`, `actorFor`, endpoint, generate, `goNativePrefixes`, seeder, dan skrip cek. Skrip cek tidak boleh berisi `INSERT/UPDATE/DELETE`, hanya `SELECT`/CTE.
- [ ] **Step 4: Run, expect PASS.** Jalankan juga `cd backend/database && node --test seeders/paddy-hris-admins.test.js` (atau runner yang dipakai file itu) dan `cd frontend && npx vitest run src/lib/backend-routes.test.ts`.
- [ ] **Step 5: Commit** `feat(hris): aksi approve di daun Absensi/Cuti/Lembur + cek akses sebelum/sesudah (EPIC-067)`.

### Task 6: Jalur HR keputusan memakai `approve` (Go)

**Files:**
- Modify: `backend/internal/modules/hris/attendance_requests.go` (`DecideAttendanceRequest`, `ListAttendanceRequests`, akses foto), `backend/internal/modules/hris/selfservice.go` (`NavBadges`: badge pengajuan absensi), `backend/internal/modules/hris/overtime.go` (`DecideOvertime`), `backend/internal/modules/hris/leaves.go` (`DecideLeave`), `backend/internal/modules/hris/business_trips.go` (`ListTrips`, `CreateTrip`, `DecideTrip`), dan pemanggil lain yang ditemukan grep `IsHRD` di jalur keputusan keempat fitur ini (mis. `approvals.go`)
- Test: perluas `attendance_request_decision_integration_test.go`, `overtime_approval_integration_test.go`, `leave_approval_integration_test.go`, `business_trips_integration_test.go`

**Interfaces:**
- Consumes: `Actor.CanApprove`, `iam.LeafAbsensi`, `iam.LeafCuti`, `iam.LeafLembur` (Task 5).

- [ ] **Step 1: Write the failing tests.** Untuk tiap keputusan:
  - (i) User `role='hrd'` yang IAM role-nya tidak punya `approve` di daun (buat role uji dengan menu `read` saja) → 403 dengan pesan HR yang sudah ada. Sebelumnya lulus karena `IsHRD`.
  - (ii) User ber-role aplikasi non-HR (mis. `pos`) dengan IAM role yang punya `approve` di daun → bisa memutus sebagai HR (`decided_as = 'hr'` untuk cuti/lembur).
  - (iii) User HR ber-`approve` memutus pengajuannya sendiri → tetap 403 dengan pesan self-approval yang sudah ada.
  - (iv) Atasan langsung tanpa menu → tetap bisa memutus bawahan (`decided_as = 'manager'`).
  - Untuk pengajuan absensi, tambahkan juga `GET /api/hris/attendance-requests` dan badge `/dashboard/hris/attendance`. Untuk Perjalanan Dinas, tambahkan list semua, catat atas nama karyawan, dan cancel oleh HR.
  - Test yang sudah ada tetap hijau. Ekspektasi lama yang hanya bergantung pada `IsHRD` boleh diubah fixture-nya, tapi assert-nya tidak dilonggarkan.
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement** R11. Untuk Next fallback, cek R15 dan catat hasilnya di laporan.
- [ ] **Step 4: Run, expect PASS.** Jalankan seluruh paket `go test ./internal/modules/hris/... ./internal/modules/payroll/... -count=1` dan tempel tabel grep `IsHRD` (file:line → tetap / diganti, beserta alasannya) di laporan task.
- [ ] **Step 5: Commit** `feat(hris): keputusan HR cuti/lembur/absensi/dinas memakai aksi approve (EPIC-067)`.

### Task 7: Frontend mengikuti akses `approve`

**Files:**
- Modify: `frontend/src/features/hris/attendance/components/attendance-page.tsx` (tab `pengajuan`/`dinas` dari `useHrApproveAccess().attendance`)
- Create: hook `useHrApproveAccess()` di `frontend/src/features/hris/attendance/queries.ts` atau fitur yang paling dekat (+ api, query key, tipe)
- Modify: halaman HR Cuti (komponen tombol keputusan di `/dashboard/hris/leaves`, mis. `components/hris/ApprovalButtons.tsx` pemakainya) dan `frontend/src/features/hris/overtime/components/overtime-table.tsx` (R14)
- Test: test komponen yang sudah ada untuk ketiga halaman diperluas

**Interfaces:**
- Consumes: `GET /api/hris/me/approve-access` (Task 5).
- Produces: `useHrApproveAccess(): { data?: { attendance: boolean; leaves: boolean; overtime: boolean }, … }`.

- [ ] **Step 1: Write the failing tests.**
  - Halaman Absensi: `attendance=false` → tab `Pengajuan Absensi` dan `Perjalanan Dinas` tidak ada walau `user.role='hrd'`; `attendance=true` dengan `role='pos'` → tab ada.
  - Tabel Lembur: `overtime=false` → tombol Setujui/Tolak tidak dirender.
  - Halaman Cuti: `leaves=false` → tombol keputusan tidak dirender.
  - Saat memuat, tombol dan tab tidak berkedip tampil (render setelah data ada).
- [ ] **Step 2: Run, expect FAIL.**
- [ ] **Step 3: Implement.** Bila `canManageTrips` tidak lagi dipakai di mana pun, hapus beserta test-nya.
- [ ] **Step 4: Run, expect PASS.** Jalankan juga `npx tsc --noEmit` dan eslint file yang disentuh.
- [ ] **Step 5: Commit** `feat(hris): UI keputusan HR mengikuti aksi approve (EPIC-067)`.

### Task 8: Verifikasi penuh, smoke, dan dokumen (controller)

Dikerjakan oleh controller, bukan implementer.

- [ ] Gerbang:
  - Go: `go test -p 1 -count=1 ./...` di `paddy_test_e067` → 0 FAIL, plus `gofmt -l` dan `go vet ./...` bersih.
  - Frontend: `npx vitest run` penuh → 0 gagal, `npx tsc --noEmit`, eslint file yang disentuh, lalu `pnpm build` + `pnpm check:safari`.
  - `make -C backend generate` tanpa diff.
  - Migrasi `-apply` dan `-verify`.
- [ ] Smoke API dan Playwright di staging lokal (API Go + `next dev` terhadap `paddy_test_e067`):
  - Kartu ringkasan tampil dengan data fixture.
  - `RunGapDigest` lewat test sudah terbukti; smoke cukup memeriksa notifikasi di lonceng setelah pemanggilan manual dari test helper bila mudah.
  - Export CSV dan XLSX memuat kolom baru.
  - Negatif: akun tanpa menu Absensi → 403 di gap-summary; akun HR tanpa `approve` → 403 saat memutus; keputusan pengajuan absensi di periode closing → 409.
- [ ] Review akhir seluruh branch dengan model terkuat, perbaikan sampai "Ready to merge".
- [ ] Dokumen: Automation Log epic, `status-requirement.md` (AB-05 export, AB-06 ringkasan HR, catatan approve di AB-04/AP-01/Perjalanan Dinas), `matriks-approval.md` (kolom "Kode sekarang": HR = aksi `approve` daun), memori proyek.
