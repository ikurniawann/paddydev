# EPIC-091: Kasbon/Pinjaman Terlihat oleh Atasan Langsung (`MODULE-HRIS`, `MODULE-PAYROLL`)

status: backlog
environment: dev
retries: 0

## Goal
Laporan user 05-11-2026: karyawan tidak bisa mengajukan pinjaman, atau pinjamannya tidak muncul di atasannya (Yosianti, Didit, Firdaus), padahal HR sudah mengatur atasan mereka.

### Temuan (production, read-only atas izin user 05-11)
- **Approver tahap 1 bukan atasan langsung.** Kasbon/pinjaman memakai "Manager Departemen" (EPIC-060 Q-APV-2, EPIC-076 R7): orang pertama berlevel Manager ke atas yang punya akun di rantai `reporting_to`.
  - Atasan langsung yang Supervisor dilewati, sehingga pengajuan tidak muncul di atasan langsung.
  - Yosianti → Regha (atasan langsung Alinda, SCM Supervisor). Firdaus Eka → Dimas (atasan langsung Asriani, Store Supervisor).
  - 244 karyawan aktif punya atasan langsung berlevel di bawah Manager.
- **Loan tidak mengirim notifikasi apa pun** saat diajukan. Manager hanya melihat angka badge.
- **Didit**: berstatus Probation sampai 29-10 13:21, jadi ditolak sesuai aturan. Sekarang Kontrak (masuk 21-06-2026): kasbon bisa; Pinjaman Karyawan butuh masa kerja 12 bulan.
- **Firdaus Nuzula**: sudah resign.

### Keputusan user 05-11
- **Approver tetap Manager**, tetapi atasan langsung (SPV) **ikut melihat detail dan mendapat notifikasi**.
- Probation tetap tidak boleh mengajukan.

## Ruling
- **R1 Notifikasi** (dalam transaksi pengajuan, `notify_user` type `approval`), judul "Pengajuan {Kasbon|Pinjaman Karyawan}: {nama}":
  - **Manager penyetuju**: "{nama} mengajukan {jenis} Rp… ({n}x cicilan). Menunggu persetujuan Anda di Area Karyawan → Pinjaman." Tautan `/dashboard/me/pinjaman`.
  - **Atasan langsung** (bila ada akun, bukan dirinya, dan bukan Manager penyetuju): "… Diputuskan oleh Manager {nama} lalu HR (atau 'HR'); lihat detail di Persetujuan Tim → Riwayat." Tautan `/dashboard/me/persetujuan-tim?tab=riwayat`.
- **R2 Baca lampiran.** Selain pemohon, Manager penyetuju, dan HR, **atasan langsung pemohon** boleh membuka dokumen pendukung dan jaminan.
- **R3 Riwayat.** `GET /api/hris/me/team-history` menambah `loans` lewat port payroll `TeamLoanHistory`:
  - kasbon/pinjaman bawahan langsung yang diajukan sejak awal periode lalu, ditambah yang masih menunggu (pending/manager_approved) berapa pun umurnya;
  - maksimal 300, terbaru dulu.

  Tab Riwayat menampilkan bagian "Kasbon & Pinjaman" dengan lencana:
  - Menunggu Manager {nama} / Menunggu HR / Disetujui Manager — menunggu HR;
  - Berjalan / Lunas / Ditolak.

  Detail baca-saja: jenis, pokok, cicilan, keperluan, dokumen, jaminan, alasan penolakan. `?tab=riwayat` membuka tab Riwayat.
- **Di luar cakupan**:
  - mengubah approver menjadi atasan langsung (user memilih tetap Manager);
  - membuka probation;
  - data HR (status/level jabatan) tetap urusan HR.

## Acceptance Criteria
- [ ] Manager penyetuju dan atasan langsung SPV mendapat lonceng saat kasbon/pinjaman diajukan; tanpa lonceng ganda bila keduanya orang yang sama.
- [ ] Atasan langsung melihat kasbon/pinjaman tim di Riwayat dan bisa membuka detail beserta lampirannya.
- [ ] Gates hijau; QA staging (HP + desktop).

## Automation Log
- 2026-11-05 — Investigasi laporan (staging + production read-only atas izin) dan 3 jawaban user. Worktree `.worktrees/epic091`, branch `feat/epic091-pinjaman-atasan`, ditumpuk di atas EPIC-090.
