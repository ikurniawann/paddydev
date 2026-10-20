-- =============================================================================
-- Jenis cuti EPIC-058.
--
-- leader      : Izin Leader (Supervisor ke atas, 1 hari/bulan, tidak memotong).
-- marriage,
-- bereavement : Cuti Menikah & Cuti Duka. Form sudah menawarkan keduanya,
--               tetapi enum belum punya nilainya, sehingga insert gagal.
--
-- menstrual, emergency, unpaid TIDAK dihapus dari enum: riwayat masih
-- memakainya. Penghapusan untuk pengajuan baru ada di validasi Go.
-- File terpisah: nilai enum baru baru bisa dipakai setelah transaksi ini selesai.
-- =============================================================================

ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'leader';
ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'marriage';
ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'bereavement';
