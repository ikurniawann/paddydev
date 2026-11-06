# Keterlambatan Tanpa Potongan + Peringatan Lonceng + Surat HR (EPIC-094) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Karyawan tidak lagi melihat label Teguran/SP. Setiap keterlambatan memicu lonceng peringatan ke karyawan, penyetuju, dan HR. HR menerbitkan Surat Teguran/SP secara manual dengan lonceng ke pihak yang sama. Payroll tidak memotong keterlambatan.

**Architecture:**
- Peringatan dikirim sesudah `ClockIn` berhasil menyisipkan baris terlambat, lewat repo baru `NotifyRequestApprovers` (`notify_user` untuk `request_approver` ∪ `request_manager`), `NotifyEmployee`, dan `NotifyHR` yang sudah ada.
- Surat disimpan di tabel baru `hris.disciplinary_letters`. API Go ada di file baru `disciplinary_letters.go`, dengan menu IAM daun di bawah Kehadiran & Cuti.
- Frontend:
  - fitur baru `features/hris/disciplinary-letters` (halaman HR, dialog terbitkan, daftar);
  - tab profil karyawan;
  - bagian ESS Absensi.
- Potongan telat dimatikan lewat migrasi data. Halaman slip HR menyembunyikan baris nol.

**Tech Stack:** Go (pgx, `backend/internal/modules/hris`, `payroll`), PostgreSQL deltas, Next.js + React Query + Vitest.

**Spec:** `docs/epics/EPIC-094-disiplin-keterlambatan.md`. Ruling R1–R15 mengikat. Latar belakang ada di EPIC-077 (keterlambatan ESS) dan EPIC-091/092 (penyetuju).

## Global Constraints

- Worktree `.worktrees/epic094`, branch `feat/epic094-disiplin-keterlambatan`, base `feat/epic092-approval-berjenjang` (`e09fee9e`).
- Migrasi:
  - `backend/database/migrations/deltas/20261112090000_disciplinary_letters.sql`;
  - `backend/database/migrations/deltas/20261112090100_late_deduction_off.sql`.
  - Keduanya sesudah `20261111090000` EPIC-093.
- Tes Go:
  - `cd backend && PATH=$HOME/.local/go/bin:$PATH TEST_DATABASE_URL='postgres://postgres@127.0.0.1:55499/paddy_test_e084?sslmode=disable' go test -p 1 ./internal/modules/<mod>/ -count=1 -run '<pola>'`.
  - Migrasi baru diterapkan ke DB uji lewat `MIGRATE_DATABASE_URL=<sama> go run ./cmd/migrate -apply`.
  - `TestGapDigestRun` kadang flaky saat seluruh paket berjalan.
- Tes frontend: `cd frontend && pnpm vitest run <path>`. Gerbang akhir: `pnpm test`, `pnpm lint`, `pnpm exec tsc --noEmit`.
- Untuk `node_modules`, symlink ke `.worktrees/epic087/frontend/node_modules` boleh dipakai, tetapi jangan di-commit. Jangan commit `.swarm` atau `.claude-flow`.
- Teks persis:
  - peringatan, untuk karyawan:
    - judul `Anda terlambat hari ini`;
    - pesan `Anda terlambat {m} menit hari ini ({n} kali di periode ini). Hati-hati, jika berlanjut akan ada teguran dari HR.`;
  - peringatan, untuk penyetuju/HR: judul `{Nama} terlambat hari ini`, pesan `{Nama} terlambat {m} menit hari ini ({n} kali di periode ini).`;
  - surat, untuk karyawan: judul `{Label} dari HR`, pesan `HR menerbitkan {Label} untuk Anda tanggal {dd-mm-yyyy}.`;
  - surat, untuk penyetuju/HR: judul `{Label} untuk {Nama}`, pesan `HR menerbitkan {Label} untuk {Nama} tanggal {dd-mm-yyyy}.`;
  - label jenis: `teguran` = `Surat Teguran`, `sp1` = `Surat Peringatan 1`, `sp2` = `Surat Peringatan 2`, `sp3` = `Surat Peringatan 3`;
  - pengingat ESS (R2): `Hati-hati, keterlambatan yang berlanjut dapat berujung teguran dari HR.`
