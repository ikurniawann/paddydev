# Ketentuan Cuti & Izin + SLA Pengajuan (EPIC-084) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Terapkan TnC cuti/izin 03-11 (jenis baru, batas hari, Birthday Leave, prorata 2027, potongan payroll berbasis masa kerja) dan SLA pengajuan (H-n, hari H, H+1, kunci setelah cut off).

**Architecture:**
- Aturan per jenis tinggal di katalog Go `domain.LeaveType` sebagai kolom data, dengan cermin TS.
- Fungsi domain murni memutuskan penolakan; `CreateLeave`, `CreateOvertime` dan pengajuan absensi hanya memanggilnya.
- Prorata baru masuk ke fungsi SQL `hris.default_annual_leave` dan kembarannya di Go.
- Payroll mengklasifikasikan tiap hari cuti lewat satu fungsi domain.

**Tech Stack:** Go (hris, payroll, desktop), PostgreSQL delta migration, Next.js/TS (vitest).

**Spec:** `docs/epics/EPIC-084-ketentuan-cuti-izin.md` (R1–R9). Rujuk nomor R di commit/komentar.

## Global Constraints
- Pesan error berbahasa Indonesia persis seperti di spec. Gunakan `httpx.BadRequest`, kecuali kunci periode (R3) yang memakai 409 seperti `closedPeriodError`.
- "Hari ini" = `domain.TodayWIB(s.now())`. Jangan pakai `time.Now()` langsung.
- Kode jenis baru: `birthday`, `miscarriage`, `wife_miscarriage`, `child_marriage`, `child_ceremony`. Nilai enum baru masuk ke file delta tersendiri.
- Delta baru:
  - `backend/database/migrations/deltas/20261107090000_leave_type_tnc_values.sql` (hanya `ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS …`);
  - `20261107090100_leave_tnc_rules.sql` (label notifikasi + `hris.default_annual_leave`).
- Saldo ≤ 2026 tidak boleh berubah. Baris `hris.leave_balances` yang sudah ada tidak disentuh migrasi.
- Pengecualian HR: aturan R2, R5 dan R6 hanya untuk pengajuan milik sendiri. "Milik sendiri" = target karyawan = `a.EmployeeID`.
- Tes integrasi: `TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e084?sslmode=disable'` (buat DB baru dari container `paddydev-sdd-testdb`), `export PATH=$HOME/.local/go/bin:$PATH`, `go test -p 1`.
- Jangan commit `node_modules` symlink, `.swarm`, atau `.claude-flow`. Trailer commit: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus
1. **Cuti melintasi cut off** (mulai 18, selesai 23, diajukan tanggal 22 dengan cutoff 20): ditolak karena tanggal mulai ada di periode yang sudah lewat. Diuji di Task 3.
2. **HR mengajukan untuk karyawan lain** H-1 atau mundur: lolos R2/R5. Diuji di Task 3.
3. **Birthday 29 Feb di tahun bukan kabisat**: ulang tahun dianggap 28 Feb; cuti di bulan Februari lolos. Diuji di Task 2.
4. **Haji melintasi bulan payroll** (mulai 25 Okt, 40 hari, < 1 tahun): hari ke-15 dihitung dari tanggal mulai cuti, bukan dari awal periode. Diuji di Task 5.
5. **Cuti bersama di bulan genap 1 tahun**: tidak mengurangi prorata; hanya bulan sesudahnya. Diuji di Task 4.

---

### Task 1: Katalog jenis cuti + enum + label

**Files:**
- Modify:
  - `backend/internal/modules/hris/domain/leave_types.go`
  - `frontend/src/lib/hris/leave-types.ts`
  - `backend/internal/modules/desktop/domain/board.go:115-130`
- Create:
  - `backend/database/migrations/deltas/20261107090000_leave_type_tnc_values.sql`
  - `backend/database/migrations/deltas/20261107090100_leave_tnc_rules.sql` (bagian label saja; Task 4 menambah fungsi saldo di file yang sama)
