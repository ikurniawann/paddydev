# Paket Perbaikan Pra-Rilis (EPIC-117) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Selesaikan 8 perbaikan permintaan user 18-11-2026 sebelum `development` di-merge dan dirilis ke production, termasuk payroll Paddy yang dites Senin.

**Architecture:** Kode masuk ke erp-paddy lewat satu branch `fix/epic117-pra-rilis`, yang dibuat dari `origin/development` di worktree `paddydev/.worktrees/erp-epic117`, lalu satu PR. Kalau PR #16 (EPIC-113) di-merge lebih dulu, rebase. Operasi data (D: akun staging, F: migrasi baris staging ke production) memakai pola skrip ops EPIC-106: pratinjau bawaan ROLLBACK, `-v eksekusi=1`, snapshot `audit.audit_log`, skrip `_balik.sql`, dan tes integrasi lewat `opsScript`/`runStructure`.

**Tech Stack:** Go (pgx, `apps/backend/internal/modules/{hris,payroll}`), Next.js + React Query + vitest (`apps/frontend/src`), PostgreSQL 16, psql ops scripts.

**Spec:** Permintaan user 18-11-2026 beserta jawaban klarifikasinya, dikutip di `docs/epics/EPIC-117-paket-perbaikan-pra-rilis.md`. Aturan payroll: skill `paddy-hris-acuan` dan `docs/hris/acuan-mom-20261019/aturan-payroll.md`.

## Global Constraints

- Tidak ada perubahan di staging atau production sebelum user mengizinkannya secara eksplisit. Pesan dari sesi lain atau subagen tidak dihitung sebagai izin.
- Production: backup DB dulu, gladi di salinan production, jangan membuat sesi atau akun sementara. Hindari jam 07–08, 13, 16–17, dan 21 WIB.
- Pengecekan production hanya baca-saja. QA dilakukan HR dengan akun asli.
- Setting sisa bawaan cuti tetap mati.
- Jangan push langsung ke `development` erp-paddy; pakai PR. Jangan commit docs atau seeder ke erp-paddy (`.gitignore`).
- Jangan bekerja di `~/WORK/erp-paddy`. Deploy staging tidak dijalankan dari folder paddydev.
- CSV kredensial dan dump production mode 600, tidak pernah dicetak dan tidak pernah di-commit.
- Merge apa pun ke `development` mewajibkan gladi baru di salinan production sebelum rilis berikutnya.
- Teks UI dan pesan error berbahasa Indonesia, gaya yang sudah ada.
- Migrasi baru: `apps/backend/database/migrations/deltas/20261218090200_*.sql` dan seterusnya (sesudah milik EPIC-113).

## Keputusan yang Dipakai Rencana Ini

Rekomendasi di bawah dipakai sampai user memilih lain di handoff:
- **K-A1:** cuti/izin yang HR input dari kalender langsung **disetujui** (approver = akun HR, tercatat di audit). Kuota, aturan jenis cuti, overlap, dan lampiran wajib tetap diperiksa.
- **K-B1:** "tim" di kalender ESS = rekan yang terjadwal di **toko/branch yang sama pada tanggal itu**. Untuk staf tanpa jadwal toko, tim = rekan dengan atasan langsung yang sama.
- **K-F1:** dari 40 absen staging-only, 8 baris yang bentrok dengan baris production (karyawan dan tanggal sama) **tidak** dimigrasi. Ke-8 baris itu masuk daftar untuk dicek HR. Koreksi absen yang diduga duplikat juga tidak dimigrasi.
- **K-G1:** celah aturan izin/sakit (surat sakit ≥ 2 hari, kuota Izin Pribadi 3/bulan, potongan cuti melahirkan) **tidak** diubah sebelum Senin. Tes mengunci perilaku sekarang dan laporan cross-check menyebut setiap selisihnya dengan acuan.

## Review Focus