- Tautan lonceng:
  - peringatan: karyawan `/dashboard/me/absensi#keterlambatan`, penyetuju `/dashboard/me/absensi-tim`, HR `/dashboard/hris/attendance`;
  - surat: karyawan `/dashboard/me/absensi#surat`, penyetuju `/dashboard/me/absensi-tim`, HR `/dashboard/hris/surat-peringatan`.
- Jenis notifikasi selalu `alert` (CHECK `public.notifications.type`).
- Commit diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Production read-only, tanpa sesi uji. Backup sebelum setiap deploy staging. Merge dan deploy production hanya dengan izin user.

## Review Focus

1. **Penyetuju ganda.** Bila penyetuju = Manager (SPV kosong, atau atasan langsung adalah Manager), hanya satu lonceng yang masuk. Karyawan yang dirinya sendiri penyetuju tertinggi tidak mendapat lonceng penyetuju. Tes ada di Task 1.
2. **Lonceng gagal.** Clock-in tetap 200 meski notifikasi gagal. Tes ada di Task 1, lewat repo yang mengembalikan galat atau dengan memeriksa bahwa galat hanya di-log.
3. **Surat dibatalkan.** Surat yang dibatalkan tidak muncul di `GET /api/hris/me/disciplinary-letters`, dan lampirannya tidak bisa dibuka karyawan lain. Tes ada di Task 4.
4. **Hak akses.** Karyawan biasa tidak bisa menerbitkan atau membaca surat orang lain (403). HR tanpa aksi `create` tidak bisa menerbitkan. Tes ada di Task 4.
5. **Label SP di ESS.** Kartu ESS karyawan tidak memuat teks `Teguran`, `SP`, atau `Surat Peringatan` dari tingkat keterlambatan, sedangkan kartu tim masih memuatnya. Tes ada di Task 2.

---

### Task 1: Peringatan lonceng saat clock-in terlambat (Go)

**Files:**
- Create: `backend/internal/modules/hris/domain/late_notice.go` + `late_notice_test.go`.
- Create: `backend/internal/modules/hris/late_notice.go` (service + store method).
- Modify: `backend/internal/modules/hris/attendance_clock.go:174` (sesudah `InsertAttendance` sukses).
- Modify: interface repo di `ports.go` dan store di `attendance_requests.go`, mengikuti `NotifyEmployee`/`NotifyHR`.
- Test: `backend/internal/modules/hris/late_notice_integration_test.go`.

**Interfaces:**
- Produces:
  - `func domain.LateNotice(name string, minutes, count int) (selfTitle, selfMsg, teamTitle, teamMsg string)`, dengan teks persis sesuai Global Constraints.
  - Repo `NotifyRequestApprovers(ctx context.Context, employeeID, kind, title, message, link string) (int, error)`. Ia menyisipkan satu notifikasi per user unik dari `hris.request_approver(e)` dan `hris.request_manager(e)`, kecuali `e.user_id` sendiri, lalu mengembalikan jumlahnya. Task 4 juga memakainya.
  - `func (s *Service) notifyLate(ctx context.Context, row *Row)`. Tidak mengembalikan apa pun; galat di-log `slog.Warn`.

- [ ] **Step 1: Tes yang gagal**
  - Unit `TestLateNotice`: `LateNotice("Ana", 25, 3)` → keempat string persis seperti Global Constraints.
  - Integrasi (ikuti fixture clock-in yang ada, mis. tes `ClockIn` dengan jadwal shift dan jam di-mock):
    - `TestClockInLateNotifiesEmployeeApproversHR`:
      - Rantai: karyawan → SPV → Manager. Ada akun `role = hrd`.
      - Clock-in 25 menit lewat toleransi menghasilkan satu baris `public.notifications` per penerima:
        - karyawan: judul `Anda terlambat hari ini`, link `/dashboard/me/absensi#keterlambatan`, type `alert`;
        - SPV dan Manager: link `/dashboard/me/absensi-tim`;
        - HR: link `/dashboard/hris/attendance`.
      - Pesan memuat `(1 kali di periode ini)`.
    - `TestClockInOnTimeNoNotice`: clock-in tepat waktu → 0 notifikasi baru.
    - `TestLateNoticeApproverDedup`: atasan langsung karyawan adalah Manager (`request_approver` = `request_manager`) → tepat 1 notifikasi untuk Manager.
