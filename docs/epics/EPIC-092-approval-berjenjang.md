# EPIC-092: Approval Berjenjang SPV → Manager + HR Mewakili Atasan (`MODULE-HRIS`, `MODULE-PAYROLL`)

status: review
environment: dev
retries: 0

## Goal
Permintaan user 05-11-2026:

> "pastikan jika atasan langsung karyawan adalah koordinator, approval langsung ke SPV karena minimal approval SPV. Untuk approval semuanya berjenjang kecuali lembur. Yang berjenjang ini HR bisa approve jika atasan langsung sedang tidak available utk approve"

### Jawaban user 05-11
- **Cuti/izin dan perjalanan dinas**: berjenjang **SPV → Manager** (bila ada Manager lain di atasnya), lalu selesai. Cuti berlampiran tetap diverifikasi HR (EPIC-088).
- **Kasbon/pinjaman**: tetap SPV → Manager → HR (EPIC-091).
- **HR menggantikan**: HR (pemegang `approve`) boleh menyetujui tahap atasan yang sedang berjalan **kapan saja**. Tercatat "mewakili {nama atasan}", lalu lanjut ke tahap berikutnya.
- **Lembur**: tidak berjenjang. SPV terdekat atau Manager di atasnya, cukup salah satu (EPIC-091 R0c); HR cadangan.

### Koordinator
Di staging, semua jabatan Koordinator/Coordinator (15 jabatan) berlevel **Staff**, jadi otomatis dilewati `request_approver` dan approval langsung ke SPV. Tidak ada perubahan kode; tes memastikan perilakunya.

## Ruling
- **R1 Tahap.** `hris.stage_approver(karyawan, spv_selesai)` (delta `20261110090300_tiered_approval.sql`) menentukan penyetuju tahap berjalan:
  - SPV = `request_approver`, Manager = `request_manager`;
  - tanpa SPV → Manager; tanpa Manager atau sama dengan SPV → SPV (satu tahap);
  - selain itu SPV dulu, lalu Manager setelah `spv_approved_at` terisi.
- **R2 Cuti/izin.** Kolom `spv_approved_by/name/at` di `hris.leaves`.
  - **SPV menyetujui** saat masih ada Manager: tetap `pending`, `spv_approved_*` diisi, Manager dilonceng. Pesan "Disetujui SPV — menunggu persetujuan Manager".
  - **Penyetuju tahap terakhir** menyetujui: alur lama (approved, atau menunggu verifikasi HR bila berlampiran).
  - **Menolak**: penyetuju tahap berjalan atau HR.
  - **Atasan yang bukan gilirannya** → 403 "Bukan giliran Anda memutus pengajuan ini".
- **R3 Perjalanan dinas.** Sama dengan R2, dengan kolom `spv_approved_*` di `hris.business_trips`.
- **R4 HR mewakili.** HR yang menyetujui saat ada penyetuju tahap berjalan memajukan tahap itu, dan `decided_by_name`/`spv_approved_name` = "{HR} (mewakili {atasan})". Tanpa penyetuju sama sekali, HR memutus final seperti sebelumnya.
- **R5 Kasbon.** Saat `pending`, HR boleh menyetujui tahap SPV atau Manager atas nama mereka:
  - tahap SPV → `supervisor_approved_at` diisi + `supervisor_approved_name`, Manager dilonceng;
  - tahap Manager → `manager_approved` + `manager_approved_name`;
  - lalu tahap HR seperti biasa.
- **R6 Antrean.** Persetujuan Tim (cuti, dinas) dan badge hanya menampilkan pengajuan yang tahap berjalannya milik akun itu. Lembur tetap salah satu.
- **R7 Tampilan.** Lencana "Disetujui SPV — menunggu Manager" di HRIS Cuti & Izin, Perjalanan Dinas, ESS, dan Riwayat; Detail menampilkan siapa yang menyetujui tahap SPV.
- **Di luar cakupan**:
  - lembur berjenjang;
  - pengajuan pending lama (tidak punya tahap SPV tercatat, jadi tetap mengikuti tahap berjalan menurut fungsi).