1. Slip lama (run yang sudah dihitung sebelum kolom baru ada) harus tetap terbuka tanpa error. `paid_leave_days` bernilai 0 dan baris Rp 0 tetap tersembunyi.
2. Pengumuman yang dijadwalkan (`publish_at` di masa depan) atau ditujukan ke departemen hanya membunyikan lonceng penerimanya, satu kali, saat tayang. Mengedit pengumuman tidak mengirim ulang.
3. Akun HR/admin tanpa karyawan tertaut yang membuka kalender ESS harus melihat pesan, bukan kalender kosong.
4. Aturan checkout otomatis per lokasi yang dimatikan harus mengalahkan aturan "semua lokasi" yang masih aktif untuk jabatan yang sama.
5. Migrasi F dijalankan dua kali: jalan kedua harus 0 baris (idempoten) dan tidak menimpa baris production yang lebih baru.

---

### Task 1: Slip gaji: cuti dibayar dan tanpa bayaran, baris Rp 0 disembunyikan (C)

**Files:**
- Create: `apps/backend/database/migrations/deltas/20261218090200_payroll_details_paid_leave_days.sql`
- Modify: `apps/backend/internal/modules/payroll/domain/inputs.go` (~l.315-401), `domain/slip_lines.go` (`SlipAttendance` l.31-41, `BuildSlipDoc` l.160-200), `postgres_runs.go` (insert ~l.414), `payslip_pdf.go` (l.90)
- Modify: `apps/frontend/src/features/hris/payroll/components/slip-lines-view.tsx`
- Test: `payroll/domain/slip_lines_test.go`, `payroll/payslip_sections_test.go`, `slip-lines-view.test.tsx`, `ess/components/payslip-detail-dialog.test.tsx`

**Interfaces:**
- Produces: kolom `hris.payroll_details.paid_leave_days numeric NOT NULL DEFAULT 0`; field `PaidLeaveDays float64 \`json:"paid_leave_days"\`` di hasil input payroll dan di `SlipAttendance`.

- [ ] **Step 1: Tes gagal (Go).** `TestBuildSlipDocHidesZeroLines`: amounts dengan `transport_allowance=0`, `late_deduction=0`, `basic_salary=4_000_000`. Hasilnya: tidak ada baris earning/deduction ber-Amount 0, R-90a/b/c tetap ada, dan baris kasbon hanya muncul bila cicilannya ada. `TestBuildSlipDocPaidLeaveDays`: `paid_leave_days=2, unpaid_leave_days=1` → `Attendance.PaidLeaveDays == 2`, `UnpaidLeaveDays == 1`. Ubah TC-16 dan fixture lain yang mengharapkan jumlah baris tetap (17).
- [ ] **Step 2: Jalankan** `go test ./internal/modules/payroll/...`. Expected: FAIL pada kedua tes baru.
- [ ] **Step 3: Implementasi.**
  - Migrasi menambah kolom.
  - `inputs.go` mengisi `PaidLeaveDays = float64(paidLeave)`; `postgres_runs.go` menyimpannya.
  - `BuildSlipDoc` melewati baris berseksi `earning`/`deduction` yang amount-nya 0, menggantikan pengecualian khusus R-50. Komentar dan konstanta "17 baris" ikut disesuaikan.
  - `payslip_pdf.go` menambah `paid_leave_days` ke daftar kunci.
- [ ] **Step 4: Tes gagal (vitest).** Ringkasan kehadiran di `slip-lines-view` menampilkan "Cuti dibayar: 2 hari" dan "Cuti tanpa bayaran: 1 hari". Baris Rp 0 dari doc lama (yang masih memuat baris 0) tidak dirender.
- [ ] **Step 5: Implementasi frontend**, lalu jalankan `pnpm vitest run src/features/hris/payroll src/features/hris/ess` dan `pnpm tsc --noEmit`. Expected: PASS, 0 error.
- [ ] **Step 6: Commit** `feat(payroll): slip tampilkan cuti dibayar/tanpa bayaran, sembunyikan baris Rp 0 (EPIC-117)`.

### Task 2: Cross-check potongan izin dan sakit (G)

**Files:**
- Test: `apps/backend/internal/modules/payroll/domain/leave_deduction_rules_test.go` (baru)
- Create (paddydev docs, bukan erp-paddy): `docs/hris/acuan-mom-20261019/crosscheck-izin-sakit-2026-11.md`

**Interfaces:**
- Consumes: kalkulator yang sudah ada (`calculator.go` l.473-481).

