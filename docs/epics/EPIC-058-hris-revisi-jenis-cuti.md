# EPIC-058: Revisi Jenis Cuti HRIS

status: backlog
environment: dev
retries: 0

## Goal
Jenis cuti untuk pengajuan baru disesuaikan dengan permintaan user:
- **Dihapus:** Cuti Haid, Izin Darurat, Izin Tanpa Gaji.
- **Ditambah:**
  - Izin Leader: Supervisor ke atas, 1x per bulan, tidak memotong gaji/THP.
  - Izin Pribadi: 3x per bulan, memotong 1 hari THP.
- Antar jenis cuti dapat dikombinasikan.

Riwayat cuti dengan jenis yang dihapus tetap terbaca di daftar, filter, ekspor, laporan, dan payroll.

## Tasks
Lihat docs/superpowers/plans/2026-10-20-hris-revisi-jenis-cuti.md (Task 1–8):
1. Enum `leader`/`marriage`/`bereavement` + label notifikasi in-app.
2. Katalog jenis cuti Go (satu sumber label, kuota, syarat jabatan).
3. Validasi pengajuan: jenis dihapus, Izin Leader (Supervisor ke atas), satu hari, kuota bulanan, tabrakan tanggal.
4. Endpoint `GET /api/hris/leave-quota` (pemilik, atau HR dengan menu `hris.workforce`; tanpa angka gaji).
5. Test payroll: Izin Leader tanpa potongan, Izin Pribadi = THP / pembagi × 1 hari (rule PIP EPIC-055).
6. Katalog jenis cuti frontend + ganti peta label lama.
7. Form ESS & HR: opsi sesuai kelayakan, aturan satu hari, sisa kuota.
8. Cek read-only level jabatan & riwayat, uji stack salinan DB, dokumentasi. Tanpa deploy.

## Acceptance Criteria
- Form ESS (`/dashboard/me/cuti`) dan HR (`/dashboard/hris/leaves/insert`) tidak menawarkan Cuti Haid, Izin Darurat, Izin Tanpa Gaji. API menolak ketiganya untuk pengajuan baru.
- Izin Leader hanya bisa diajukan untuk karyawan berlevel jabatan Supervisor ke atas, satu hari per pengajuan, maksimal 1 hari per bulan, dan tidak menambah potongan payroll.
- Izin Pribadi: satu hari per pengajuan, maksimal 3 hari per bulan, memotong THP / pembagi × hari (setting `personal_leave_deduction`).
- Pengajuan ditolak/dibatalkan mengembalikan kuota; kuota tiap bulan terpisah.
- Jenis berbeda boleh berdampingan; satu tanggal tidak boleh tercakup dua pengajuan aktif.
- Riwayat jenis yang dihapus tetap tampil dan bisa difilter; pengajuan lama yang pending tetap bisa diproses; ekspor CSV memakai label lama.
- Form dan endpoint kuota tidak menampilkan angka rupiah.

## Pertanyaan terbuka
Dua belas pertanyaan di bagian akhir rencana (makna "dikombinasikan", definisi SPV ke atas & pengisian level jabatan, satuan & bulan kuota, tunjangan harian, Cuti Menikah/Duka, data `emergency` Odoo, override HR, dan seterusnya) wajib dijawab user sebelum status naik ke `on-progress`.

## Automation Log
- 2026-10-20: rencana ditulis (`docs/superpowers/plans/2026-10-20-hris-revisi-jenis-cuti.md`). Temuan investigasi:
  - Jenis cuti adalah enum `public.leave_type`; tidak ada tabel master. `personal` (Izin Pribadi) sudah ada sejak EPIC-055 dan dipotong lewat PIP.
  - Label tersebar di 4 tempat Go, 7 tempat frontend, dan 1 fungsi SQL.
  - Form ESS menawarkan Menikah/Duka padahal enum belum punya nilainya (pengajuan gagal).
  - Tidak ada validasi tabrakan tanggal.
  - Level jabatan hasil import Odoo kemungkinan besar masih `Staff`.
  - DB test: 0 baris cuti. Live tidak disentuh; dari paket import diperkirakan ≈5 baris `emergency` (Cuti Menikah Odoo) dan 0 `unpaid`/`menstrual`.
  - Ruling R1–R9 tercatat di rencana, menunggu konfirmasi user.
