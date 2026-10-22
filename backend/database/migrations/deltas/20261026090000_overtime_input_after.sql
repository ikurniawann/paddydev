-- =============================================================================
-- Input lembur setelah selesai (EPIC-066): istirahat (menit) dan dokumentasi
-- wajib untuk input milik sendiri. hours tetap jam dibayar (durasi − istirahat).
-- Baris lama: break_minutes 0, attachments kosong.
-- =============================================================================

ALTER TABLE hris.overtime_requests
  ADD COLUMN IF NOT EXISTS break_minutes integer NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS attachments text[] NOT NULL DEFAULT '{}';

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'overtime_requests_break_minutes_check') THEN
    ALTER TABLE hris.overtime_requests ADD CONSTRAINT overtime_requests_break_minutes_check
      CHECK (break_minutes BETWEEN 0 AND 720);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'overtime_requests_attachments_check') THEN
    ALTER TABLE hris.overtime_requests ADD CONSTRAINT overtime_requests_attachments_check
      CHECK (cardinality(attachments) <= 3);
  END IF;
END $$;