- [ ] **Step 1: Tes yang mengunci acuan.** Pakai satu karyawan contoh: gaji pokok 4.000.000, makan 500.000, transport 500.000, jabatan 300.000, 26 hari kerja.
  - `TestPersonalLeaveDeductsOneThpDay`: 1 hari Izin Pribadi memotong (4 komponen ÷ hari kerja) × 1.
  - `TestInternPersonalLeaveNotDeducted`: potongan 0.
  - `TestSickDeductsTransportAndMeal`: 2 hari sakit memotong (makan + transport) ÷ hari kerja × 2.
  - `TestLeaveDayWithAttendanceNotDeducted`: hari izin yang punya absen tidak dipotong.
- [ ] **Step 2: Jalankan** `go test ./internal/modules/payroll/domain -run 'Leave|Sick'`. Expected: PASS, karena tes ini mengunci perilaku yang sudah sesuai. Bila ada yang FAIL, itu temuan: catat dan tanyakan ke user sebelum mengubah kode.
- [ ] **Step 3: Cross-check data (baca-saja, staging).** Untuk periode 21-10 s.d. 20-11, hitung ulang potongan Izin Pribadi dan Sakit tiap karyawan dari `hris.leaves` yang disetujui dan bandingkan dengan pratinjau payroll. Tulis ke dokumen cross-check: jumlah karyawan yang cocok, daftar selisih, serta celah K-G1 (surat sakit ≥ 2 hari, kuota 3/bulan, cuti melahirkan menurut acuan vs kode).
- [ ] **Step 4: Commit** tes di erp-paddy (`test(payroll): kunci aturan potongan izin/sakit (EPIC-117)`) dan dokumen di paddydev docs.

### Task 3: Checkout otomatis: aturan lokasi mengalahkan aturan "semua lokasi" (E)

Data 18-11 (baca-saja): di production keempat aturan nonaktif (3 "semua lokasi" sejak 10-11, ODOO-PPBA hari ini) dan tidak ada absen yang ditutup otomatis dalam 10 hari terakhir. Di staging ketiga aturan "semua lokasi" masih aktif. Kemungkinan besar karyawan yang melapor memakai staging. Task 7 menutup akses itu. Celah di kode tetap ada dan diperbaiki di sini.

**Files:**
- Modify: `apps/backend/internal/modules/hris/auto_checkout.go` (`autoCheckoutSQL` l.47-50)
- Test: `apps/backend/internal/modules/hris/auto_checkout_rules_integration_test.go`

- [ ] **Step 1: Tes gagal.** `TestAutoCheckoutBranchRuleOffOverridesAllBranches`: aturan jabatan X "semua lokasi" aktif dan aturan jabatan X di branch B nonaktif, absen terbuka di B lewat jam selesai shift → `RunAutoCheckout` menutup 0 baris. Absen di branch C tetap ditutup (1 baris).
- [ ] **Step 2: Jalankan** `go test ./internal/modules/hris -run AutoCheckout`. Expected: FAIL (2 baris ditutup).
- [ ] **Step 3: Implementasi.** Predikat memilih aturan paling spesifik: bila ada aturan `(position, branch = a.branch_id)`, status `is_active` aturan itu yang menentukan. Bila tidak ada, aturan `(position, branch IS NULL)` yang dipakai.
- [ ] **Step 4: Jalankan** seluruh tes `AutoCheckout`. Expected: PASS.
- [ ] **Step 5: Commit** `fix(hris): aturan checkout otomatis per lokasi mengalahkan aturan semua lokasi (EPIC-117)`.

### Task 4: Pengumuman tampil langsung di beranda ESS dan membunyikan lonceng (H)

**Files:**
- Create: `apps/backend/database/migrations/deltas/20261218090300_announcements_notified_at.sql`
- Modify: `apps/backend/internal/modules/hris/announcements.go`, `module.go` (loop)
- Modify: `apps/frontend/src/features/hris/ess/queries.ts` (`useEssBeranda`), `ess/components/beranda-page.tsx`
- Test: `hris/announcements_notify_integration_test.go` (baru), `ess/components/beranda-page.test.tsx`

**Interfaces:**
- Produces: kolom `hris.announcements.notified_at timestamptz NULL`; `func (s *Service) NotifyDueAnnouncements(ctx context.Context, now time.Time) (int, error)`.