- [ ] **Step 2: Jalankan** pola `-run 'LateNotice|ClockInLate|ClockInOnTime'`. Expected: FAIL (`LateNotice` belum ada atau 0 notifikasi).
- [ ] **Step 3: Implementasi**
  - **Pemicu.** `ClockIn` memanggil `s.notifyLate(ctx, row)` bila `row.Bool("is_late")`.
  - **Hitungan.** `notifyLate` menghitung jumlah terlambat di periode tanggal baris itu (`periodContaining` + `lateDays` dari `lateness.go`). Nama dan menit diambil dari baris dan karyawan.
  - **Pengiriman.** Tiga notifikasi dikirim. Galat masing-masing di-log dan tidak dikembalikan (R4).
- [ ] **Step 4: Jalankan ulang** pola yang sama, lalu `-run 'ClockIn|Attendance|Lateness'`. Expected: PASS.
- [ ] **Step 5: Commit**: `feat(hris): lonceng peringatan terlambat ke karyawan, penyetuju, dan HR (EPIC-094)`.

### Task 2: Label tingkat hilang dari ESS karyawan (frontend)

**Files:**
- Modify: `frontend/src/features/hris/lateness/components/my-lateness-card.tsx:76-102`, `lateness-beranda-card.tsx:30`, `lateness-summary.tsx`.
- Modify: `frontend/src/lib/hris/lateness-view.ts`. Tambah `LATENESS_REMINDER` = teks R2.
- Test: `my-lateness-card.test.tsx`, `lateness-beranda-card.test.tsx`, `team-lateness-card.test.tsx` (regresi).

**Interfaces:**
- Produces: `LatenessSummary` mendapat prop `showLevel?: boolean` (default `true`). Bila `false`, badge dan petunjuk tingkat berikutnya tidak dirender.

- [ ] **Step 1: Tes yang gagal**
  - `MyLatenessCard` dengan data `count: 3, level: sp1`:
    - teks `3` dan total menit tetap ada, dan daftar hari tetap ada;
    - `queryByText(/Surat Peringatan|SP ?1|Teguran/)` → null;
    - teks `LATENESS_REMINDER` tampil;
    - tangga dan bagian "Ketentuan Keterlambatan" tidak ada.
  - `LatenessBerandaCard` dengan data yang sama: tanpa badge atau petunjuk, jumlah tetap ada.
  - Tes `TeamLatenessCard` yang ada tetap hijau. Tambahkan assertion bahwa badge `SP 1` masih tampil untuk anggota `sp1`.
- [ ] **Step 2: Jalankan** `pnpm vitest run src/features/hris/lateness`. Expected: FAIL (`Surat Peringatan 1` masih ada).
- [ ] **Step 3: Implementasi**
  - `MyLatenessCard`: `LatenessSummary showLevel={false}`, tanpa `LatenessLadder` dan tanpa `LatenessPolicy`. Disclaimer diganti `LATENESS_REMINDER`.
  - Beranda: `LatenessSummary compact showLevel={false}`.
  - Komponen tim tidak disentuh.
- [ ] **Step 4: Jalankan ulang** `pnpm vitest run src/features/hris/lateness src/features/hris/ess`. Expected: PASS.
- [ ] **Step 5: Commit**: `feat(hris): label Teguran/SP tidak tampil di ESS karyawan (EPIC-094)`.

### Task 3: Payroll tanpa potongan keterlambatan

