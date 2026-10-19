-- Jadwal per tanggal hasil bulk upload Jadwal Kerja (Spec §2.4). shift_id
-- NULL = libur. Menang atas pola mingguan (hris.employee_shifts) di
-- tanggal yang sama; view v_employee_schedule menyatukan keduanya dalam
-- bentuk baris pola sehingga domain.ResolveScheduleRow tidak berubah:
-- baris per tanggal = pola satu hari (effective_from = effective_to = tanggal),
-- priority 0 dipakai sebagai pemenang bila effective_from sama.
CREATE TABLE IF NOT EXISTS hris.employee_shift_days (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id     uuid NOT NULL REFERENCES hris.employees(id) ON DELETE CASCADE,
  work_date       date NOT NULL,
  shift_id        uuid REFERENCES hris.shifts(id),
  branch_id       uuid REFERENCES configuration.branches(id),
  batch_id        uuid NOT NULL,
  created_by_name text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT employee_shift_days_unique UNIQUE (employee_id, work_date)
);
CREATE INDEX IF NOT EXISTS employee_shift_days_branch_date_idx
  ON hris.employee_shift_days (branch_id, work_date);

CREATE OR REPLACE VIEW hris.v_employee_schedule AS
  SELECT d.employee_id,
         EXTRACT(ISODOW FROM d.work_date)::int AS day_of_week,
         d.shift_id,
         d.work_date AS effective_from,
         d.work_date AS effective_to,
         0 AS priority
  FROM hris.employee_shift_days d
  UNION ALL
  SELECT es.employee_id, es.day_of_week, es.shift_id, es.effective_from, es.effective_to, 1
  FROM hris.employee_shifts es;