- [ ] **Step 1: Tes gagal.** `TestAnnouncementNotifiesVisibleEmployeesOnce`:
  - pengumuman global yang tayang sekarang membuat 1 baris `public.notifications` per karyawan aktif yang punya `user_id`;
  - pengumuman departemen hanya untuk karyawan departemen itu;
  - `publish_at` besok membuat 0 baris, lalu dengan `now` = besok tepat 1 kali;
  - edit atau panggilan kedua membuat 0 baris baru.
  - Notifikasi bertipe `announcement` dan link-nya `/dashboard/me?pengumuman=<id>`.
- [ ] **Step 2: Jalankan** dan pastikan FAIL.
- [ ] **Step 3: Implementasi.**
  - `NotifyDueAnnouncements` memakai predikat `visibleTo` yang sama dengan feed, untuk pengumuman dengan `notified_at IS NULL`, lalu mengisi `notified_at` dalam satu transaksi.
  - `CreateAnnouncement` dan `UpdateAnnouncement` memanggilnya sesudah commit; kegagalannya hanya di-log.
  - Loop 5 menit di `module.go`, seperti `AutoCheckoutLoop`.
  - Migrasi mengisi `notified_at = now()` untuk pengumuman yang sudah ada, supaya rilis tidak membunyikan lonceng pengumuman lama.
- [ ] **Step 4: Tes gagal (vitest)** lalu implementasi:
  - `useEssBeranda` memakai `refetchOnMount: "always"` dan `staleTime: 0`;
  - `beranda-page` membuka `announcement-detail-dialog` bila URL memuat `?pengumuman=<id>`.
- [ ] **Step 5: Jalankan** tes Go hris dan vitest ess. Expected: PASS.
- [ ] **Step 6: Commit** `feat(hris): pengumuman membunyikan lonceng dan beranda ESS selalu segar (EPIC-117)`.

### Task 5: HR input cuti/izin dadakan dari kalender absensi (A)

**Files:**
- Modify: `apps/frontend/src/components/hris/AttendanceDayDialog.tsx`, `features/hris/attendance/.../attendance-recap-tab.tsx` (l.~303), `features/hris/leaves/components/new-leave-page.tsx`
- Modify (K-A1): `apps/backend/internal/modules/hris/leaves.go` (l.258-345)
- Test: `hris/leave_hr_direct_integration_test.go` (baru), `components/hris/AttendanceDayDialog.test.tsx` (baru), `new-leave-page.test.tsx`

**Interfaces:**
- Produces: props `AttendanceDayDialog` `{ employeeId?: string; canFileLeave?: boolean }`; `new-leave-page` membaca `?employee_id=&date=` sebagai nilai awal.

- [ ] **Step 1: Tes gagal (Go).** `TestHRFiledLeaveIsApproved`: HR mengajukan untuk karyawan lain → status `approved`, approver = karyawan HR, saldo terpotong, ada audit. `TestHRFiledLeaveStillChecksQuota`: kuota habis → ditolak dengan pesan kuota.
- [ ] **Step 2: Implementasi** di `CreateLeave`: bila `a.IsHR && !isSelf`, sisipkan dengan status `approved` lewat jalur persetujuan yang sudah ada, sehingga saldo, absen, dan notifikasi karyawan ikut. Notifikasi ke atasan tidak dikirim.
- [ ] **Step 3: Tes gagal (vitest).** Dialog hari dengan `canFileLeave` menampilkan tombol "Input cuti/izin" yang menuju `/dashboard/hris/leaves/insert?employee_id=E&date=2026-11-20`. Tanpa `canFileLeave`, tombol tidak ada. `new-leave-page` mengisi karyawan dan tanggal dari query string.
- [ ] **Step 4: Implementasi**, lalu jalankan tes Go hris, vitest, dan tsc. Expected: PASS, 0 error.
- [ ] **Step 5: Commit** `feat(hris): HR input cuti/izin dadakan dari kalender absensi (EPIC-117)`.

### Task 6: Kalender ESS: jadwal tidak kosong dan info tim (B)