**Files:**
- Create: `backend/database/migrations/deltas/20261112090100_late_deduction_off.sql`.
- Modify: `frontend/src/features/hris/payroll/components/payslip-page.tsx:214,318-336`.
- Test: `frontend/src/features/hris/payroll/components/payslip-page.test.tsx` (buat bila belum ada, dengan pola mock tes payroll lain).
- Test Go: `backend/internal/modules/payroll/late_off_integration_test.go`.

- [ ] **Step 1: Tes yang gagal**
  - Go `TestLateDeductionOffAfterMigration`:
    - `SELECT late_deduction_mode FROM hris.payroll_settings` = `off`.
    - Hitung satu slip karyawan dengan 7 hari terlambat (pola fixture kalkulasi run yang ada) → `late_deduction = 0`.
    - Potongan Izin Pribadi untuk 1 hari izin pribadi yang disetujui tetap > 0.
  - Frontend `"baris Potongan Keterlambatan tidak tampil bila nol"`: detail `late_deduction: 0` → teks `Potongan Keterlambatan` tidak ada. Bila 15000, teks tetap tampil (data lama).
- [ ] **Step 2: Jalankan** keduanya. Expected: FAIL (mode `tier`; baris "-" tampil).
- [ ] **Step 3: Implementasi**
  - **Migrasi.** `UPDATE hris.payroll_settings SET late_deduction_mode = 'off', updated_at = now() WHERE late_deduction_mode <> 'off';` dengan komentar EPIC-094 R7.
  - **Halaman slip.** Baris telat dirender hanya bila `late_deduction > 0`. Baris potongan lain tidak berubah.
  - Terapkan migrasi ke DB uji.
- [ ] **Step 4: Jalankan ulang** keduanya, lalu `go test -p 1 ./internal/modules/payroll/... -count=1` dan `pnpm vitest run src/features/hris/payroll`. Expected: PASS.
- [ ] **Step 5: Commit**: `feat(payroll): potongan keterlambatan dimatikan, baris telat nol disembunyikan (EPIC-094)`.

### Task 4: Surat Teguran/SP oleh HR (Go: migrasi, menu, API, lonceng)

**Files:**
- Create: `backend/database/migrations/deltas/20261112090000_disciplinary_letters.sql`.
- Create: `backend/internal/modules/hris/disciplinary_letters.go`, `domain/disciplinary.go` (+ `_test.go`), dan `disciplinary_letters_integration_test.go`.
- Modify: `backend/internal/modules/hris/routes.go` (rute baru).
- Modify: `frontend/src/lib/iam/prefixes.ts` (`hrisDisciplinaryLetters: ["hris.workforce.disciplinary-letters"]`), lalu jalankan `make -C backend generate`.

