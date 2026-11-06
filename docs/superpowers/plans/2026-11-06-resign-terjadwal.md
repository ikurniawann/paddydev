# Resign Terjadwal (EPIC-095) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR bisa mencatat resign dengan tanggal efektif di masa depan. Resign disimpan sebagai jadwal dan dijalankan otomatis sehari setelah hari terakhir kerja.

**Architecture:**
- **Pemisahan logika resign.** Transaksi resign yang ada dipecah jadi `applyResign`, yang dipakai oleh resign langsung maupun job.
- **Jadwal.** POST resign dengan tanggal > hari ini menyimpan baris di `hris.scheduled_resignations`.
- **Job.** `ScheduledResignLoop` (tiap 5 menit, pola `AutoCheckoutLoop`) menjalankan jadwal yang tanggalnya sudah lewat.
- **Rute baru.** `GET`/`DELETE /api/hris/employees/{id}/resign` untuk membaca dan membatalkan jadwal.

**Tech Stack:** Go (pgx, `backend/internal/modules/hris`), PostgreSQL delta, Next.js + React Query + Vitest (`frontend/src/features/users`).

**Spec:** `docs/epics/EPIC-095-resign-terjadwal.md`. Ruling R1–R10 mengikat.

## Global Constraints

- Worktree `.worktrees/resign`, branch `fix/resign-terjadwal`, base `paddydev/main` (`45305bdd`).
- Migrasi: `backend/database/migrations/deltas/20261113090000_scheduled_resignations.sql`, sesudah `20261112090100` milik EPIC-094.
- Tes Go:
  - `cd backend && PATH=$HOME/.local/go/bin:$PATH TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e084?sslmode=disable' go test -p 1 ./internal/modules/hris/ -count=1 -run '<pola>'`.
  - Migrasi baru diterapkan ke DB uji lewat `docker exec -i paddydev-sdd-testdb psql -U postgres -d paddy_test_e084 -1 -v ON_ERROR_STOP=1 < <file>`, karena ledger DB uji belum mencatat 088–092.
- Tes frontend: `cd frontend && pnpm vitest run <path>`. Gerbang: `pnpm test`, `pnpm lint`, `pnpm exec tsc --noEmit`. Untuk `node_modules`, symlink ke `.worktrees/epic087/frontend/node_modules`, dan jangan di-commit.
- Teks persis:
  - `Tanggal efektif paling jauh 180 hari dari hari ini`
  - `Sudah ada resign terjadwal efektif <dd-mm-yyyy> — batalkan jadwal itu dulu`
  - `Resign dijadwalkan efektif <dd-mm-yyyy>`
  - `Jadwal resign dibatalkan`
  - `Tidak ada resign terjadwal`
  - Aktor audit job: `Sistem (resign terjadwal)`
  - Alasan batal otomatis: `Karyawan sudah nonaktif`
  - Teks UI (R8):
    - banner: `Resign terjadwal: <jenis> efektif <tgl> (hari terakhir kerja)`;
    - tombol: `Batalkan jadwal`, `Jadwalkan resign`;
    - catatan dialog terjadwal: `Karyawan tetap aktif sampai <tgl>. Akun dinonaktifkan dan login diblokir otomatis pada <tgl+1> 00.00 WIB.`
- Commit diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Production read-only. Merge dan deploy production hanya dengan izin user. Backup sebelum deploy staging.

## Review Focus

1. **Tanggal efektif = hari ini.** Tetap resign langsung (bukan jadwal), sama dengan perilaku lama. Tes di Task 1.
2. **Job berjalan dua kali.** Dua replika atau dua tick tidak boleh menjalankan satu jadwal dua kali. Jadwal di-claim dengan `FOR UPDATE SKIP LOCKED` dan `applied_at` diisi di transaksi yang sama. Tes di Task 2 menjalankan Run dua kali.
3. **Karyawan diaktifkan kembali (rehire) atau dinonaktifkan lewat jalur lain sebelum tanggal.** Jadwal dibatalkan otomatis saat job berjalan (R7), dan tidak menonaktifkan karyawan aktif kembali secara keliru. Tes di Task 2.
4. **Batas 180 hari dan tanggal sebelum tanggal masuk** ditolak sebelum apa pun tersimpan. Tes di Task 1.
5. **Pembatalan setelah jadwal dijalankan** → 404 `Tidak ada resign terjadwal`. Tes di Task 1.

