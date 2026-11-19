# HR Batalkan Cuti/Izin Disetujui Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HR/admin bisa membatalkan pengajuan cuti/izin (menunggu atau disetujui) dari halaman Cuti & Izin dengan alasan wajib yang tersimpan dan teraudit, sehingga izin yang batal (karyawan ternyata masuk) tidak memotong payroll.

**Architecture:** Endpoint yang sudah ada (`PUT /api/hris/leaves/:id` dengan `status: "cancelled"`) diperluas: menerima `cancel_reason`, menyimpan alasan/pembatal/waktu di `hris.leaves`, dan menulis audit lewat `auditDecision` yang dipakai keputusan cuti. Halaman Cuti & Izin mendapat tombol Batalkan dengan dialog alasan. Payroll tidak berubah karena hanya membaca status `approved`.

**Tech Stack:** Go (`apps/backend/internal/modules/hris`), migrasi delta SQL, Next.js + React Query (`apps/frontend`), Vitest, Go integration test (`-p 1`).

**Spec:** `docs/epics/EPIC-121-hr-batalkan-cuti-disetujui.md`

## Global Constraints

- Repo erp-paddy, branch `feat/epic121-batalkan-cuti` dari `origin/development`, worktree `.worktrees/erp-epic121`. Jangan bekerja di `~/WORK/erp-paddy`.
- Pembatalan tidak menghapus baris. Status `cancelled`.
- Alasan wajib bila HR membatalkan pengajuan milik orang lain atau yang sudah disetujui: dipangkas, minimal 5 karakter. Pesan galat: "Alasan pembatalan wajib diisi (min. 5 karakter)". Pemilik yang membatalkan pengajuannya sendiri saat masih menunggu tidak wajib alasan (perilaku lama).
- Aturan izin tidak berubah: pemilik hanya saat menunggu; disetujui hanya HR/admin; kuota Cuti Tahunan dikembalikan seperti sekarang.
- Kolom baru di `hris.leaves`: `cancel_reason text`, `cancelled_by uuid`, `cancelled_by_name text`, `cancelled_at timestamptz` (semua nullable).
- Audit: aksi `leave.cancel`, tabel `hris.leaves`, berisi status sebelum dan `cancel_reason`, label "Nama · Jenis · tanggal" seperti keputusan.
- Teks UI Indonesia, ikuti `stop-slop`. Tombol "Batalkan", judul dialog "Batalkan pengajuan?", tombol konfirmasi "Batalkan pengajuan", toast "Pengajuan dibatalkan".
- Next fallback `apps/frontend/src/app/api/hris/leaves/[id]/route.ts` mengikuti aturan yang sama.
- Production: tidak ada skrip database. Rilis mengikuti aturan rilis (backup dulu, hindari 07-08, 13, 16-17, 21 WIB), lalu HR membatalkan lewat UI.

## Review Focus

1. HR membatalkan izin yang tanggalnya sudah lewat dan periodenya masih terbuka: berhasil; payroll hitung ulang tidak lagi memotong.
2. Periode absensi sudah ditutup: pembatalan tetap boleh (tidak mengubah absensi); run payroll yang sudah disetujui tidak ikut berubah; dialog memberi catatan bila tanggal cuti ada di periode tertutup.
3. Dua HR membatalkan bersamaan: yang kedua mendapat 409 "Pengajuan sudah berubah, muat ulang halaman".
4. HR membatalkan pengajuan miliknya sendiri yang sudah disetujui: boleh (HR), alasan wajib.
5. Alasan berisi spasi saja: ditolak.

---

### Task 1: Backend batal dengan alasan, pembatal, dan audit

**Files:**
- Create: `apps/backend/database/migrations/deltas/20261220090000_leaves_cancel_reason.sql`
- Modify: `apps/backend/internal/modules/hris/leaves.go` (`leaveUpdate`, `UpdateLeave`)
- Modify: `apps/backend/internal/modules/hris/http_leaves.go` (`updateLeave` membaca `cancel_reason`)
- Modify: `apps/backend/internal/modules/hris/postgres_leaves.go` (`CancelLeave`) dan interface Repository yang mendeklarasikannya
- Modify: Next fallback `apps/frontend/src/app/api/hris/leaves/[id]/route.ts` dan fungsi lib yang dipanggilnya
- Test: `apps/backend/internal/modules/hris/leave_cancel_integration_test.go`

**Interfaces:**
- Produces: `PUT /api/hris/leaves/:id` body `{ status: "cancelled", cancel_reason?: string }`. Baris cuti (list/detail) membawa `cancel_reason`, `cancelled_by_name`, `cancelled_at`.
- `CancelLeave(ctx, id, fromStatus string, refund *quotaMove, by CancelBy) error` dengan `type CancelBy struct { UserID string; Name string; Reason *string }`.