**Interfaces:**
- Consumes: `NotifyRequestApprovers`, `NotifyEmployee`, dan `NotifyHR` (Task 1); `lateDays`/`periodContaining`/`domain.LatenessLevelFor` (EPIC-077).
- Produces:
  - **Tabel `hris.disciplinary_letters`:**
    - `id uuid pk default gen_random_uuid()`;
    - `employee_id uuid not null references hris.employees on delete cascade`;
    - `letter_type text not null check in ('teguran','sp1','sp2','sp3')`;
    - `issued_date date not null`, `notes text`, `attachment_path text`;
    - `created_by uuid`, `created_at timestamptz default now()`;
    - `revoked_at timestamptz`, `revoked_by uuid`, `revoke_reason text`;
    - indeks `(employee_id, issued_date desc)`.
  - **Menu.** `hris.workforce.disciplinary-letters` "Surat Peringatan", route `/dashboard/hris/surat-peringatan`, icon `clipboard`, order 15, aksi `["read","create","update"]`. Grant mengikuti pola `20261028090000_leave_balance_allocation.sql` §5, tanpa masker.
  - **Domain.** `domain.DisciplinaryLabel(code string) string` (label persis Global Constraints, `""` bila tidak dikenal) dan `domain.DisciplinaryCodes = []string{"teguran","sp1","sp2","sp3"}`.
  - **API**, semua dengan objek `{id, employee_id, employee_name, nip, letter_type, label, issued_date, notes, attachment_path, created_by_name, created_at, revoked_at, revoked_by_name, revoke_reason}`:
    - `GET /api/hris/disciplinary-letters?employee_id=&include_revoked=1`: menu `read`, urut `issued_date desc`.
    - `POST /api/hris/disciplinary-letters` `{employee_id, letter_type, issued_date, notes?, attachment_path?}`:
      - menu `create`; karyawan wajib aktif; tanggal tidak boleh di masa depan;
      - audit `disciplinary_letter.issue`; lonceng sesuai Global Constraints, dalam transaksi yang sama;
      - respons 201.
    - `POST /api/hris/disciplinary-letters/{id}/revoke` `{reason}`:
      - menu `update`; alasan wajib (`Alasan pembatalan wajib diisi`);
      - sudah dibatalkan → 400 `Surat sudah dibatalkan`;
      - audit `disciplinary_letter.revoke`; tanpa lonceng.
    - `GET /api/hris/disciplinary-letters/lateness?employee_id=`: menu `read`, menghasilkan `{count, level, label}` untuk periode berjalan (R11).
    - `POST /api/hris/disciplinary-letters/attachments`: menu `create`. Pola `loan_files.go`: `SavePrivateDocument`, folder `disciplinary-letters/<employee_id>`, maks. 5 MB, PDF/gambar.
    - `GET /api/hris/disciplinary-letters/attachment/{path...}`: HR (menu `read`), karyawan pemilik surat yang belum dibatalkan, serta `request_approver`/`request_manager` karyawan itu (R13). Selain itu 403.
    - `GET /api/hris/me/disciplinary-letters`: surat milik sendiri yang belum dibatalkan, urut `issued_date desc`.

- [ ] **Step 1: Tes yang gagal**
  - `TestDisciplinaryLabel`: keempat label persis; `"x"` menghasilkan `""`.
  - `TestDisciplinaryIssueNotifiesAndAudits`:
    1. HR (grant menu penuh) POST sp1 untuk karyawan dengan rantai SPV → Manager → 201.
    2. Notifikasi dengan judul `Surat Peringatan 1 dari HR` ke karyawan (link `#surat`), `Surat Peringatan 1 untuk {Nama}` ke SPV, Manager, dan HR.
    3. Satu baris audit `disciplinary_letter.issue`.
  - `TestDisciplinaryValidation`:
    - jenis `sp4` → 400;
    - tanggal besok → 400;
    - karyawan nonaktif → 400;
    - staf tanpa menu → 403;
    - HR dengan `read` saja → 403 untuk POST.
  - `TestDisciplinaryRevoke`:
    - tanpa alasan → 400;
    - dengan alasan → 200 dan audit `disciplinary_letter.revoke`;
    - batal kedua → 400;
    - `GET /me/disciplinary-letters` milik karyawan tidak lagi memuatnya.
  - `TestDisciplinaryAttachmentAccess`:
    - lampiran dibuka karyawan pemilik, SPV-nya, dan HR → 200;
    - karyawan lain → 403;
    - pemilik setelah surat dibatalkan → 403.
  - `TestDisciplinaryLatenessHint`: karyawan dengan 3 baris terlambat di periode berjalan → `{count:3, level:"sp1", label:"Surat Peringatan 1"}`.
- [ ] **Step 2: Jalankan** pola `-run 'Disciplinary'`, sesudah migrasi diterapkan ke DB uji. Expected: FAIL (rute 404 atau tipe belum ada).
- [ ] **Step 3: Implementasi** sesuai Interfaces.
  - Penjaga menu memakai `h.auth.HasMenuAction(ctx, u, action, prefix)` seperti rute HR lain.
  - Pembacaan baris dan nama memakai pola `delegateRows` di `schedule_delegates.go`.