- Test:
  - `backend/internal/modules/hris/domain/leave_types_test.go` (buat bila belum ada)
  - `frontend/src/lib/hris/leave-types.test.ts`

**Interfaces:**
- Produces: kolom baru `LeaveType`:
  - `MaxDays int`
  - `MaxSpanDays int`
  - `LeadDays int`
  - `SameDayOnly bool`
  - `AttachmentRequired bool`
  - `Gender string`

  Nilainya persis tabel R1. Label baru: paternity "Cuti Istri Melahirkan" / short "Istri Melahirkan"; bereavement "Cuti Berkabung" / short "Berkabung". Export lama tetap. Jenis baru: Short = "Ulang Tahun", "Keguguran", "Istri Keguguran", "Menikahkan Anak", "Khitanan/Baptis"; Export = Label.
- Urutan katalog: annual, sick, personal, leader, birthday, maternity, miscarriage, paternity, wife_miscarriage, marriage, child_marriage, child_ceremony, pilgrimage, bereavement, lalu tiga jenis riwayat.
- TS mirror: field camelCase `maxDays`, `maxSpanDays`, `leadDays`, `sameDayOnly`, `attachmentRequired`, `gender` (opsional), dan `LeaveTypeCode` ditambah 5 kode.

- [ ] **Step 1: Failing tests.**
  - Go `TestLeaveCatalogTnC`:
    - `LookupLeaveType("birthday")` → Selectable, SingleDay, LeadDays 3, AttachmentRequired;
    - `miscarriage` → MaxSpanDays 45, Gender "female", AttachmentRequired;
    - `pilgrimage` → MaxSpanDays 40, LeadDays 30;
    - `sick` → SameDayOnly;
    - `marriage` → MaxDays 3;
    - `LeaveTypeLabel("paternity") == "Cuti Istri Melahirkan"`;
    - `LeaveTypeExportLabel("paternity") == "Cuti Ayah"`;
    - jumlah Selectable = 14.
  - TS: mirror yang sama, plus `SELECTABLE_LEAVE_CODES` sama urutannya dengan Go (salin daftar kode sebagai fixture).
- [ ] **Step 2: Run, expect FAIL.**
  - `cd backend && go test ./internal/modules/hris/domain/ -run TestLeaveCatalogTnC`
  - `cd frontend && npx vitest run src/lib/hris/leave-types.test.ts`
- [ ] **Step 3: Implement.**
  - Katalog Go + TS.
  - `board.go` label map (5 kode baru + 2 label baru).
  - Delta enum.
  - Delta label: `CREATE OR REPLACE` `fn_notify_leave_changes` dari isi terakhir di `20261020150100_leave_notify_labels.sql`, dengan CASE ditambah 5 jenis dan 2 label baru.
- [ ] **Step 4: Run** kedua tes, `go test ./internal/modules/desktop/...`, dan tes paket hris yang menyebut label (`go test -p 1 ./internal/modules/hris/ -run 'Leave|Notify'`). Expect PASS. Perbarui ekspektasi label lama yang memang berubah.
- [ ] **Step 5: Commit** `feat(hris): jenis cuti TnC 03-11 — Birthday, keguguran, cuti khusus anak (EPIC-084)`.

### Task 2: Aturan pengajuan cuti (domain murni)

**Files:**
- Create: `backend/internal/modules/hris/domain/leave_rules.go`
- Test: `backend/internal/modules/hris/domain/leave_rules_test.go`

