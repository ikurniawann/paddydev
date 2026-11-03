-- EPIC-088 (ES-11, MOM 29-10-2026): pengajuan cuti/izin berlampiran disetujui
-- atasan langsung lalu diverifikasi HR. Selama menunggu HR status tetap
-- 'pending' (kuota, bentrok, payroll, absensi tidak berubah); tahapnya di
-- attachment_review.
ALTER TABLE hris.leaves
  ADD COLUMN IF NOT EXISTS attachment_review text NULL,
  ADD COLUMN IF NOT EXISTS manager_approved_by uuid NULL REFERENCES hris.employees(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS manager_approved_name text NULL,
  ADD COLUMN IF NOT EXISTS manager_approved_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS attachment_reviewed_by uuid NULL,
  ADD COLUMN IF NOT EXISTS attachment_reviewed_name text NULL,
  ADD COLUMN IF NOT EXISTS attachment_reviewed_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS attachment_return_reason text NULL;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'leaves_attachment_review_check') THEN
    ALTER TABLE hris.leaves ADD CONSTRAINT leaves_attachment_review_check
      CHECK (attachment_review IN ('awaiting', 'returned', 'verified'));
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS leaves_attachment_review_pending_idx
  ON hris.leaves (created_at)
  WHERE status = 'pending' AND attachment_review IS NOT NULL;

COMMENT ON COLUMN hris.leaves.attachment_review IS
  'EPIC-088: NULL = tanpa tahap HR (menunggu atasan / tanpa lampiran); awaiting = atasan setuju, menunggu verifikasi lampiran HR; returned = HR mengembalikan, karyawan unggah ulang; verified = HR memverifikasi';
COMMENT ON COLUMN hris.leaves.attachment_reviewed_by IS
  'EPIC-088: configuration.users.id pemeriksa lampiran (HR bisa tanpa data karyawan)';
