# EPIC-091: Penyetuju Minimal SPV untuk Semua Pengajuan + Kasbon/Pinjaman Tiga Tahap (`MODULE-HRIS`, `MODULE-PAYROLL`)

status: review
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
- Keputusan awal: approver tetap Manager, SPV ikut melihat. **Diralat user** ("Approver minimal atasan dengan jabatan SPV untuk semua pengajuan bisa menyetujui"):
  - **semua pengajuan** (cuti/izin, lembur, perjalanan dinas, kasbon/pinjaman): penyetuju = **atasan terdekat di rantai `reporting_to` yang berjabatan minimal Supervisor**. Atasan langsung berlevel di bawah SPV (Staff, mis. Store Leader) dilewati;
  - **kasbon/pinjaman: SPV → Manager (bila ada) → HR**.
- Probation tetap tidak boleh mengajukan.

## Ruling
- **R0 Penyetuju minimal SPV.** Fungsi `hris.request_approver(karyawan)` (delta `20261110090000_request_approver_spv.sql`) mengembalikan atasan terdekat yang memenuhi semua syarat berikut:
  - aktif dan punya akun login;
  - level jabatan `supervisor`, `assistant manager`, `manager`, `senior manager`, `general manager`, `director`, atau `c-level`, **atau level kosong** (tidak terbukti di bawah SPV);
  - maksimal 10 tingkat; siklus dan atasan = dirinya berhenti; tidak ada → HR saja.

  Dipakai untuk:
  - antrean dan badge Persetujuan Tim, keputusan cuti, lembur (termasuk koreksi), dan perjalanan dinas;
  - baca lampiran, lonceng/WA pengajuan;
  - Riwayat tim.

  `approver_id` menggantikan `reporting_to_active` di data lembur HR (tombol Koreksi). Absensi Tim tetap memakai subtree `reporting_to`.
- **R0b Kasbon/pinjaman tiga tahap.**
  - **Penyetuju** (`loanStages`): tahap SPV = `request_approver`, dan tahap Manager = `LoanApprover` (≥ Manager) bila orangnya berbeda. Bila keduanya sama, atau salah satunya tidak ada, hanya satu tahap atasan.
  - **Kolom** `supervisor_approver_id/approved_by/approved_at`. Status tetap `pending` sampai Manager setuju (`manager_approved`), lalu HR.
  - **SPV menyetujui**: "{Jenis} disetujui SPV — menunggu persetujuan Manager", dan Manager dilonceng. **SPV menolak**: selesai.
  - **Giliran**: yang belum gilirannya → 403 "Bukan giliran Anda memutus pengajuan ini".
  - **Pengajuan lama** yang pending tanpa tahap SPV tetap di Manager.
- **R1 Notifikasi** (dalam transaksi pengajuan, `notify_user` type `approval`), judul "Pengajuan {Kasbon|Pinjaman Karyawan}: {nama}":
  - **Penyetuju tahap pertama** (SPV, atau Manager bila tanpa tahap SPV): "{nama} mengajukan {jenis} Rp… ({n}x cicilan). Menunggu persetujuan Anda di Area Karyawan → Pinjaman." Tautan `/dashboard/me/pinjaman`. Manager dilonceng setelah SPV setuju.
  - **Atasan langsung** yang bukan penyetuju (mis. Store Leader berlevel Staff): "… Diputuskan oleh {SPV}, lalu {Manager}, lalu HR; lihat detail di Persetujuan Tim → Riwayat." Tautan `/dashboard/me/persetujuan-tim?tab=riwayat`.
