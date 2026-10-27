# Alokasi Saldo Cuti oleh HR — Rencana Implementasi (EPIC-068)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR mengatur saldo Cuti Tahunan (manual, import massal, sisa bawaan otomatis 1 Januari) lewat aksi menu, dengan validasi, audit, dan ESS yang memakai saldo baru.

**Architecture:**
- **Data:** `hris.leave_balances` mendapat kolom penyesuaian dan sisa bawaan. Sisa jatah = jatah + penyesuaian − terpakai (kolom generated). Sisa bawaan berlaku sampai tanggal kedaluwarsa dan dihitung di query.
- **Jatah default** dihitung satu fungsi SQL `hris.default_annual_leave(join_date, year)`, dipakai trigger, backfill, dan Go (paritas dites).
- **Endpoint** Go baru di modul `hris`, dijaga `withAction` pada daun `hris.workforce.leave-balances`.
- **Import** meniru `schedule_days.go`: satu endpoint `step=preview|commit|error_file`, fingerprint, satu transaksi.
- **Carry-over:** goroutine loop seperti `GapDigestLoop` dengan klaim per tahun.
- **Frontend:** memperbaiki tab Leave Balance, menambah halaman Saldo Cuti dan dialog import, serta memperbarui ESS.

**Tech Stack:** Go (pgx, `platform/xlsx` excelize), PostgreSQL 16, Next.js 16.4 + React Query + Vitest.

**Spec:** `docs/epics/EPIC-068-alokasi-saldo-cuti.md` (keputusan user 26-10-2026) + acuan `.claude/skills/paddy-hris-acuan` (Cuti Tahunan 12/tahun setelah 12 bulan kerja).

## Global Constraints
- Kelipatan 0,5 untuk jatah, penyesuaian, sisa bawaan. Tahun yang boleh diubah hanya tahun berjalan dan tahun depan (WIB).
- Sisa jatah (jatah + penyesuaian − terpakai) ≥ 0, dan sisa bawaan − bawaan terpakai ≥ 0.
- Tidak mengubah "terpakai" lewat endpoint HR. Terpakai hanya berubah lewat persetujuan atau pembatalan cuti.
- Audit setiap perubahan saldo memakai `audit.Write` dengan `Entity = "hris.employees"`, `EntityID = employee_id`, before/after, dan `Reason`. Action: `leave_balance.update` | `leave_balance.import` | `leave_balance.carry_over` | `leave_settings.update`.
- Pesan galat berbahasa Indonesia, mengikuti gaya `httpx.BadRequest` yang sudah ada.
- Go: `gofmt`, `go vet`, route manifest diperbarui (`frontend/src/lib/go-routes.generated.json`; CI "Route manifest is current"). Frontend: baca `frontend/AGENTS.md`.
- Test Go memakai DB test `TEST_DATABASE_URL=postgres://postgres@127.0.0.1:55499/paddy_test?sslmode=disable`, `go test -p 1`.

## Ruling
- **R1 Jatah default** `default_annual_leave(join, Y)`, dengan A = join + 12 bulan:
  - A > 31-12-Y → 0;
  - A ≤ 01-01-Y → 12;
  - selain itu: `floor(12 × (hari A..31-12-Y inklusif) / hari dalam tahun Y × 2) / 2`;
  - join NULL → 12.
  - Contoh: join 2025-03-10 → 2026: 297/365×12 = 9,76 → **9,5**; join 2026-02-01 → 2026: 0; join 2024-12-31 → 2026: 12.
- **R2 Baris lama tidak dihitung ulang.** Hanya baris baru (trigger karyawan baru, backfill karyawan aktif tanpa saldo tahun berjalan, pembuatan lazy, job 1 Januari) yang memakai R1.
- **R3 Kolom baru:**
  - `annual_leave_adjustment numeric(5,2) DEFAULT 0` (boleh negatif);
  - `carry_over_days numeric(5,2) DEFAULT 0`, `carry_over_used numeric(5,2) DEFAULT 0`, `carry_over_expires_on date NULL`;
  - `annual_leave_remaining` diganti menjadi `annual_leave_total + annual_leave_adjustment - annual_leave_used` (generated);
  - CHECK: kelipatan 0,5 untuk total/adjustment/carry_over_days; carry_over_used ≤ carry_over_days; remaining ≥ 0.
