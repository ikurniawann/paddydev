-- =============================================================================
-- Jenis cuti TnC 03-11-2026 (EPIC-084 R1).
--
-- birthday         : Birthday Leave (Cuti Ulang Tahun), 1 hari di bulan lahir.
-- miscarriage      : Cuti Keguguran, maks 45 hari kalender, surat dokter.
-- wife_miscarriage : Cuti Istri Keguguran, 2 hari.
-- child_marriage   : Cuti Menikahkan Anak, 2 hari.
-- child_ceremony   : Cuti Khitanan/Baptis Anak, 2 hari.
-- Aturan per jenis ada di katalog Go (domain/leave_types.go).
-- File terpisah: nilai enum baru baru bisa dipakai setelah transaksi ini selesai.
-- =============================================================================

ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'birthday';
ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'miscarriage';
ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'wife_miscarriage';
ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'child_marriage';
ALTER TYPE public.leave_type ADD VALUE IF NOT EXISTS 'child_ceremony';