- **R2 Baca lampiran.** Selain pemohon, SPV dan Manager penyetuju, dan HR, **atasan langsung pemohon** boleh membuka dokumen pendukung dan jaminan.
- **R3 Riwayat.** `GET /api/hris/me/team-history` menambah `loans` lewat port payroll `TeamLoanHistory`:
  - kasbon/pinjaman bawahan langsung yang diajukan sejak awal periode lalu, ditambah yang masih menunggu (pending/manager_approved) berapa pun umurnya;
  - maksimal 300, terbaru dulu.

  Tab Riwayat menampilkan bagian "Kasbon & Pinjaman" dengan lencana:
  - Menunggu SPV {nama} / Menunggu Manager {nama} / Menunggu HR / Disetujui Manager — menunggu HR;
  - Berjalan / Lunas / Ditolak.

  Detail baca-saja: jenis, pokok, cicilan, keperluan, dokumen, jaminan, alasan penolakan. `?tab=riwayat` membuka tab Riwayat.
- **Di luar cakupan**:
  - membuka probation;
  - data HR (status/level jabatan) tetap urusan HR.

## Acceptance Criteria
- [x] Semua pengajuan diputus atasan terdekat minimal SPV; atasan langsung berlevel Staff tidak memutus.
- [x] Kasbon/pinjaman SPV → Manager (bila ada) → HR; lonceng per tahap tanpa duplikasi; atasan langsung non-penyetuju dilonceng untuk melihat detail.
- [x] Atasan langsung dan penyetuju melihat kasbon/pinjaman tim di Riwayat beserta lampirannya.
- [x] Gates hijau; QA staging (HP + desktop).

## Automation Log
- 2026-11-05 — Investigasi laporan (staging + production read-only atas izin) dan 3 jawaban user. Worktree `.worktrees/epic091`, branch `feat/epic091-pinjaman-atasan`, ditumpuk di atas EPIC-090.
- 2026-11-05 — Implementasi.
  - Lonceng + Riwayat kasbon (`52a7b9f3`, `1c2035a5`). Setelah ralat user: `hris.request_approver` untuk semua pengajuan (`ac1d88d0`), tiga tahap kasbon (`7c8ba419`), status SPV di Riwayat (`986169b8`), Riwayat untuk atasan langsung/penyetuju tersimpan (`42a7c7db`).
  - Gates: `go test -p 1` hris + payroll + app ok, vet + gofmt bersih, vitest 4594 lulus, tsc + eslint bersih.
- 2026-11-05 — Review akhir (reviewer terpisah): 0 Critical, 4 Important, diperbaiki dengan tes (`a3e2dfbd`):
  - lonceng Manager "(0x cicilan)";
  - dokumen 403 bagi penyetuju saat ini pada pengajuan lama;
  - tahap SPV dilewati bila SPV tersimpan nonaktif/tanpa akun;
  - HR yang juga penyetuju tersimpan tetap bisa bertindak sebagai HR;
  - laporan "tanpa atasan" mengikuti `request_approver` (alasan baru: `no_account`, `below_spv`).

  Ditunda:
  - fallback Next masih `reporting_to`;
  - lonceng ke atasan langsung nonaktif;
  - performa fungsi per baris (EXPLAIN di staging);
  - `?tab=` hanya saat halaman dibuka;
  - teks penolakan "Hanya atasan langsung…" dan "Manager Departemen";
  - "SPV SPV Toko" ganda;
  - tes HR di tahap SPV untuk approve.
- 2026-11-05 — **Staging** `local-20261105-162804` (gabungan lokal EPIC-087 + EPIC-091, yang memuat #76–#78; merge `10254196` tidak di-push).
  - Backup `staging-db-before-epic091b-20261105-162803.dump`. Migrasi `20261110090000` ok, verify 10780 objek, 0 berbeda.
  - Data staging: 4 karyawan berganti penyetuju (atasan langsung berlevel Staff), 2 karyawan tanpa penyetuju (ke HR).
  - QA 19/19 lulus (karyawan → Leader Staff → SPV → Manager; HP + desktop):
    - cuti: lonceng ke SPV, bukan Leader; Leader 403; SPV menyetujui dari Persetujuan Tim;
    - kasbon: tahap SPV lalu Manager; Manager 403 sebelum SPV;
    - lonceng Manager setelah SPV; status `manager_approved`;
    - Leader melihat Riwayat "Menunggu SPV" + dokumen;
    - 0 error API dan console.
  - Data demo dan berkas dihapus.