- **R4 Sisa tersedia** pada tanggal D = `annual_leave_remaining + (carry_over_days − carry_over_used, bila carry_over_expires_on ≥ D)`. Tanpa tanggal kedaluwarsa, sisa bawaan tidak berlaku.
- **R5 Persetujuan cuti tahunan** (tahun = tahun `start_date`):
  - pastikan baris ada (R1);
  - ambil bawaan dulu: `min(total_days, sisa bawaan berlaku pada start_date)`, sisanya ke jatah;
  - simpan porsi bawaan di `hris.leaves.carry_over_days_used` (kolom baru, default 0).
  
  **Pembatalan** cuti tahunan yang disetujui mengembalikan porsi bawaan ke `carry_over_used` dan sisanya ke `annual_leave_used`.
- **R6 Pengajuan (`CreateLeave`) cuti tahunan:** pastikan baris ada (R1), lalu tolak bila `total_days > sisa tersedia pada start_date` ("Sisa cuti tahunan tidak cukup", data `{remaining, requested}`). Pending tidak dikurangkan (perilaku lama).
- **R7 Hak akses:** menu daun baru `hris.workforce.leave-balances` "Saldo Cuti" (`/dashboard/hris/leave-balances`, parent `hris.workforce`, actions `["read","update","import","export"]`).
  - **Grant:** super_admin & admin semua aksi; admin_hris & hris_manager semua aksi.
  - **Masker** `iam.hris_staff_role_masks` untuk hris_non_payroll & hris_non_rekrutmen_kpi: `excluded_actions {update,import,export}` (tetap read). Lalu `iam.sync_hris_staff_roles()`.
  - Role `hrd` tidak lagi bisa mengubah lewat cek role.
  - **GET saldo satu karyawan:** pemilik, `read` di daun baru, atau prefix menu Karyawan.
- **R8 Setting** `hris.leave_settings` (satu baris, pola `hris.attendance_settings`):
  - `carry_over_enabled bool DEFAULT false`;
  - `carry_over_max_days numeric(5,2) NULL` (NULL = semua sisa; kelipatan 0,5; ≥ 0);
  - `carry_over_expiry_month smallint DEFAULT 3`, `carry_over_expiry_day smallint DEFAULT 31` (tanggal valid, 29-02 ditolak);
  - `updated_by`, `updated_by_name`, `updated_at`.
  
  `GET/PUT /api/hris/leave-settings` (PUT butuh `update`, audit `leave_settings.update`).
- **R9 Job sisa bawaan:** `LeaveCarryOverLoop` (tiap 15 menit, tidak jalan di `AppEnv == "test"`) memanggil `RunLeaveCarryOver(ctx, now)`. Untuk tahun Y = tahun WIB `now`, job berjalan bila sudah ≥ 01-01-Y 00:00 WIB dan klaim `INSERT INTO hris.leave_carry_over_runs(year) ON CONFLICT DO NOTHING` berhasil. Dalam satu transaksi:
  - untuk setiap karyawan aktif, pastikan baris Y (R1);
  - bila setting aktif dan baris Y-1 ada, carry = `min(annual_leave_remaining(Y-1), max)` dibulatkan ke bawah ke 0,5; hanya bila `carry > 0`, dan baris Y masih `carry_over_days = 0 AND carry_over_expires_on IS NULL` (tidak menimpa isian HR);
  - set carry_over_days, carry_over_used = 0, expires_on = tanggal setting di tahun Y;
  - audit `leave_balance.carry_over` per karyawan yang berubah;
  - simpan ringkasan di `leave_carry_over_runs` (`employees`, `carried`, `finished_at`).
- **R10 Import:**
  - `GET /api/hris/leave-balances/template?year=&branch_id=` (kosong = semua cabang; butuh `export`);
  - `POST /api/hris/leave-balances/import` multipart (`file`, `year`, `branch_id`, `step`, `fingerprint`, `ack_warnings`; butuh `import`);
  - **kolom template:** NIP (kunci, read-only), Nama (read-only), Cabang (read-only), Terpakai (read-only), Jatah, Penyesuaian, Sisa Bawaan, Kedaluwarsa (tanggal), Alasan;
  - baris tanpa perubahan diabaikan; baris berubah wajib Alasan;
  - membuat baris yang belum ada;
  - peringatan (perlu ack): jatah > 24, sisa bawaan tanpa kedaluwarsa (bawaan dianggap tidak berlaku), kedaluwarsa di masa lalu;
  - galat: NIP tidak dikenal atau nonaktif, karyawan di luar filter cabang, bukan kelipatan 0,5, sisa < 0, sisa bawaan < bawaan terpakai, alasan kosong, tahun tidak boleh, file bukan template (info sheet Tahun/Cabang tidak cocok), duplikat NIP;
  - fingerprint = md5 baris saldo karyawan dalam cakupan;
  - commit: satu transaksi + audit `leave_balance.import` per karyawan.