## Acceptance Criteria
- [x] Karyawan dengan atasan langsung Koordinator: penyetuju SPV.
- [x] Cuti/dinas: SPV lalu Manager; Manager tidak bisa sebelum SPV; HR bisa mewakili kedua tahap.
- [x] Kasbon: HR bisa mewakili tahap SPV/Manager.
- [x] Lembur: SPV atau Manager (tidak berubah).
- [x] Gates hijau; QA staging.

## Automation Log
- 2026-11-05 — Spesifikasi dari 3 jawaban user. Worktree `.worktrees/epic092`, branch `feat/epic092-approval-berjenjang`, ditumpuk di atas EPIC-091 (#79).
- 2026-11-05 — Backend: `02cf0a5b` cuti/dinas berjenjang + HR mewakili (R1–R4, R6), `748717a7` kasbon HR mewakili tahap SPV/Manager (R5). Tes lama yang mengunci aturan lama (HR sebelum Manager → 400) diganti dengan komentar EPIC-092 R5.
- 2026-11-05 — UI (R7) `289314d0`: Pinjaman HR "Setujui (mewakili atasan)" pada tahap atasan; lencana "Disetujui SPV — menunggu Manager" di HRIS Cuti & Izin, Perjalanan Dinas, Riwayat; Detail + antrean Manager menampilkan "Disetujui SPV oleh …". Helper `frontend/src/lib/hris/tiered-approval.ts`.
- 2026-11-05 — Review akhir (opus, terpisah). Diperbaiki:
  - `ed0fbac5` HR yang juga peminjam tidak bisa menyetujui tahap atasan atas pinjamannya sendiri (403, tombol disembunyikan); tes `TestLoanHRCannotActOnBehalfForOwnLoan` RED→GREEN.
  - `ed0fbac5` saat menunggu verifikasi lampiran, tolakan HR tidak ditulis "mewakili"; nama verifikator lampiran selalu nama HR sendiri; tes di `TestLeaveTieredApproval` RED→GREEN.
  - `a7021001` antrean/badge berjenjang disaring `request_team` dulu: staging 80 → 20 ms, setara pada 27 atasan × 632 pasangan (0 berbeda).
  - `a7021001` ESS Cuti memakai label pendek "Menunggu Manager" (QA HP: label panjang memotong tanggal).
- 2026-11-05 — Ruling: HR yang juga peminjam tetap boleh memutus **tahap HR terakhir** pinjamannya sendiri seperti sebelum EPIC-092; yang ditolak hanya "mewakili atasan". Biaya bila salah: satu pengecekan tambahan di `decideLoanHR`.
- 2026-11-05 — Minor ditunda (dari review akhir):
  - SPV yang gilirannya sudah lewat (HR mewakili) masih bisa menolak dari tampilan lama;
  - bila Manager keluar setelah SPV setuju, tahap kembali ke SPV dan lencana tetap "menunggu Manager";
  - `supervisor_approved_name`/`manager_approved_name` kasbon hanya terisi di jalur HR mewakili, dan belum tampil di UI;
  - label "mewakili atasan" tetap tampil bila HR sendiri adalah atasan tahap berjalan;
  - Manager yang juga HR menerima lonceng dari tindakannya sendiri.
- 2026-11-05 — Gates: Go `go test -p 1 ./...` hijau; Vitest 6492 lulus; tsc + eslint bersih. Staging: backup `staging-db-before-epic092-20261105-175230.dump` dan `staging-db-before-epic092b-20261105-175743.dump`, deploy `local-20261105-175749` (EPIC-087 + EPIC-092 di atas #79), migrasi `20261110090300` ok, 0 berbeda. QA Playwright 23/23 (HP + desktop): Leader/Koordinator (Staff) 403, Manager sebelum SPV 403 (cuti/dinas), lembur langsung oleh Manager, SPV → Manager cuti, lencana di Riwayat/ESS/HR Cuti/HR Dinas, HR mewakili SPV + Manager (dinas, cuti), HR mewakili SPV kasbon lalu Manager → menunggu HR; 0 respons ≥400, 0 error konsol. Data demo dibersihkan.