**Interfaces:**
- Consumes: kolom katalog Task 1.
- Produces:
  - `func LeaveTimingError(lt LeaveType, start, today string) string`: "" bila lolos; pesan R2 bila tidak. Format tanggal di pesan `02-01-2006`.
  - `func LeaveSizeError(lt LeaveType, start, end string, workDays int) string`: MaxDays → "{Label} maksimal {n} hari kerja"; MaxSpanDays → "{Label} maksimal {n} hari kalender".
  - `func LeaveGenderError(lt LeaveType, gender string) string`: "{Label} hanya untuk karyawan perempuan" / "laki-laki". Gender kosong → "".
  - `type FirstContract struct{ EndDate, ProbationEnd string }` ("" = kosong).
  - `func BirthdayInYear(birthDate string, year int) (string, bool)`: 29 Feb → 28 Feb di tahun bukan kabisat.
  - `func InitialContractPassed(first *FirstContract, employmentStatus, onDate string) bool` (aturan R4).

- [ ] **Step 1: Failing tests** (`today = "2026-11-03"`):
  - Timing:
    - annual mulai 11-06 → ""; mulai 11-05 → "Cuti Tahunan diajukan paling lambat H-3 (mulai 06-11-2026 atau sesudahnya)";
    - pilgrimage mulai 12-02 → ""; mulai 12-01 → pesan H-30 (mulai 03-12-2026 …);
    - sick mulai 11-03 → ""; mulai 11-02 → "Sakit diajukan pada hari sakit (tanggal mulai = hari ini)"; mulai 11-04 → pesan yang sama;
    - bereavement mulai 10-30 → "".
  - Size:
    - marriage workDays 4 → "Cuti Menikah maksimal 3 hari kerja";
    - maternity 2026-11-01..2027-01-29 (90 hari) → ""; ..2027-01-30 → "Cuti Melahirkan maksimal 90 hari kalender";
    - pilgrimage 41 hari → pesan 40.
  - Gender: maternity + "male" → "Cuti Melahirkan hanya untuk karyawan perempuan"; maternity + "" → ""; paternity + "female" → "… laki-laki".
  - Birthday: `BirthdayInYear("1990-02-29", 2027)` → "2027-02-28"; tahun 2028 → "2028-02-29".
  - Kontrak:
    - `{EndDate:"2026-10-31"}` on "2026-11-10" → true; on "2026-10-31" → false;
    - `{ProbationEnd:""}` → true; `{ProbationEnd:"2026-12-01"}` on "2026-11-10" → false;
    - nil + "permanent" → true; nil + "probation" → false.