- [ ] **Step 1: Tes gagal** di `leave_cancel_integration_test.go`:
  - `TestHRCancelsApprovedPersonalLeaveWithReason`: Izin Pribadi 1 hari disetujui HR, lalu `PUT {status:"cancelled", cancel_reason:"Batal, karyawan masuk"}` → 200; `status = cancelled`, `cancel_reason = "Batal, karyawan masuk"`, `cancelled_by_name` = nama HR, `cancelled_at` terisi; ada audit `leave.cancel` untuk id itu.
  - `TestHRCancelApprovedNeedsReason`: tanpa `cancel_reason` → 400 "Alasan pembatalan wajib diisi (min. 5 karakter)"; `"   "` → 400; status tetap `approved`.
  - `TestOwnerCancelsPendingWithoutReason`: pemilik membatalkan pengajuan menunggu miliknya tanpa alasan → 200.
  - `TestStaffCannotCancelApproved`: pemilik membatalkan yang disetujui → 403 "Cuti yang sudah disetujui hanya bisa dibatalkan oleh HRD/admin".
  - `TestCancelledPersonalLeaveNotInMonthlyUse`: sesudah dibatalkan, `MonthlyLeaveUse` bulan itu tidak menghitungnya.
- [ ] **Step 2:** `source .superpowers/env.sh && cd apps/backend && go test -p 1 ./internal/modules/hris -run 'Cancel'` → FAIL.
- [ ] **Step 3:** Migrasi kolom; `UpdateLeave` memvalidasi alasan; `CancelLeave` menyimpan kolom; audit lewat `auditDecision(ctx, q, a, "leave.cancel", "hris.leaves", id, label, before, after, meta)` dalam transaksi yang sama. Terapkan ke DB tes (`go run ./cmd/migrate -apply`). Samakan Next fallback.
- [ ] **Step 4:** Tes Step 2 → PASS; `go test -p 1 ./internal/modules/hris` hijau (pemanggilan HR di `TestCancelOldLeaveRefundsAnnualQuota` dan `TestCancelLeaveRefundsOnce` diberi alasan).
- [ ] **Step 5:** Commit `feat(hris): HR membatalkan cuti disetujui dengan alasan dan audit (EPIC-121)`.

### Task 2: Tombol Batalkan di halaman Cuti & Izin

**Files:**
- Modify: `apps/frontend/src/features/hris/leaves/components/leaves-page.tsx`
- Create: `apps/frontend/src/features/hris/leaves/components/cancel-leave-dialog.tsx`
- Modify: api/mutations cuti (tempat approve dipanggil) untuk `cancelLeave(id, reason)`
- Test: `apps/frontend/src/features/hris/leaves/components/cancel-leave-dialog.test.tsx`, `leaves-page.test.tsx`

**Interfaces:**
- Consumes: endpoint Task 1.
- Produces: `CancelLeaveDialog({ leave, open, onOpenChange })`.

- [ ] **Step 1: Tes gagal:**
  - Halaman: HR melihat "Batalkan" pada baris menunggu dan disetujui; tidak pada ditolak/dibatalkan; pengguna tanpa hak keputusan tidak melihatnya.
  - Dialog: "Batalkan pengajuan" nonaktif sampai alasan ≥ 5 karakter setelah dipangkas; kirim memanggil API dengan `{status:"cancelled", cancel_reason}`; sukses → toast "Pengajuan dibatalkan" dan daftar dimuat ulang.
  - Baris "Dibatalkan" menampilkan alasan dan nama pembatal.
- [ ] **Step 2:** vitest → FAIL.
- [ ] **Step 3:** Implementasi. Dialog menampilkan nama, jenis, tanggal dan teks "Pengajuan tidak lagi dihitung di payroll dan jatah bulanan."
- [ ] **Step 4:** vitest folder `features/hris/leaves` → PASS; tsc dan eslint bersih.
- [ ] **Step 5:** Commit `feat(hris): tombol Batalkan pengajuan cuti untuk HR (EPIC-121)`.

### Task 3: Gerbang, staging, rilis, dan kasus Sri Wardani

- [ ] Gerbang lokal (`go test -p 1 ./...`, `go vet`, `tsc`, lint, `pnpm test`), review akhir, PR ke `development`.
- [ ] Merge dan deploy staging hanya dengan izin user; QA HR di staging.
- [ ] Ikut rilis production berikutnya (backup dulu). Sesudahnya HR membatalkan Izin Pribadi Sri Wardani 16-11 dengan alasan "Batal, karyawan masuk (clock in 08.17, clock out 17.16)". Cek read-only: status `cancelled`, audit ada.
- [ ] Bila payroll November production sudah dihitung sebelum pembatalan: hitung ulang slip Sri Wardani (Edit → Hitung ulang).
- [ ] Perbarui EPIC-121 Automation Log.
