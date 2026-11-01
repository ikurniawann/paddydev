-- EPIC-078: jam kerja HQ dan Operations (TnC "Hari dan Jam Kerja", 02-11-2026).
--   HQ (semua staf kantor pusat): Sen–Jum masuk 08.00–09.00, istirahat
--     12.00–13.00, pulang 17.00–18.00 → shift NS 09.00–18.00 tanpa toleransi:
--     terlambat bila masuk lewat 09.00.
--   Operations (seluruh staf Warehouse): Sen–Jum 08.00–16.00 (istirahat
--     12.00–13.00), Sabtu 08.00–14.00 → shift OPS / OPSSAT, toleransi 10 menit
--     (keputusan user 02-11).
-- Jadwal tetap Non-Shift kini mengikuti departemen (Warehouse → Operations).
-- Penerapan sekali saat deploy, mulai hari ini (WIB): staf Warehouse Non-Shift
-- dan staf HQ/Warehouse bertipe Shift tanpa jadwal mendapat jadwal tetapnya.
-- Absensi yang sudah tercatat tidak dihitung ulang.

UPDATE hris.shifts
SET name = 'Non-Shift HQ (masuk 08.00–09.00)', late_tolerance_minutes = 0, updated_at = now()
WHERE upper(code) = 'NS';

INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes, is_overnight, sort_order)
SELECT 'OPS', 'Operations Sen–Jum 08.00–16.00', '08:00', '16:00', 60, 10, false, 2
WHERE NOT EXISTS (SELECT 1 FROM hris.shifts WHERE upper(code) = 'OPS');

INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes, is_overnight, sort_order)
SELECT 'OPSSAT', 'Operations Sabtu 08.00–14.00', '08:00', '14:00', 0, 10, false, 3
WHERE NOT EXISTS (SELECT 1 FROM hris.shifts WHERE upper(code) = 'OPSSAT');

-- apply_work_hours_rule gives every target its fixed weekly pattern from
-- p_from (per-date rows from p_from removed, running weekly rows ended the
-- day before), as applyFixedSchedule does, and audits it. Targets: active
-- Warehouse Non-Shift staff, and active Shift staff of any department but
-- Retail Operational (store crews) with no schedule from p_from on — they
-- become Non-Shift. HQ Non-Shift staff already on NS are left alone.
CREATE OR REPLACE FUNCTION hris.apply_work_hours_rule(p_from date, p_actor text)
RETURNS TABLE (employee_id uuid, full_name text, pattern text, type_changed boolean)
LANGUAGE plpgsql AS $$
DECLARE
  ns uuid;
  ops uuid;
  opssat uuid;
  r record;
BEGIN
  SELECT s.id INTO ns FROM hris.shifts s WHERE upper(s.code) = 'NS';
  SELECT s.id INTO ops FROM hris.shifts s WHERE upper(s.code) = 'OPS';
  SELECT s.id INTO opssat FROM hris.shifts s WHERE upper(s.code) = 'OPSSAT';
  FOR r IN
    SELECT e.id, e.full_name AS name, e.schedule_type, lower(btrim(d.name)) = 'warehouse' AS is_ops
    FROM hris.employees e
    JOIN hris.departments d ON d.id = e.department_id
    WHERE e.is_active
      AND lower(btrim(d.name)) <> 'retail operational'
      AND ((lower(btrim(d.name)) = 'warehouse' AND e.schedule_type = 'non_shift')
        OR (e.schedule_type = 'shift'
          AND NOT EXISTS (SELECT 1 FROM hris.employee_shift_days sd
                          WHERE sd.employee_id = e.id AND sd.work_date >= p_from)
          AND NOT EXISTS (SELECT 1 FROM hris.employee_shifts es
                          WHERE es.employee_id = e.id AND (es.effective_to IS NULL OR es.effective_to >= p_from))))
    ORDER BY e.full_name
  LOOP
    DELETE FROM hris.employee_shift_days sd WHERE sd.employee_id = r.id AND sd.work_date >= p_from;
    DELETE FROM hris.employee_shifts es WHERE es.employee_id = r.id AND es.effective_from >= p_from;
    UPDATE hris.employee_shifts es SET effective_to = p_from - 1
    WHERE es.employee_id = r.id AND es.effective_from < p_from
      AND (es.effective_to IS NULL OR es.effective_to >= p_from);
    INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from, created_by_name)
    SELECT r.id, dow,
      CASE WHEN dow <= 5 THEN CASE WHEN r.is_ops THEN ops ELSE ns END
           WHEN dow = 6 AND r.is_ops THEN opssat END,
      p_from, p_actor
    FROM generate_series(1, 7) dow;

    employee_id := r.id;
    full_name := r.name;
    pattern := CASE WHEN r.is_ops
      THEN 'Operations: Sen–Jum OPS 08.00–16.00, Sab OPSSAT 08.00–14.00, Min libur'
      ELSE 'HQ: Sen–Jum NS (masuk 08.00–09.00), Sab–Min libur' END;
    type_changed := r.schedule_type = 'shift';
    IF type_changed THEN
      UPDATE hris.employees e SET schedule_type = 'non_shift', updated_at = now() WHERE e.id = r.id;
      INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, before, after, reason)
      VALUES (p_actor, 'employee.schedule_type', 'hris.employees', r.id::text, r.name,
        jsonb_build_object('schedule_type', 'shift'),
        jsonb_build_object('schedule_type', 'non_shift', 'effective_from', p_from), 'Aturan jam kerja HQ/Operations (EPIC-078)');
    END IF;
    INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, after, reason)
    VALUES (p_actor, 'schedule.fixed_apply', 'hris.employees', r.id::text, r.name,
      jsonb_build_object('effective_from', p_from, 'pattern', pattern), 'Aturan jam kerja HQ/Operations (EPIC-078)');
    RETURN NEXT;
  END LOOP;
END $$;

SELECT count(*) AS applied
FROM hris.apply_work_hours_rule((now() AT TIME ZONE 'Asia/Jakarta')::date, 'Sistem (aturan jam kerja EPIC-078)');