- [ ] **Step 2: Run** `go test ./internal/modules/hris/domain/ -run 'Leave(Timing|Size|Gender)|Birthday|InitialContract'`. Expect FAIL.
- [ ] **Step 3: Implement** the six functions. Dates via `ParseDate`/`DateLayout`; span = end − start + 1.
- [ ] **Step 4: Run.** Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): aturan batas waktu, ukuran, gender & Birthday Leave (EPIC-084)`.

### Task 3: Sambungkan aturan ke pengajuan (cuti, lembur, absensi)

**Files:**
- Modify:
  - `backend/internal/modules/hris/leaves.go` (`CreateLeave`)
  - `postgres_leaves.go` (`LeaveEmployee` + query kontrak awal)
  - Repository interface (file tempat `LeaveEmployee` dideklarasikan)
  - `overtime.go` (`CreateOvertime`)
  - `attendance_requests.go`
  - `attendance_periods.go` (helper kunci)
- Test: `backend/internal/modules/hris/leave_tnc_integration_test.go`

**Interfaces:**
- Consumes: Task 2 functions; katalog Task 1.
- Produces:
  - `func (s *Service) ensureSubmissionOpen(ctx context.Context, date string) error` (R3). Panggil `ensureAttendanceOpen`, lalu tolak bila `today > p.End` dengan 409 "Periode {p.Label()} sudah lewat cut off; pengajuan untuk tanggal ini ditutup".
  - `LeaveEmployee` mengembalikan juga `gender`, `birth_date::text`, `join_date::text`, `employment_status`.
  - `FirstContract(ctx, employeeID) (*domain.FirstContract, error)`: kontrak `status <> 'draft'` dengan urutan `start_date, sequence` paling awal; nil bila tidak ada.

- [ ] **Step 1: Failing integration test** `TestLeaveTnCSubmission`. Pakai harness hris dengan `s.now` dipatok 2026-11-03 10:00 WIB, sebagai karyawan sendiri. Kasus:
  - (a) annual mulai 11-05 → 400 pesan H-3;
  - (b) sick mulai 11-02 → 400;
  - (c) cuti mulai 10-31 (periode Okt lewat) → 409 cut off;
  - (d) **cutoff 20**: set `attendance_settings` cutoff 20, now 2026-11-22; cuti mulai 11-18 s.d. 11-23 → 409 (Review Focus 1);
  - (e) birthday tanpa attachment → 400 "Birthday Leave (Cuti Ulang Tahun) wajib melampirkan dokumen";
  - (f) birthday di luar bulan lahir → 400 "Birthday Leave hanya dalam bulan ulang tahun Anda";
  - (g) birthday kedua di tahun sama → 400 "Birthday Leave sudah dipakai tahun {yyyy}";
  - (h) birthday saat kontrak awal belum lewat → 400 "Birthday Leave berlaku setelah masa kontrak awal selesai";
  - (i) birth_date kosong → pesan R4;
  - (j) personal saat saldo tahunan 5 → 400 pesan R5 (sisa 5); saldo 0 → 201;
  - (k) marriage 4 hari kerja → 400;
  - (l) HR untuk karyawan lain: annual mulai besok dan personal saat saldo 5 → 201 (Review Focus 2);
  - (m) lembur sendiri tanggal 11-01 diajukan 11-03 → 400 "Lembur diajukan paling lambat H+1 (02-11-2026)"; tanggal 11-02 → 201;
  - (n) pengajuan absensi sendiri untuk 10-31 → 409 cut off.
- [ ] **Step 2: Run** `go test -p 1 ./internal/modules/hris/ -run TestLeaveTnCSubmission`. Expect FAIL.
- [ ] **Step 3: Implement.**
  - Di `CreateLeave`, setelah cek SingleDay, urutannya: gender → attachment wajib ("{Label} wajib melampirkan dokumen") → bila self: timing (R2) + `ensureSubmissionOpen(start)` → birthday (R4) → setelah `totalDays`: `LeaveSizeError` → personal (R5, self saja; pakai `EnsureLeaveBalance` + `AnnualAvailable` tahun mulai; `!found` dianggap 0).
  - Kuota birthday tahunan dicek di dalam transaksi dengan `MonthlyLeaveUse` pada batas tahun.
  - `CreateOvertime` self: H+1 sebelum `ensureAttendanceOpen`, lalu ganti panggilan menjadi `ensureSubmissionOpen`.
  - Pengajuan absensi self: `ensureSubmissionOpen`.
- [ ] **Step 4: Run** the test, then the whole hris package (`go test -p 1 ./internal/modules/hris/`). Fix existing tests whose fixtures now violate R2/R3. Geser tanggal fixture; jangan longgarkan aturan.
- [ ] **Step 5: Commit** `feat(hris): SLA pengajuan cuti/lembur/absensi dan kunci cut off (EPIC-084)`.

### Task 4: Prorata cuti tahunan mulai 2027

**Files:**
- Modify:
  - `backend/internal/modules/hris/domain/workforce.go` (`DefaultAnnualLeave`)
  - `backend/database/migrations/deltas/20261107090100_leave_tnc_rules.sql` (tambah `CREATE OR REPLACE FUNCTION hris.default_annual_leave(date, int)` STABLE)
- Test:
  - `domain/workforce_test.go`
  - `leave_default_integration_test.go`

**Interfaces:**
- Produces: `func DefaultAnnualLeave(joinDate *string, year int, cutiBersama []string) float64` (R7). `cutiBersama` = tanggal YYYY-MM-DD yang memenuhi filter R7; fungsi sendiri yang memilih tanggal di tahun Y dan bulan > A.bulan. Signature SQL tidak berubah.

- [ ] **Step 1: Failing tests.**
  - Unit (CB = cuti bersama):
    - join 2026-05-10: Y2027 tanpa CB → 7; CB {2027-05-20, 2027-06-15} → 6 (Review Focus 5);
    - join 2026-01-15: Y2027 → 12;
    - join 2026-12-01: Y2027 → 0;
    - join 2025-03-01: Y2027 → 12;
    - join 2026-11-10: Y2027 dengan CB 2027-12-24, 12-25, 12-26 → 0 (lantai 0);
    - join 2026-05-10: Y2026 → 0;
    - join 2025-05-10: Y2026 → 7.5 (rumus lama);
    - nil → 12.
  - Paritas `TestDefaultAnnualLeaveSQLMatchesGo`: sisipkan cuti bersama 2027 di tx tes, lalu bandingkan untuk join tiap tanggal 1 dan 15 di 2025–2026 × tahun 2026–2028. Go menerima daftar CB yang sama.
- [ ] **Step 2: Run** `go test ./internal/modules/hris/domain/ -run TestDefaultAnnualLeave` dan `go test -p 1 ./internal/modules/hris/ -run TestDefaultAnnualLeaveSQLMatchesGo`. Expect FAIL.
- [ ] **Step 3: Implement** Go + SQL. Untuk tahun < 2027, cabang lama disalin persis.
- [ ] **Step 4: Run** both + `go test -p 1 ./internal/modules/hris/ -run 'Balance|Leave'`. Expect PASS. Lalu cek checksum: `md5(string_agg(row::text,'' order by id))` atas `hris.leave_balances` sebelum dan sesudah apply delta di DB uji yang berisi baris → sama.
- [ ] **Step 5: Commit** `feat(hris): prorata cuti tahunan per bulan efektif dikurangi cuti bersama mulai 2027 (EPIC-084)`.

### Task 5: Payroll — potongan berbasis masa kerja

**Files:**
- Modify:
  - `backend/internal/modules/payroll/domain/inputs.go:262-300`
  - `calculator.go:425-438`
  - `period.go` (helper)
- Test: `backend/internal/modules/payroll/domain/leave_pay_test.go`; perbarui tes PIS lama yang mengharapkan maternity dipotong.

**Interfaces:**
- Produces:
  - `func ServiceYearReached(joinDate, onDate string) bool`: join + 1 tahun ≤ onDate; tak terbaca → true.
  - `func LeaveDayKind(leaveType, leaveStart, day, joinDate string) string` (`"personal"`|`"sick"`|`""`, R8). Haji: hari ke-15 = leaveStart + 14 hari kalender.
- Loop cuti di `inputs.go` memakai `LeaveDayKind(l.LeaveType, *s, d, emp.JoinDate)` untuk menaikkan `personal`/`sick`.
- `MaternityLeaveDays` tetap diisi (hari maternity + miscarriage, informasi) tetapi PIS = `SickLeaveDays` saja.

- [ ] **Step 1: Failing tests.**
  - `LeaveDayKind`:
    - maternity start 2026-11-02, join 2025-11-02 → "";
    - join 2025-11-03 → "personal";
    - miscarriage sama;
    - pilgrimage start 2026-10-25, join 2026-06-01: day 2026-11-07 → "", day 2026-11-08 → "personal" (Review Focus 4);
    - join 2025-01-01 → "";
    - leader → "";
    - birthday → "";
    - sick → "sick".
  - `Calculate`: input dengan MaternityLeaveDays 5, SickLeaveDays 0, transport/makan flat → potongan sakit 0.
- [ ] **Step 2: Run** `go test ./internal/modules/payroll/domain/`. Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/modules/payroll/...` (domain + integration `-p 1`). Expect PASS.
- [ ] **Step 5: Commit** `feat(payroll): melahirkan/keguguran full paid ≥1 th, haji >14 hari unpaid <1 th (EPIC-084)`.

