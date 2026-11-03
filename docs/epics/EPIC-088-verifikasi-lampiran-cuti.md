# EPIC-088: Verifikasi Lampiran Cuti/Izin oleh HR (`MODULE-HRIS`, ES-11)

status: backlog
environment: dev
retries: 0

## Goal
MOM 29-10-2026 butir 2.1 dan 3.1 (ES-11): role HRIS memeriksa lampiran pengajuan cuti dan izin, termasuk Birthday Leave dan Sakit. Permintaan user 03-11 (poin 11): "cuti ulang tahun ada attachment, upload attachment dari HR baru bisa dipake". Artinya cuti berlampiran baru berlaku setelah HR memverifikasi lampirannya.

Poin 12 (Izin Leader 1x/bulan, hangus, SPV ke atas, tanpa potong THP = ES-15) **sudah live** (EPIC-058 + EPIC-084). Tidak dikerjakan di sini.

### Jawaban user 03-11
- **Alur**: karyawan mengunggah lampiran saat mengajukan. HR memeriksa lampirannya, dan cuti baru disetujui setelah itu.
- **Urutan**: atasan langsung menyetujui dulu, lalu HR memverifikasi. Status "Disetujui" final baru setelah HR.
- **Cakupan**: semua pengajuan cuti/izin yang berlampiran (Birthday, Sakit, Keguguran, dan jenis lain yang lampirannya diisi).
- **Lampiran tidak sesuai**: dikembalikan ke karyawan dengan alasan. Karyawan cukup mengunggah ulang tanpa mengajukan dari awal, dan persetujuan atasan tetap berlaku.

## Kondisi kode per 03-11 (main `614d3694`)
- Status `hris.leaves`: `pending → approved | rejected | cancelled`.
- `DecideLeave` (`hris/leaves.go:546`): atasan langsung memutuskan, HR pemegang `approve` daun Cuti menjadi cadangan (EPIC-063/067). Menyetujui Cuti Tahunan memotong saldo dalam transaksi yang sama.
- Lampiran:
  - wajib untuk `birthday` dan `miscarriage` (`AttachmentRequired`, EPIC-084);
  - opsional untuk jenis lain;
  - path `leave-attachments/<employee_id>/<file>`;
  - hanya HR yang boleh mengubah lampiran (`UpdateLeave`).
- Pengecekan lampiran oleh HR **belum ada**. EPIC-084 sengaja mengeluarkannya dari cakupan.
- Konsumen status:
  - kuota bulanan dan bentrok tanggal menghitung `pending + approved`;
  - payroll dan absensi hanya membaca `approved`;
  - antrean dan badge Persetujuan Tim menghitung `pending` (`approvals.go:150,172`).
- Fallback Next `decideLeave` (`lib/hris/leaves-repo.ts`) adalah jalur lama yang tidak terjangkau selama Go aktif.

## Ruling
- **R1 Data.** Delta `20261109090000_leave_attachment_review.sql` menambah kolom di `hris.leaves`:
  - `attachment_review text NULL`, CHECK `('awaiting','returned','verified')`;
  - `manager_approved_by uuid NULL` (FK `hris.employees`, `ON DELETE SET NULL`), `manager_approved_name text`, `manager_approved_at timestamptz`;
  - `attachment_reviewed_by uuid NULL` (user id, tanpa FK), `attachment_reviewed_name text`, `attachment_reviewed_at timestamptz`;
  - `attachment_return_reason text`.

  Ditambah index parsial `(created_at) WHERE status='pending' AND attachment_review IS NOT NULL`. **Status tidak ditambah**: selama menunggu HR, pengajuan tetap `pending`, sehingga kuota, bentrok, payroll, dan absensi tidak berubah.
- **R2 Kapan perlu verifikasi.** Pengajuan perlu diverifikasi HR bila `attachment_url` terisi (tidak kosong setelah trim) **dan** penyetuju bukan verifikator. Verifikator = pemegang `approve` daun Cuti (`a.CanApprove(iam.LeafCuti)`).
  - Bila HR (sebagai cadangan atau karena ia atasan langsung) menyetujui pengajuan berlampiran, persetujuan itu sekaligus verifikasi: `attachment_review='verified'` dengan HR sebagai pemeriksa.
  - Pengajuan tanpa lampiran berjalan seperti sekarang.
- **R3 Atasan menyetujui pengajuan berlampiran.** Status tetap `pending`. Kolom diisi: `attachment_review='awaiting'`, `manager_approved_*`, `decided_as`.
  - Respons: "Disetujui atasan — menunggu verifikasi lampiran oleh HR".
  - HR mendapat notifikasi in-app (`NotifyHR`, kind `leave_attachment`): judul "Lampiran cuti menunggu verifikasi", tautan `/dashboard/hris/leaves?review=awaiting`.
  - Pesan WA ke karyawan: "... telah disetujui atasan dan menunggu verifikasi lampiran oleh HR."
  - Saldo Cuti Tahunan **belum** dipotong.
- **R4 Antrean atasan.** `TeamPendingLeaves` dan `TeamPendingCount` hanya menghitung `pending` dengan `attachment_review IS NULL`.
  - `DecideLeave` pada pengajuan yang `attachment_review`-nya terisi: atasan → 400 "Pengajuan sudah disetujui atasan dan menunggu verifikasi lampiran oleh HR".
  - HR boleh **menolak** lewat `DecideLeave`. HR yang menyetujui lewat `DecideLeave` → 400 yang sama; persetujuan harus lewat verifikasi.
