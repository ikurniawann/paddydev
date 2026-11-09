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

-- Jadwal yang masih menunggu dibatalkan saat karyawan dinonaktifkan lewat jalur
-- apa pun (hapus, edit, akun, impor), supaya rehire sesudahnya tidak ikut
-- di-resign oleh job (review akhir C1). Job menandai applied_at sebelum
-- menjalankan resign, jadi trigger ini tidak membatalkan jadwal yang sedang
-- dijalankan. Nama tabel lengkap: role DB production tidak menyetel search_path.
CREATE OR REPLACE FUNCTION hris.cancel_scheduled_resign_on_deactivate() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  UPDATE hris.scheduled_resignations
  SET cancelled_at = now(), cancel_reason = 'Karyawan dinonaktifkan'
  WHERE employee_id = NEW.id AND applied_at IS NULL AND cancelled_at IS NULL;
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS trg_cancel_scheduled_resign ON hris.employees;
CREATE TRIGGER trg_cancel_scheduled_resign
  AFTER UPDATE OF is_active ON hris.employees
  FOR EACH ROW WHEN (OLD.is_active AND NOT NEW.is_active)
  EXECUTE FUNCTION hris.cancel_scheduled_resign_on_deactivate();
