-- =============================================================================
-- Pengajuan absen manual / koreksi absen (EPIC-065, MOM AB-04 + AB-06, K-9).
-- Karyawan mengajukan dari ESS dengan keterangan (dan foto bukti opsional);
-- HR memutus (matriks approval: "Absen Manual / Koreksi Absen | HR").
-- Persetujuan menulis hris.attendance lewat jalur penyesuaian HR yang sama
-- (riwayat hris.attendance_adjustments + audit) dan menautkan attendance_id.
-- Satu pengajuan pending per karyawan per tanggal.
-- =============================================================================

CREATE TABLE IF NOT EXISTS hris.attendance_requests (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id      uuid NOT NULL REFERENCES hris.employees(id) ON DELETE CASCADE,
  date             date NOT NULL,
  kind             text NOT NULL,
  clock_in         text,
  clock_out        text,
  reason           text NOT NULL,
  attachment_path  text,
  status           text NOT NULL DEFAULT 'pending',
  attendance_id    uuid REFERENCES hris.attendance(id) ON DELETE SET NULL,
  decided_by       uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  decided_by_name  text,
  decided_at       timestamptz,
  rejection_reason text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT attendance_requests_kind_check CHECK (kind IN ('manual', 'correction')),
  CONSTRAINT attendance_requests_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
  CONSTRAINT attendance_requests_clock_check CHECK (
    (clock_in IS NOT NULL OR clock_out IS NOT NULL)
    AND (clock_in IS NULL OR clock_in ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$')
    AND (clock_out IS NULL OR clock_out ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$')),
  CONSTRAINT attendance_requests_reason_check CHECK (length(btrim(reason)) >= 5)
);

CREATE UNIQUE INDEX IF NOT EXISTS attendance_requests_one_pending
  ON hris.attendance_requests (employee_id, date) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_attendance_requests_status ON hris.attendance_requests (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_attendance_requests_employee ON hris.attendance_requests (employee_id, date DESC);

DROP TRIGGER IF EXISTS attendance_requests_updated_at ON hris.attendance_requests;
CREATE TRIGGER attendance_requests_updated_at BEFORE UPDATE ON hris.attendance_requests
  FOR EACH ROW EXECUTE FUNCTION public.update_hris_updated_at();

COMMENT ON TABLE hris.attendance_requests IS
  'EPIC-065: pengajuan absen manual/koreksi karyawan; HR memutus, persetujuan menulis hris.attendance lewat penyesuaian HR.';