**Files:**
- Modify: `apps/frontend/src/features/hris/attendance/api.ts` (l.171-177), `components/hris/AttendanceDayCell.tsx` (l.~108-125), `lib/hris/shifts.ts` (l.43-58), `components/hris/AttendanceCalendar.tsx`, `features/hris/ess/components/absensi-page.tsx`
- Create: `apps/backend/internal/modules/hris/team_calendar.go`, route di `routes.go`
- Test: `hris/team_calendar_integration_test.go`, `AttendanceDayCell.test.tsx`, `lib/hris/attendance-calendar.test.ts`, `features/hris/attendance/api.test.ts` (baru)

**Interfaces:**
- Produces: `GET /api/hris/me/team-calendar?start=YYYY-MM-DD&end=YYYY-MM-DD` → `{ days: [{ date, members: [{ employee_id, name, shift: "08.00-17.00"|null, off: bool, leave: bool, clocked_in: bool }] }] }`. Jenis cuti tidak dikirim (privasi). Rentang maksimal 62 hari.

- [ ] **Step 0: Reproduksi.** Minta HR menyebut satu akun yang kalendernya kosong. Di staging, buka `/dashboard/me/absensi` dengan akun itu dan catat respons `/api/hris/attendance/schedule`. Akar masalahnya dicatat di ledger sebelum Step 1.
- [ ] **Step 1: Tes gagal (vitest).**
  - `fetchEmployeeSchedule` meneruskan error, tidak lagi mengembalikan `rows: []`, dan kalender menampilkan pesan "Jadwal gagal dimuat";
  - sel hari di layar sempit menampilkan jam shift ringkas ("08–17") dan "Libur";
  - resolusi shift baris per-tanggal mengalahkan pola mingguan pada `effective_from` yang sama.
- [ ] **Step 2: Implementasi** ketiganya. Akun tanpa karyawan tertaut menerima pesan "Akun ini tidak tertaut ke data karyawan".
- [ ] **Step 3: Tes gagal (Go).** `TestTeamCalendarSameBranch`: dua rekan terjadwal di branch yang sama (satu sudah clock-in, satu cuti) dan satu karyawan branch lain → hanya kedua rekan yang muncul, dengan `clocked_in`/`leave` benar. `TestTeamCalendarFallbackReportingTo`: karyawan tanpa jadwal toko mendapat rekan seatasan.
- [ ] **Step 4: Implementasi** endpoint dari `hris.v_employee_schedule`, `hris.leaves` yang disetujui, dan `hris.attendance`. Tampilkan lapisan tim di dialog hari `AttendanceCalendar` hanya untuk `employeeId="me"`.
- [ ] **Step 5: Jalankan** tes Go hris, vitest hris, dan tsc. Expected: PASS, 0 error.
- [ ] **Step 6: Commit** `fix(hris): kalender ESS tampilkan jadwal, error, dan info tim (EPIC-117)`.

### Task 7: Tutup login karyawan di staging (D)

**Files:**
- Create: `apps/backend/database/sql/fixes/2026-11-18_staging_tutup_login_karyawan.sql`, `..._balik.sql`
- Test: `apps/backend/internal/modules/hris/staging_login_lock_integration_test.go`

Mekanisme: `auth.users.banned_until = 'infinity'` untuk akun ber-role `employee` (dan role lain selain `hrd`/`super_admin`), lalu hapus `auth.sessions` akun tersebut. `session.go:64` dan `apitoken.go:121` sudah menolak akun yang di-ban, termasuk sesi yang masih terbuka. Password tidak dihapus, sehingga pembalik cukup mengosongkan `banned_until` akun yang tercatat di snapshot.

- [ ] **Step 1: Tes gagal.**
  - Pratinjau tidak mengubah apa pun.
  - Eksekusi mem-ban `employee`, tidak menyentuh `hrd` dan `super_admin`, dan menghapus sesi akun yang di-ban.
  - BLOKIR bila `current_setting('kunci.lingkungan', true)` bukan `staging`, sebagai pengaman supaya skrip tidak jalan di production.
  - Pembalik hanya mengembalikan akun yang ada di snapshot.