- [ ] **Step 4: Jalankan ulang** `-run 'Disciplinary|LateNotice'`, lalu `make -C backend generate`. Expected: PASS, dengan diff generate hanya prefix baru dan manifest rute.
- [ ] **Step 5: Commit**: `feat(hris): HR menerbitkan Surat Teguran/SP dengan lonceng ke karyawan, penyetuju, dan HR (EPIC-094)`.

### Task 5: Halaman HR Surat Peringatan + tab profil (frontend)

**Files:**
- Create: `frontend/src/features/hris/disciplinary-letters/{api.ts,queries.ts,mutations.ts,query-keys.ts,types.ts,index.ts}`.
- Create: `components/disciplinary-letters-page.tsx`, `components/issue-letter-dialog.tsx`, `components/letters-table.tsx` (+ tes).
- Create: route `frontend/src/app/dashboard/(dashboard)/hris/surat-peringatan/page.tsx`, dijaga `requireIamPage(IAM.hrisDisciplinaryLetters, "/dashboard/me")`.
- Modify: `frontend/src/features/users/components/user-detail-page.tsx:39-106`, tambah tab `surat` "Surat Peringatan" (hanya `isHR`) yang merender `LettersTable` dengan `employeeId`.

**Interfaces:**
- Consumes: API Task 4.
- Produces:
  - `LettersTable({ employeeId?: string, canIssue: boolean, canRevoke: boolean })`;
  - `IssueLetterDialog({ open, onOpenChange, employeeId?: string })`. Bila `employeeId` diberikan, karyawan terkunci.

- [ ] **Step 1: Tes yang gagal**
  - **Dialog terbitkan:**
    1. Pilih karyawan, lalu teks bantu `Terlambat 3 kali di periode ini (ketentuan: Surat Peringatan 1)` tampil dari endpoint lateness.
    2. Pilih jenis `Surat Peringatan 1`, tanggal, dan catatan, unggah PDF, lalu Simpan → mutasi dikirim dengan `{employee_id, letter_type:"sp1", issued_date, notes, attachment_path}`.
    3. Toast `Surat Peringatan 1 diterbitkan`.
    4. Tanggal masa depan → tombol Simpan nonaktif dengan pesan `Tanggal terbit tidak boleh di masa depan`.
  - **Tabel:**
    - menampilkan label, tanggal, catatan, tautan lampiran, dan penerbit;
    - surat dibatalkan tampil abu-abu dengan badge `Dibatalkan` dan alasannya (`include_revoked=1`);
    - tombol `Batalkan` membuka dialog alasan wajib;
    - tanpa `canRevoke` tombol tidak ada.
  - **Profil:** tab `Surat Peringatan` tampil untuk HR dan tidak tampil tanpa prefix HR.
- [ ] **Step 2: Jalankan** `pnpm vitest run src/features/hris/disciplinary-letters src/features/users`. Expected: FAIL.
- [ ] **Step 3: Implementasi**
  - **Pola kode.** Ikuti `features/hris/schedule-delegates` (api, query, mutation, dialog `DialogPanel`).
  - **Unggah.** Pakai komponen unggah lampiran yang dipakai pinjaman.
  - **Hak aksi.** Diambil dari `useIamAccess().hasAction`; cari helper yang dipakai halaman lain.
- [ ] **Step 4: Jalankan ulang**, lalu `pnpm exec tsc --noEmit && pnpm lint`. Expected: PASS dan bersih.
- [ ] **Step 5: Commit**: `feat(hris): halaman Surat Peringatan HR dan tab profil karyawan (EPIC-094)`.

### Task 6: Surat dari HR di ESS Absensi + SLA regresi

**Files:**
- Create: `frontend/src/features/hris/disciplinary-letters/components/my-letters-section.tsx` (+ tes).
- Modify: `frontend/src/features/hris/ess/components/absensi-page.tsx:54`. Bagian ditempatkan di bawah `MyLatenessCard`, dengan `id="surat"`.
- Test Go: `backend/internal/modules/hris/domain/leave_rules_test.go`, tambah `TestLeaveSLACatalog`.