---

### Task 1: Jadwal resign di API (Go: migrasi, simpan, baca, batal)

**Files:**
- Create: `backend/database/migrations/deltas/20261113090000_scheduled_resignations.sql`
- Modify: `backend/internal/modules/hris/employee_resign.go`
- Modify: `backend/internal/modules/hris/routes.go:232`, di peta `"resign"` tambah `GET` (`withMenu(kepegawaian, …)`) dan `DELETE` (`withAction("update", employeeRecordManagers, …)`).
- Test: `backend/internal/modules/hris/employee_resign_schedule_integration_test.go`

**Interfaces:**
- Produces:
  - **Tabel `hris.scheduled_resignations`:**
    - `id uuid pk default gen_random_uuid()`;
    - `employee_id uuid not null references hris.employees on delete cascade`;
    - `effective_date date not null`;
    - `resign_type text not null check in ('resign','terminated','end_of_contract')`;
    - `reason text`;
    - `created_by uuid references configuration.users on delete set null`;
    - `created_at timestamptz default now()`;
    - `applied_at timestamptz`;
    - `cancelled_at timestamptz`, `cancelled_by uuid`, `cancel_reason text`.
    - Indeks unik parsial `(employee_id) WHERE applied_at IS NULL AND cancelled_at IS NULL`.
  - `func (s *Service) applyResign(ctx context.Context, q database.Querier, actor *auth.User, recordedBy *string, id string, in resignInput) (map[string]any, error)`.
    - Isinya badan transaksi lama, dimulai dari `SELECT … FOR UPDATE`.
    - `recordedBy` adalah `configuration.users.id` untuk `employment_history.recorded_by` (lewat lookup yang sama). `nil` → pakai `actor.ID`.
  - `ResignEmployee` menghasilkan `(map[string]any, scheduled bool, err error)`.
    - Tanggal ≤ hari ini → `applyResign`.
    - Tanggal > hari ini → simpan jadwal, hasil `{"id","scheduled":true,"effective_date","resign_type"}`.
    - Keduanya menolak bila ada jadwal aktif (R5).
  - `GET /api/hris/employees/{id}/resign` → `{data: {id, effective_date, resign_type, reason, created_by_name, created_at} | null}`.
  - `DELETE /api/hris/employees/{id}/resign` dengan body opsional `{reason}`:
    - ada jadwal aktif → 200 dengan `Jadwal resign dibatalkan`;
    - tidak ada → 404 `Tidak ada resign terjadwal`.

- [ ] **Step 1: Tes yang gagal** (`setNow("2026-11-06 10:00")`, pola fixture `TestResignEmployee`)
  - `TestResignScheduledFuture`:
    1. POST `effective_date: 2026-11-20` → 200, pesan `Resign dijadwalkan efektif 20-11-2026`, `data.scheduled = true`.
    2. Karyawan masih `is_active = true`; login tidak diblokir (`auth.users.banned_until` NULL).
    3. Penempatan masih terbuka.
    4. Ada 1 baris jadwal aktif dan audit `employee.resign_schedule` = 1.
    5. GET mengembalikan `effective_date 2026-11-20`.
  - `TestResignScheduledLimits`:
    - `2027-05-06` (> 180 hari) → 400 batas;
    - sebelum `join_date` → 400 (pesan lama);
    - tanggal hari ini `2026-11-06` → resign langsung (`is_active = false`), tanpa baris jadwal.
  - `TestResignScheduledOnlyOne`:
    - sesudah jadwal 20 Nov, POST lagi (20 Nov maupun hari ini) → 400 `Sudah ada resign terjadwal efektif 20-11-2026 — batalkan jadwal itu dulu`.
  - `TestResignScheduleCancel`:
    1. DELETE dengan `{reason: "Batal resign"}` → 200; `cancelled_at` terisi; audit `employee.resign_schedule_cancel` = 1.
    2. GET → `data` null.
    3. DELETE kedua → 404 `Tidak ada resign terjadwal`.
    4. Staf tanpa aksi update → 403.
