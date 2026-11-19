# EPIC-121: HR Membatalkan Cuti/Izin yang Sudah Disetujui (`MODULE-HRIS`)

status: backlog
environment: dev
retries: 0

## Goal
Permintaan user 20-11-2026:

> tgl 16 november email paddyhroperation@gmail.com melakukan pengajuan tapi tidak jadi karena ybs masuk, apa bisa dihapus karena hari tersebut user melakukan clock in clock out

Rencana: [2026-11-20-hr-batalkan-cuti-disetujui.md](../superpowers/plans/2026-11-20-hr-batalkan-cuti-disetujui.md).

## Temuan (production, read-only 20-11)
- Pengajuan: **Izin Pribadi** Sri Wardani (NIP 202604579) tanggal 16-11-2026, 1 hari, status **disetujui** oleh Risky Melinda 11-11 17.22 WIB.
- Absensi 16-11: hadir di Paddy HQ Bandung, masuk 08.17, pulang 17.16, tidak terlambat.
- Dampak bila dibiarkan: Izin Pribadi memotong 1 hari THP dan memakai 1 dari 3 jatah Izin Pribadi bulan itu. Payroll hanya menghitung cuti berstatus `approved`.
- Periode absensi 21-10 s.d. 20-11 masih terbuka (closing otomatis 22-11). Production belum punya run payroll.
- Backend sudah bisa membatalkan cuti yang disetujui oleh HR (`PUT /api/hris/leaves/:id`, `status: cancelled`, kuota Cuti Tahunan dikembalikan). Halaman Cuti & Izin belum punya tombolnya, dan pembatalan tidak menyimpan alasan maupun audit.

## Keputusan desain (diusulkan, menunggu review user)
- **Dibatalkan, bukan dihapus.** Status menjadi "Dibatalkan" dengan alasan, nama pembatal, dan waktu; riwayat tetap ada untuk audit. Payroll dan jatah bulanan otomatis tidak menghitungnya.
- Tombol **Batalkan** di halaman Cuti & Izin untuk HR/admin, pada pengajuan menunggu dan disetujui. Alasan wajib (min. 5 karakter).
- Setelah rilis, HR membatalkan izin Sri Wardani 16-11 lewat tombol itu dengan alasan "Batal, karyawan masuk (clock in 08.17, clock out 17.16)". Tidak ada skrip database.

## Tasks
Lihat rencana. Belum dieksekusi.

## Automation Log
- 20-11-2026: temuan production dicek read-only; rencana ditulis, menunggu review user.
- 20-11-2026 00.36 WIB: atas permintaan user ("production status tgl 16 ... izin pribadi cancel atau di hapus aja agar ketika payroll tidak kepotong thp nya"), Izin Pribadi Sri Wardani 16-11 **dibatalkan langsung di production** sebelum EPIC-121 rilis: backup `~/backups/prod-db-before-batal-izin-sri-20261120-0036.dump`, satu transaksi dengan penjaga (id, karyawan, status approved), audit `leave.cancel` beralasan "Batal, karyawan masuk (clock in 08.17, clock out 17.16)". Percobaan pertama batal sendiri karena trigger `fn_notify_leave_changes` butuh search_path aplikasi; diulang dengan `SET LOCAL search_path` (lihat memori ops-sql-search-path). Production belum punya run payroll, jadi tidak ada yang perlu dihitung ulang.
- Implementasi: branch `feat/epic121-batalkan-cuti` (9669bee, 0d92ded, perbaikan review akhir 9931671). Branch rilis `release/epic121-prod` = production 80afcce + 3 commit itu (tanpa EPIC-118/119/120).