### Task 6: Frontend — form ESS & HR

**Files:**
- Modify:
  - `frontend/src/features/hris/ess/components/cuti-request-dialog.tsx`
  - `frontend/src/features/hris/leaves/components/new-leave-page.tsx`
  - `frontend/src/lib/hris/leave-form.ts`
- Test: `frontend/src/lib/hris/leave-form.test.ts`

**Interfaces:**
- Consumes: TS catalog fields (Task 1).
- Produces:
  - `export function leaveRuleHint(info: LeaveTypeInfo): string | null` di `leave-form.ts`. Contoh: annual → "Diajukan paling lambat H-3"; pilgrimage → "Diajukan paling lambat H-30 · maksimal 40 hari kalender"; sick → "Diajukan pada hari sakit"; leader → "1x per bulan, hangus bila tidak dipakai, tanpa potong gaji"; birthday → "1 hari dalam bulan ulang tahun · wajib lampiran (mis. foto KTP)"; marriage → "Maksimal 3 hari kerja".
  - `export function earliestStartDate(info: LeaveTypeInfo, today: string): string | null` (today + leadDays; sick → today).
  - Validasi form: attachment wajib bila `attachmentRequired`, dengan pesan "Lampiran wajib untuk {label}".

- [ ] **Step 1: Failing vitest:** hint per jenis di atas, `earliestStartDate(annual, "2026-11-03") === "2026-11-06"`, birthday tanpa attachment → error.
- [ ] **Step 2: Run** `npx vitest run src/lib/hris/leave-form.test.ts`. Expect FAIL.
- [ ] **Step 3: Implement.**
  - Kedua form menampilkan hint di bawah pilihan jenis dan memberi `min` pada input tanggal mulai (ESS saja; form HR tanpa `min`).
  - Label attachment "(wajib)" bila wajib.
  - Pesan error server tetap ditampilkan apa adanya.