- [ ] **Step 2: Jalankan** pola `-run 'ResignSchedul|TestResignEmployee'` sesudah migrasi diterapkan ke DB uji. Expected: tes baru FAIL (400 masa depan / 405), `TestResignEmployee` lama PASS.
- [ ] **Step 3: Implementasi** sesuai Interfaces.
  - Handler POST memilih pesan `Karyawan diresign` atau `Resign dijadwalkan efektif …` dari nilai `scheduled`.
  - Tanggal 180 hari dihitung `domain.AddDaysISO(today, 180)`.
  - Format tanggal pesan memakai `dd-mm-yyyy`.
- [ ] **Step 4: Jalankan ulang** pola yang sama ditambah `-run 'Rehire|EmployeeAudit|GoLive'`. Expected: PASS.
- [ ] **Step 5: Commit**: `feat(hris): resign dengan tanggal efektif mendatang disimpan sebagai jadwal (EPIC-095)`.

### Task 2: Job menjalankan resign terjadwal (Go)

**Files:**
- Create: `backend/internal/modules/hris/scheduled_resign.go`
- Modify: `backend/internal/modules/hris/module.go:86-91`, tambah `go svc.ScheduledResignLoop(context.Background(), ScheduledResignEvery)`.
- Test: `backend/internal/modules/hris/scheduled_resign_integration_test.go`

**Interfaces:**
- Consumes: `applyResign` (Task 1).
- Produces:
  - `const ScheduledResignEvery = 5 * time.Minute`.
  - `func (s *Service) RunScheduledResigns(ctx context.Context, now time.Time) (applied, cancelled int, err error)`.
    - Mengambil jadwal aktif dengan `effective_date < TodayWIB(now)`.
    - Tiap jadwal diproses dalam transaksi sendiri dengan claim `FOR UPDATE SKIP LOCKED`.
    - Karyawan nonaktif → `cancelled_at`, `cancel_reason = 'Karyawan sudah nonaktif'`.
    - Karyawan aktif → `applyResign` dengan actor `&auth.User{FullName: "Sistem (resign terjadwal)"}` dan `recordedBy = created_by`, lalu `applied_at = now()`.
    - Galat satu jadwal di-log, lalu lanjut ke jadwal berikutnya.
  - `func (s *Service) ScheduledResignLoop(ctx context.Context, every time.Duration)` dengan tick, recover, dan timeout 1 menit, persis pola `AutoCheckoutLoop`.

- [ ] **Step 1: Tes yang gagal**
  - `TestRunScheduledResigns`:
    1. Karyawan berakun dengan penempatan dan `employee_shift_days` 19, 20, dan 21 Nov. Jadwal 20 Nov dibuat lewat API pada 2026-11-06.
    2. `RunScheduledResigns(2026-11-20 23:00 WIB)` → applied 0, karyawan masih aktif.
    3. `RunScheduledResigns(2026-11-21 00:05 WIB)` → applied 1:
       - `is_active = false`, `end_date = 2026-11-20`;
       - shift day 21 Nov terhapus, 19 dan 20 Nov tetap;
       - penempatan `end_date = 2026-11-20`;
       - `banned_until` terisi;
       - `employment_history.recorded_by` = karyawan HR pembuat;
       - audit `employee.resign` dengan `actor_name = 'Sistem (resign terjadwal)'`;
       - `applied_at` terisi.
    4. Run kedua → applied 0 (tidak dijalankan ulang).
  - `TestRunScheduledResignsInactive`: karyawan dengan jadwal lalu dinonaktifkan langsung (`UPDATE is_active = false`); Run sesudah tanggal → cancelled 1, `cancel_reason = 'Karyawan sudah nonaktif'`, tanpa audit `employee.resign` baru.
- [ ] **Step 2: Jalankan** `-run 'RunScheduledResigns'`. Expected: FAIL (fungsi tidak ada).
- [ ] **Step 3: Implementasi** sesuai Interfaces, lalu pasang loop di `module.go` (hanya bila `deps.DB != nil && AppEnv != "test"`, seperti loop lain).
- [ ] **Step 4: Jalankan ulang** `-run 'RunScheduledResigns|ResignSchedul|TestResignEmployee'`. Expected: PASS.
- [ ] **Step 5: Commit**: `feat(hris): job menjalankan resign terjadwal sehari setelah hari terakhir kerja (EPIC-095)`.

### Task 3: Dialog Resign dan banner profil (frontend)