- **R11 Manual:** `PUT /api/hris/leave-balances/{employee_id}`, body `{year, annual_leave_total, annual_leave_adjustment, carry_over_days, carry_over_expires_on, reason}`.
  - Butuh `update`. Validasi sama dengan R10.
  - Response `{data: <saldo>, message}`.
  - `annual_leave_used`/`sick_leave_used` tidak lagi diterima (400 bila dikirim).
- **R12 Bentuk respons saldo** (`LeaveBalanceView`) dipakai GET satu karyawan, daftar, ESS, dan PUT:
  `{employee_id, nip, full_name, branch_name, year, annual_leave_total, annual_leave_adjustment, annual_leave_used, annual_leave_remaining, carry_over_days, carry_over_used, carry_over_remaining, carry_over_expires_on, carry_over_active, available, updated_at}`
  
  `available` = R4 pada hari ini (WIB). `GET /api/hris/leave-balances?year=&branch_id=&q=` (butuh `read`) mengembalikan daftar karyawan aktif dalam cakupan (baris yang belum ada ditampilkan dengan jatah default dan `exists: false`, tanpa ditulis).
- **R13 ESS:**
  - `essBasics` memakai `LeaveBalanceView` (dan membuat baris tahun berjalan bila belum ada, R1);
  - Beranda/Cuti menampilkan `available` + rincian jatah/penyesuaian/terpakai/sisa bawaan (+ kedaluwarsa);
  - fallback "12" dihapus;
  - form pengajuan menampilkan sisa tersedia.
- **R14 Migrasi & backfill:** buat baris tahun berjalan (2026) untuk karyawan aktif tanpa baris memakai R1. Trigger `initialize_employee_leave_balance` diganti agar memakai R1 untuk tahun bergabung **dan** tahun berjalan.

## Review Focus
1. **Cuti tahunan disetujui saat sisa bawaan sudah kedaluwarsa** → harus diambil dari jatah, bukan dari bawaan. Test di Task 2.
2. **Pembatalan cuti lama (sebelum EPIC ini, `carry_over_days_used = 0`)** → seluruhnya kembali ke jatah. Test di Task 2.
3. **Import dua kali dengan file yang sama** → commit kedua tidak mengubah apa pun dan tidak membuat audit (baris tanpa perubahan diabaikan). Test di Task 3.
4. **Job 1 Januari berjalan dua kali atau di dua replika** → klaim tahun mencegah duplikasi. Test di Task 4.
5. **HR menurunkan jatah di bawah terpakai** → 400 dengan pesan sisa < 0, bukan galat DB. Test di Task 2.

---

### Task 1: Migrasi, fungsi jatah default, menu & hak akses
**Files:**
- Create: `backend/database/migrations/deltas/20261028090000_leave_balance_allocation.sql`
- Modify: `backend/database/seeders/iam-menus.sql` (menu baru ikut seeder dan daftar `code NOT IN`)
- Modify: `backend/internal/platform/iam/hris.go` (`LeafSaldoCuti = "hris.workforce.leave-balances"`); regenerasi `prefixes_gen.go` bila ada generator
- Modify: `backend/internal/modules/hris/domain/workforce.go` (`DefaultAnnualLeave(joinDate *string, year int) float64`, menggantikan `ProratedAnnualQuota`)
- Test: `backend/internal/modules/hris/domain/workforce_test.go` (tabel R1), `backend/internal/modules/hris/leave_default_integration_test.go` (paritas Go vs SQL untuk ±20 tanggal termasuk kabisat, 31-12, 01-01, join NULL)
- Create: `backend/database/sql/checks/2026-10-28_epic068_leave_balance_access.sql` (cek akses read-only: lost/gained, pola EPIC-067)

