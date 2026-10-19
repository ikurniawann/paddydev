-- Tipe jadwal karyawan (Spec §2.2): shift = ditugaskan lewat Jadwal Kerja,
-- non_shift = jadwal tetap Sen–Jum 09.00–18.00. Wajib untuk semua karyawan.
ALTER TABLE hris.employees
  ADD COLUMN IF NOT EXISTS schedule_type text NOT NULL DEFAULT 'shift';
ALTER TABLE hris.employees DROP CONSTRAINT IF EXISTS employees_schedule_type_check;
ALTER TABLE hris.employees ADD CONSTRAINT employees_schedule_type_check
  CHECK (schedule_type IN ('shift', 'non_shift'));