- [ ] **Step 1: Tes yang gagal**
  - `MyLettersSection`:
    - tanpa surat → tidak merender apa pun;
    - dengan 1 surat `sp1` → judul `Surat dari HR`, `Surat Peringatan 1`, tanggal, catatan, dan tautan `Lihat lampiran` ke `/api/hris/disciplinary-letters/attachment/<path>`.
  - Go `TestLeaveSLACatalog`:
    - `LeaveTimingError(annual, today+2, today)` ≠ "" dan `today+3` = "";
    - `pilgrimage` +29 ≠ "" dan +30 = "";
    - `sick` hari ini = "", besok ≠ "", kemarin ≠ "".
- [ ] **Step 2: Jalankan** kedua tes. Expected: komponen FAIL (belum ada). Tes SLA bisa langsung PASS karena kodenya sudah ada (R14). Catat itu sebagai regresi di ledger, bukan RED.
- [ ] **Step 3: Implementasi** bagian ESS memakai `GET /api/hris/me/disciplinary-letters`.
- [ ] **Step 4: Jalankan ulang** `pnpm vitest run src/features/hris/disciplinary-letters src/features/hris/ess` dan `go test ./internal/modules/hris/domain/ -run SLA`. Expected: PASS.
- [ ] **Step 5: Commit**: `feat(hris): Surat dari HR di ESS Absensi; tes regresi SLA cuti (EPIC-094)`.

### Task 7: Gerbang, review, PR, staging, QA

- [ ] **Step 1: Gerbang penuh**
  - Backend: `go vet` + `gofmt -l`; `go test -p 1 ./internal/modules/hris/... ./internal/modules/payroll/... -count=1`; `make -C backend generate` tanpa diff tersisa.
  - Frontend: `pnpm test`, `pnpm lint`, `pnpm exec tsc --noEmit`.
- [ ] **Step 2: Review akhir** seluruh branch oleh reviewer segar (model paling mampu). Satu gelombang perbaikan untuk temuan Critical/Important.
- [ ] **Step 3: Docs.** Isi Automation Log, status `review`, dan baris README. Perbarui `docs/hris/acuan-mom-20261019/aturan-payroll.md`: baris telat "dimatikan EPIC-094".
- [ ] **Step 4: Push dan PR** dengan base `feat/epic092-approval-berjenjang` (ditumpuk di atas #80). Body diakhiri `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
- [ ] **Step 5: Deploy staging**
  1. Backup `~/Desktop/paddydev-backups/staging-db-before-epic094-<stamp>.dump`.
  2. Merge ke HEAD `.worktrees/deploy-e071`.
  3. `umask 022; bash tools/deploy-paddydev.sh`. Expected: 2 migrasi dan verify 0 berbeda.
- [ ] **Step 6: QA staging** (Playwright, akun demo `qa094-*` dihapus sesudahnya, audit append-only):
  1. Karyawan clock-in terlambat (jadwal shift demo + jam lewat toleransi) → lonceng di karyawan, SPV, Manager, dan HR, dengan teks persis.
  2. ESS Beranda dan Absensi karyawan tanpa label SP, tetapi jumlah tetap. Absensi Tim SPV masih menampilkan badge.
  3. HR menerbitkan SP1 dengan PDF dari menu, dan Surat Teguran dari tab profil → lonceng ke keempat pihak. Karyawan melihat bagian `Surat dari HR` dan bisa membuka lampiran.
  4. HR membatalkan surat → hilang dari ESS.
  5. Pengaturan payroll staging `off`. Hitung ulang run draft demo: potongan telat 0, Izin Pribadi tetap.
  6. SLA lewat UI: Tahunan H-2 ditolak, Haji H-29 ditolak, Sakit besok ditolak.
  7. 0 error konsol dan 0 API ≥ 500.
- [ ] **Step 7: Production (hanya dengan izin user, sesudah tumpukan #75–#80)**
  - Backup VPS dulu. Migrasi `late_deduction_off` mengubah setting live, jadi catat di Automation Log.
  - Sesudah deploy: hitung ulang run draft periode berjalan.
