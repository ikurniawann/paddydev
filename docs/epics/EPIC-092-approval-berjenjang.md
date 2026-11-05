# EPIC-092: Approval Berjenjang SPV → Manager + HR Mewakili Atasan (`MODULE-HRIS`, `MODULE-PAYROLL`)

status: backlog
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
- [ ] Karyawan dengan atasan langsung Koordinator: penyetuju SPV.
- [ ] Cuti/dinas: SPV lalu Manager; Manager tidak bisa sebelum SPV; HR bisa mewakili kedua tahap.
- [ ] Kasbon: HR bisa mewakili tahap SPV/Manager.
- [ ] Lembur: SPV atau Manager (tidak berubah).
- [ ] Gates hijau; QA staging.

## Automation Log
- 2026-11-05 — Spesifikasi dari 3 jawaban user. Worktree `.worktrees/epic092`, branch `feat/epic092-approval-berjenjang`, ditumpuk di atas EPIC-091 (#79).