- [ ] **Step 4: Run** `npx vitest run src/lib/hris`, `npx tsc --noEmit`, `npm run lint -- --quiet` (yang dipakai CI). Expect PASS.
- [ ] **Step 5: Commit** `feat(hris): form cuti menampilkan aturan TnC dan lampiran wajib (EPIC-084)`.

### Task 7: Staging, QA, dokumen, PR

- [ ] Backup DB staging ke `~/Desktop/paddydev-backups/`. Deploy branch ke staging (`umask 022`, `tools/deploy-paddydev.sh`). Pastikan migrate apply + verify 0 berbeda.
- [ ] Bandingkan checksum `hris.leave_balances` sebelum dan sesudah deploy → sama.
- [ ] Aktifkan cutoff 20 di staging saja (R9) lewat UI Pengaturan Absensi; catat rentang periode Nov/Des.
- [ ] QA browser di staging (data demo, bersihkan sesudahnya tanpa `session_replication_role`):
  - form ESS menampilkan 14 jenis + hint;
  - annual H-1 ditolak;
  - sakit hari ini lolos;
  - birthday (isi birth_date akun uji di bulan ini) wajib lampiran lalu lolos;
  - lembur H+2 ditolak;
  - payroll draft: karyawan join < 1 th dengan cuti melahirkan dipotong PIP.
- [ ] Docs:
  - Automation Log epic;
  - `docs/hris/acuan-mom-20261019/status-requirement.md` (ES-12..ES-16 + TnC);
  - `pertanyaan-rekomendasi.md` (K-19, K-20, K-21, K-27 dengan jawaban 03-11);
  - `docs/epics/README.md` baris EPIC-084;
  - ubah status epic.
- [ ] PR ke `main` via `gh` (body diakhiri "🤖 Generated with [Claude Code](https://claude.com/claude-code)"). Merge dan production hanya dengan izin user. Aktivasi cutoff 20 di production butuh keputusan HR soal periode transisi (R9).
