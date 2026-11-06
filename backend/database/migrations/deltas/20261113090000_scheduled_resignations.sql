-- EPIC-095: resign terjadwal. Tanggal efektif (hari terakhir kerja) di masa depan
-- disimpan sebagai jadwal; job hris (ScheduledResignLoop) menjalankan resign penuh
-- sehari setelah tanggal itu. Satu jadwal aktif per karyawan. Jadwal yang batal atau
-- sudah dijalankan tetap tersimpan sebagai riwayat.
CREATE TABLE IF NOT EXISTS hris.scheduled_resignations (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id    uuid NOT NULL REFERENCES hris.employees(id) ON DELETE CASCADE,
  effective_date date NOT NULL,
  resign_type    text NOT NULL CHECK (resign_type IN ('resign', 'terminated', 'end_of_contract')),
  reason         text,
  created_by     uuid REFERENCES configuration.users(id) ON DELETE SET NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  applied_at     timestamptz,
  cancelled_at   timestamptz,
  cancelled_by   uuid REFERENCES configuration.users(id) ON DELETE SET NULL,
  cancel_reason  text,
  CHECK (applied_at IS NULL OR cancelled_at IS NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS scheduled_resignations_one_pending
  ON hris.scheduled_resignations (employee_id)
  WHERE applied_at IS NULL AND cancelled_at IS NULL;

CREATE INDEX IF NOT EXISTS scheduled_resignations_due
  ON hris.scheduled_resignations (effective_date)
  WHERE applied_at IS NULL AND cancelled_at IS NULL;

COMMENT ON TABLE hris.scheduled_resignations IS
  'Resign terjadwal (EPIC-095): dijalankan otomatis sehari setelah effective_date (hari terakhir kerja).';