**Interfaces:** Produces `hris.default_annual_leave(date, int) RETURNS numeric` (IMMUTABLE), `domain.DefaultAnnualLeave`, `iam.LeafSaldoCuti`, kolom R3, `hris.leaves.carry_over_days_used`, tabel `hris.leave_settings`, `hris.leave_carry_over_runs(year int PK, started_at, finished_at, employees int, carried int)`.

- [ ] Tulis test domain R1 (gagal), lalu implementasi Go.
- [ ] Tulis migrasi:
  - fungsi SQL R1;
  - `ALTER TABLE`: tambah kolom; `DROP CONSTRAINT annual_leave_non_negative`; `DROP COLUMN annual_leave_remaining`; `ADD COLUMN annual_leave_remaining ... GENERATED ALWAYS AS (annual_leave_total + annual_leave_adjustment - annual_leave_used) STORED`; CHECK R3;
  - `hris.leaves.carry_over_days_used`; dua tabel baru;
  - menu + grant + masker + `sync_hris_staff_roles()` (pola `20261027090100_iam_hris_approve.sql`);
  - trigger baru; backfill R14;
  - migrasi aditif dan idempoten (`IF NOT EXISTS`).
- [ ] Terapkan ke DB test (`migrate -apply`), jalankan test paritas, skrip cek akses (0 lost), `go vet`, `gofmt`.
- [ ] Commit `feat(hris): EPIC-068 migrasi saldo cuti, jatah default, menu Saldo Cuti`.

### Task 2: Inti saldo — view, lazy create, PUT manual, persetujuan/pembatalan
**Files:**
- Create: `backend/internal/modules/hris/leave_balances.go` (service + handler: view, list, get, put, ensure)
- Modify: `postgres_leaves.go`, `leaves.go` (CreateLeave R6, DecideLeave/UpdateLeave R5, hapus cek `hrd`), `http_leaves.go`, `routes.go`, `selfservice.go` (R13)
- Test: `backend/internal/modules/hris/leave_balances_integration_test.go`; perbarui `workforce_integration_test.go` (PUT 403 kini berbasis aksi)

**Interfaces:**
- `type LeaveBalanceView struct` (R12)
- `func (s *Service) EnsureLeaveBalance(ctx, q, employeeID string, year int) error`
- `func (s *Service) LeaveBalanceView(ctx, a *Actor, employeeID string, year int) (LeaveBalanceView, error)`
- `func (s *Service) ListLeaveBalances(ctx, year int, branchID, q string) ([]LeaveBalanceView, error)`
- `func (s *Service) UpdateLeaveBalanceManual(ctx, a *Actor, employeeID string, in LeaveBalanceInput) (LeaveBalanceView, error)`
- `type LeaveBalanceInput struct{Year int; Total, Adjustment, CarryDays float64; CarryExpires *string; Reason string}`
- `func validateLeaveBalance(in LeaveBalanceInput, used, carryUsed float64, active bool, nowYear int) error`; Task 3 memakai ulang fungsi ini.
- Routes:
  - `GET /api/hris/leave-balances` (`withAction("read", saldoCuti)`);
  - `GET /api/hris/leave-balances/{employee_id}` (pemilik / read / Karyawan);
  - `PUT /api/hris/leave-balances/{employee_id}` (`withAction("update", saldoCuti)`);
  - `GET /api/hris/me/leave-balance-access` (`{read, update, import, export}` untuk UI).

- [ ] **Test dulu:**
  - PUT tanpa aksi 403; tanpa alasan 400; 0,25 400; jatah < terpakai 400 (Review 5); karyawan nonaktif 400; tahun 2025 400; sukses + audit before/after/alasan;
  - persetujuan memakai bawaan dulu; bawaan kedaluwarsa memakai jatah (Review 1);
  - pembatalan mengembalikan per sumber; cuti lama kembali ke jatah (Review 2);
  - pengajuan tanpa baris dicek (R6);
  - GET pemilik/HR/orang lain;
  - ESS Me memuat `available`.
- [ ] Implementasi sampai hijau. Regenerasi route manifest. Commit.

### Task 3: Import & template Excel
**Files:**
- Create: `backend/internal/modules/hris/leave_balance_import.go`, `domain/leave_balance_upload.go` (validasi baris murni)
- Modify: `routes.go`
- Test: `domain/leave_balance_upload_test.go`, `leave_balance_import_integration_test.go`