- **R5 Verifikasi HR.** Route `POST /api/hris/leaves/attachment-review` dengan body `{ leave_id, action: "verify"|"return", reason? }`.
  - **Izin**: hanya verifikator, dan tidak untuk pengajuannya sendiri.
  - **Syarat**: `status='pending'` dan `attachment_review='awaiting'`. Bila tidak, 400 "Pengajuan ini tidak menunggu verifikasi lampiran".
  - **`verify`**, dalam satu transaksi:
    - `status='approved'`;
    - `approved_by=manager_approved_by`, `decided_as='manager'`, `decided_by_name=manager_approved_name`, `approved_at=now()`;
    - `attachment_review='verified'` beserta kolom pemeriksa;
    - potong saldo Cuti Tahunan dan hitung ulang sisa bawaan (jalur yang sama dengan `DecideLeave`);
    - audit `leave.attachment_verify`.

    Pesan "Lampiran terverifikasi — cuti disetujui". Karyawan mendapat notifikasi (`NotifyEmployee`, kind `leave_attachment`) "Cuti disetujui".
  - **`return`**:
    - `reason` wajib, kalau kosong 400 "Alasan pengembalian wajib diisi";
    - `attachment_review='returned'`, `attachment_return_reason`, kolom pemeriksa diisi;
    - audit `leave.attachment_return`.

    Pesan "Lampiran dikembalikan ke karyawan". Karyawan mendapat notifikasi "Lampiran cuti dikembalikan" berisi alasannya, dengan tautan `/dashboard/me/cuti`.
  - Balapan dua pemeriksa → 409 (`decisionRace`).
- **R6 Unggah ulang.** `PUT /api/hris/leaves/{id}` dengan `attachment_url` dari **pemilik** diterima hanya bila `status='pending'` dan `attachment_review='returned'`.
  - Path wajib diawali `leave-attachments/<employee_id pengajuan>/`. Bila tidak, 400 "Lampiran tidak valid".
  - Efeknya: `attachment_url` baru, `attachment_review='awaiting'`, `attachment_return_reason=NULL`, HR dinotifikasi lagi.
  - Pesan "Lampiran baru terkirim — menunggu verifikasi HR".
  - Selain kondisi itu, pemilik tetap 403 seperti sekarang. HR tetap boleh mengedit seperti sekarang; edit HR tidak mengubah `attachment_review`.
- **R7 Pembatalan.** Tidak berubah. Pemilik boleh membatalkan selama `pending`, termasuk saat menunggu HR atau dikembalikan.
- **R8 Tampilan.**
  - **HRIS → Cuti & Izin**:
    - filter status mendapat opsi "Menunggu verifikasi lampiran" (`review=awaiting`) dan "Lampiran dikembalikan" (`review=returned`). `GET /api/hris/leaves` menerima `review`; nilai lain diabaikan;
    - badge baris: "Menunggu verifikasi HR" atau "Lampiran dikembalikan";
    - baris `awaiting` milik orang lain menampilkan tombol "Verifikasi", "Kembalikan" (dialog alasan), dan "Tolak";
    - Detail menampilkan "Disetujui atasan: <nama>" dan "Diverifikasi HR: <nama>".
  - **ESS Cuti**: badge "Menunggu verifikasi HR" / "Lampiran dikembalikan". Bila dikembalikan, alasan tampil beserta tombol "Unggah ulang lampiran" (memakai `LeaveAttachmentField`).
  - **Persetujuan Tim**: setelah atasan menyetujui pengajuan berlampiran, toast memakai pesan dari server.
- **R9 Fallback Next.** `decideLeave` di Next menolak `approve` pada pengajuan berlampiran: 503 "Pengajuan berlampiran diproses oleh layanan utama, coba lagi sebentar". Route verifikasi hanya di Go (masuk `goNativePrefixes` di tes).
- **R10 Data lama.** Pengajuan `approved` sebelum rilis tidak diubah. Pengajuan `pending` berlampiran yang diputuskan setelah rilis mengikuti alur baru.
- **Di luar cakupan**:
  - verifikasi lampiran lembur dan koreksi absen;
  - kunci periode untuk verifikasi atau unggah ulang. Mengikuti `DecideLeave`, yang juga tidak mengunci; dicatat untuk HR;
  - badge HR terpisah. Badge Cuti tetap menghitung semua `pending`.

## Tasks
Rencana: `docs/superpowers/plans/2026-11-03-verifikasi-lampiran-cuti.md`.

## Acceptance Criteria
- [ ] Atasan menyetujui cuti berlampiran → status tetap menunggu, HR dinotifikasi, pengajuan hilang dari antrean atasan, saldo belum terpotong.
- [ ] HR memverifikasi → cuti disetujui (saldo tahunan terpotong bila Cuti Tahunan); HR mengembalikan → karyawan melihat alasan dan bisa unggah ulang, lalu antrean HR terisi lagi.
- [ ] HR menyetujui langsung pengajuan berlampiran → sekaligus terverifikasi. Pengajuan tanpa lampiran tidak berubah.
- [ ] Gates hijau; QA staging (HR, atasan, karyawan; desktop + HP).

## Automation Log
- 2026-11-03 — Pemetaan MOM 29-10 butir 11–12 + 4 jawaban user. Poin 12 sudah live (ES-15). Epic + rencana ditulis. Worktree `.worktrees/epic088`, branch `feat/epic088-verifikasi-lampiran` dari main `614d3694`.