- [ ] **Step 2: Implementasi** dengan pola EPIC-106, lalu jalankan tes. Expected: PASS.
- [ ] **Step 3: Commit** `chore(ops): skrip tutup login karyawan staging (EPIC-117)`.
- [ ] **Step 4 (butuh izin user):** backup staging ke `/mnt/hdd1/backup/paddydev/staging-before-tutup-login-*.dump`, jalankan pratinjau dan laporkan angkanya (perkiraan: 312 employee di-ban, 1 hrd + 4 super_admin tetap). Sesudah izin, eksekusi. Smoke: login akun karyawan ditolak, login HR berhasil.

### Task 8: Migrasi baris staging-only ke production (F)

**Files:**
- Create: `apps/backend/database/sql/fixes/2026-11-18_migrasi_staging_ke_prod.sql`, `..._balik.sql`
- Test: `apps/backend/internal/modules/hris/staging_migrate_integration_test.go`

Data (investigasi 18-11): staging adalah salinan production 10-11 05.52 WIB dan id-nya sama. Baris yang hanya ada di staging: 40 absen (9 karyawan, 10–18 Nov), 1 pengajuan lembur (pending), dan 1 koreksi absen yang diduga duplikat. Cuti, perjalanan dinas, dan kasbon: tidak ada.

Alur: ekspor baris staging-only ke file CSV mode 600 di `/mnt/hdd1/backup/paddydev/`. Skrip memuatnya ke tabel sementara di production dan menyisipkan dengan `ON CONFLICT DO NOTHING` pada id dan pada (employee_id, date).

- [ ] **Step 1: Tes gagal.**
  - Pratinjau tidak mengubah apa pun.
  - Eksekusi menyisipkan baris yang id-nya dan (karyawan, tanggal)-nya belum ada di tujuan.
  - Baris bentrok dilaporkan sebagai `BENTROK <karyawan> <tanggal>` dan tidak disisipkan (K-F1).
  - Jalan kedua menyisipkan 0 baris.
  - Pembalik menghapus tepat id yang disisipkan dan menolak bila baris itu sudah diubah di tujuan.
  - BLOKIR bila periode absensi tujuan berstatus `closed` atau ada payroll run final untuk periode itu.
- [ ] **Step 2: Implementasi** lalu jalankan tes. Expected: PASS.
- [ ] **Step 3: Commit** `chore(ops): skrip migrasi absen/lembur staging-only ke production (EPIC-117)`.
- [ ] **Step 4 (butuh izin user):** ekspor ulang dari staging (angka bisa bertambah sejak 18-11, sampai Task 7 dieksekusi), gladi di salinan production terbaru (pratinjau, eksekusi, pembalik, md5 identik), lalu production pada jam yang diizinkan, dengan backup lebih dulu. Smoke: 32 absen dan 1 lembur masuk, 8 bentrok dilaporkan ke HR.

### Task 9: Gerbang, review akhir, PR, staging, gladi

- [ ] **Step 1:** `go test ./internal/modules/...` (DB tes `127.0.0.1:55499`), `pnpm vitest run`, `pnpm tsc --noEmit`. Expected: semua PASS, 0 error.
- [ ] **Step 2:** review seluruh branch oleh reviewer segar (model paling mampu). Temuan Critical/Important diperbaiki RED→GREEN.
- [ ] **Step 3:** PR ke `development`. Merge, deploy staging, dan gladi rilis di salinan production masing-masing menunggu izin user. Rilis production memuat EPIC-110/111/112/114/116 dan EPIC-113 bila PR #16 sudah di-merge.
- [ ] **Step 4:** Automation Log EPIC-117 dan memori diperbarui.

## Urutan dan Ketergantungan

- Task 1, 2, 3, 4, 5, dan 6 saling lepas. Task 1 dan 4 sama-sama menambah migrasi, jadi nomor urutnya tetap seperti di atas.
- Task 7 dan 8 mengubah data. Untuk payroll Senin, urutan yang aman: Task 8 (data staging masuk production) sebelum payroll Senin dihitung; Task 7 sesudah ekspor terakhir Task 8, supaya tidak ada absen baru yang tertinggal di staging.
- Prioritas untuk Senin: Task 1, 2, 8, lalu 7. Task 3–6 bisa menyusul di rilis yang sama bila waktunya cukup.