**Interfaces:**
- `GET /api/hris/leave-balances/template` (`export`)
- `POST /api/hris/leave-balances/import` (`import`), `step=preview|commit|error_file`
- Preview: `{summary{employees,new,changed,same}, errors[], warnings[], fingerprint, changes[≤500]{nip,name,before,after}}`
- `domain.ValidateLeaveBalanceUpload(rows, ctx) (changes, errors, warnings)` memakai `validateLeaveBalance` (Task 2)

- [ ] **Test dulu:**
  - template per cabang/semua (kolom R10, info sheet);
  - setiap galat/peringatan R10;
  - commit atomik (satu baris galat → tidak ada yang tersimpan);
  - fingerprint 409;
  - file error memuat "Keterangan Error";
  - import ulang tidak mengubah apa pun (Review 3);
  - membuat baris yang belum ada.
- [ ] Implementasi (pola `schedule_days.go` + `import_errors.go`). Manifest. Commit.

### Task 4: Setting sisa bawaan + job 1 Januari
**Files:**
- Create: `backend/internal/modules/hris/leave_carry_over.go`, `leave_settings.go`
- Modify: `module.go` (jalankan loop bila bukan test), `routes.go`
- Test: `leave_carry_over_integration_test.go`, `leave_settings_integration_test.go`

**Interfaces:**
- `func (s *Service) RunLeaveCarryOver(ctx context.Context, now time.Time) error`
- `func (s *Service) LeaveCarryOverLoop(ctx context.Context, every time.Duration)`
- `GET/PUT /api/hris/leave-settings`

- [ ] **Test dulu:**
  - setting nonaktif → hanya baris baru;
  - aktif + maks → carry = min;
  - maks NULL → semua sisa;
  - tidak menimpa isian HR;
  - dua kali run → sekali (Review 4);
  - sebelum 1 Januari WIB (31-12 23:59 WIB) tidak jalan;
  - audit per karyawan;
  - setting PUT validasi (tanggal 29-02 ditolak, maks kelipatan 0,5) + audit;
  - tanpa aksi 403.
- [ ] Implementasi. Commit.

### Task 5: Frontend HR — tab Leave Balance + halaman Saldo Cuti + setting
**Files:**
- Modify: `frontend/src/features/users/components/detail/leave-balance-tab.tsx`, `features/users/{api,queries,types}.ts`
- Create: `frontend/src/features/hris/leave-balances/{api.ts,queries.ts,mutations.ts,types.ts}`, `components/{leave-balances-page.tsx,leave-balance-edit-dialog.tsx,leave-settings-card.tsx}`, route `frontend/src/app/dashboard/(dashboard)/hris/leave-balances/page.tsx` (`requireIamPage` daun baru)
- Modify: `features/hris/onboarding/components/onboarding-page.tsx:224` (tautan mati → tab karyawan)
- Test: vitest untuk tab, dialog (pratinjau sisa, alasan wajib, kelipatan 0,5, galat server), halaman (filter cabang/tahun), setting card

- [ ] Test dulu, implementasi, `tsc`, `eslint`, `vitest` area terkait. Commit.

### Task 6: Frontend import dialog
**Files:**
- Create: `frontend/src/features/hris/leave-balances/components/leave-balance-upload-dialog.tsx` (+ test), API di `api.ts` (`leaveBalanceTemplateUrl`, `importLeaveBalanceFile`, `downloadLeaveBalanceErrors`)

- [ ] Test dulu (pratinjau → ack peringatan → simpan; 409 menampilkan pesan dan meminta pratinjau ulang; unduh file error). Implementasi meniru `schedule-upload-dialog.tsx`. Commit.

### Task 7: ESS, dokumen, cek akhir
**Files:**
- Modify: `frontend/src/features/hris/ess/components/{cuti-page.tsx,beranda-page.tsx,cuti-request-dialog.tsx}`, `features/hris/ess/types.ts`
- Modify: `docs/hris/acuan-mom-20261019/status-requirement.md` (ES-03 Cuti Tahunan 12 bulan/jatah default ditegakkan), `.claude/skills/paddy-hris-acuan/SKILL.md` bila perlu, epic Automation Log

- [ ] Test dulu (tampil `available` + rincian bawaan + kedaluwarsa, tanpa "12" palsu). Implementasi.
- [ ] Jalankan seluruh test Go (`go test -p 1 ./...`), `vitest run`, `tsc`, `eslint`, `next build`, `pnpm check:safari`. Commit.