**Files:**
- Modify: `frontend/src/features/users/components/resign-dialog.tsx` (prop `maxDate`, catatan sesuai tanggal, label tombol).
- Modify: `frontend/src/features/users/api.ts`, tambah `fetchScheduledResign(id)` dan `cancelScheduledResign(id, reason)`; `resignEmployee` mengembalikan `data.scheduled`.
- Create: `frontend/src/features/users/components/detail/scheduled-resign-banner.tsx` (+ tes).
- Modify: `frontend/src/features/users/components/user-detail-page.tsx`. Banner tampil di bawah header. `onResign` hanya diberikan bila tidak ada jadwal. Sesudah resign atau batal, refetch jadwal dan karyawan.
- Test: `resign-dialog.test.tsx` (ubah tes "menolak tanggal efektif di masa depan") dan `scheduled-resign-banner.test.tsx`.

**Interfaces:**
- Consumes: rute GET/DELETE Task 1.
- Produces:
  - `ScheduledResign {id; effective_date; resign_type; reason: string|null; created_by_name: string|null; created_at}`.
  - `ScheduledResignBanner({ employeeId, canCancel, onChanged })`.

- [ ] **Step 1: Tes yang gagal**
  - **Dialog** (today `2026-11-06`):
    - tanggal `2026-11-20` → catatan biru `Karyawan tetap aktif sampai 20-11-2026. Akun dinonaktifkan dan login diblokir otomatis pada 21-11-2026 00.00 WIB.`; tombol `Jadwalkan resign` aktif; submit mengirim `effective_date: "2026-11-20"`.
    - tanggal `2027-05-06` → pesan `Tanggal efektif paling jauh 180 hari dari hari ini`, tombol nonaktif.
    - tanggal hari ini → teks peringatan merah lama dan tombol `Resign`.
  - **Banner:**
    - jadwal ada → `Resign terjadwal: Resign (mengundurkan diri) efektif 20-11-2026 (hari terakhir kerja)`;
    - `Batalkan jadwal` memanggil `cancelScheduledResign("e1", …)` lalu `onChanged`;
    - `canCancel = false` → tombol tidak ada;
    - jadwal null → tidak merender apa pun.
- [ ] **Step 2: Jalankan** `pnpm vitest run src/features/users`. Expected: FAIL.
- [ ] **Step 3: Implementasi.**
  - Label jenis diambil dari `TYPES` yang sudah ada di dialog; ekspor sebagai `RESIGN_TYPES`.
  - Konfirmasi batal memakai `ConfirmDialog` dari `@/components/ui/confirm-dialog`, dengan alasan opsional.
- [ ] **Step 4: Jalankan ulang**, lalu `pnpm exec tsc --noEmit && pnpm lint`. Expected: PASS dan bersih.
- [ ] **Step 5: Commit**: `feat(hris): dialog resign terjadwal dan banner jadwal di profil karyawan (EPIC-095)`.

### Task 4: Gerbang, review, PR, staging, QA

- [ ] **Step 1: Gerbang penuh.** `go vet` + `gofmt`; `go test -p 1 ./internal/modules/hris/... -count=1`; `make -C backend generate` (manifest rute ikut berubah, lalu di-commit); `pnpm test`, `lint`, `tsc`. Bila rute baru belum punya fallback Next, daftarkan sebagai rute khusus Go di `frontend/src/lib/backend-routes.test.ts`.
- [ ] **Step 2: Review akhir seluruh branch** oleh reviewer segar. Satu gelombang perbaikan untuk temuan Critical/Important.
- [ ] **Step 3: Docs + PR.** Isi Automation Log dan README EPIC-095, lalu buka PR ke `main`.
- [ ] **Step 4: Deploy staging.** Backup `staging-db-before-epic095-<stamp>.dump`, merge ke HEAD `deploy-e071`, lalu `tools/deploy-paddydev.sh`. Expected: 1 migrasi dan verify 0 berbeda.
- [ ] **Step 5: QA staging** (akun demo `qa095-*`, dibersihkan sesudahnya):
  1. Jadwalkan resign 20 Nov dari profil → banner tampil, karyawan tetap bisa login.
  2. Batalkan → banner hilang.
  3. Jadwalkan lagi dengan tanggal kemarin lewat SQL demo, lalu jalankan job (tunggu ≤ 5 menit sesudah deploy) → karyawan nonaktif dan login ditolak.
  4. Resign hari ini → langsung nonaktif seperti sebelumnya.
