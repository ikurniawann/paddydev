-- =============================================================================
-- Lampiran pengajuan kasbon & Pinjaman Karyawan (EPIC-076 R8): dokumen
-- pendukung (wajib untuk kasbon) dan jaminan/surat berharga (wajib untuk
-- pinjaman). Path file privat loan-attachments/<employeeId>/…, maks 3 per kolom.
-- Baris lama: kedua kolom kosong.
-- =============================================================================

ALTER TABLE hris.loans
  ADD COLUMN IF NOT EXISTS attachments text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS collateral_attachments text[] NOT NULL DEFAULT '{}';

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'loans_attachments_check') THEN
    ALTER TABLE hris.loans ADD CONSTRAINT loans_attachments_check
      CHECK (cardinality(attachments) <= 3 AND cardinality(collateral_attachments) <= 3);
  END IF;
END $$;
