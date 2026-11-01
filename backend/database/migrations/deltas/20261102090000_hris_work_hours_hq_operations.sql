-- EPIC-078: jam kerja HQ dan Operations (TnC "Hari dan Jam Kerja", 02-11-2026).
--   HQ (semua staf kantor pusat): Sen–Jum masuk 08.00–09.00, istirahat
--     12.00–13.00, pulang 17.00–18.00 → shift NS 09.00–18.00 tanpa toleransi:
--     terlambat bila masuk lewat 09.00.
--   Operations (seluruh staf Warehouse): Sen–Jum 08.00–16.00 (istirahat
--     12.00–13.00), Sabtu 08.00–14.00 → shift OPS / OPSSAT, toleransi 10 menit
--     (keputusan user 02-11).
-- Jadwal tetap Non-Shift mengikuti departemen (Warehouse → Operations) —
-- satu sumber: hris.apply_fixed_week, dipakai Go (applyFixedSchedule), trigger
-- pindah departemen, dan penerapan sekali saat deploy (mulai hari ini WIB).
-- Absensi yang sudah tercatat tidak dihitung ulang.

-- 1. Toleransi yang berlaku saat clock-in disimpan per baris, supaya koreksi
--    HR atas absen lama tetap memakai toleransi lama (NULL = baris lama,
--    pakai toleransi shift). Baris NS yang sudah ada dibekukan di 15 menit.
ALTER TABLE hris.attendance ADD COLUMN IF NOT EXISTS late_tolerance_minutes integer
  CHECK (late_tolerance_minutes IS NULL OR late_tolerance_minutes >= 0);
COMMENT ON COLUMN hris.attendance.late_tolerance_minutes IS
  'Toleransi terlambat shift saat clock-in (EPIC-078); NULL = pakai toleransi shift sekarang';
SET LOCAL hris.allow_closed_period_write = 'on';
UPDATE hris.attendance a SET late_tolerance_minutes = s.late_tolerance_minutes
FROM hris.shifts s
WHERE s.id = a.shift_id AND upper(s.code) = 'NS' AND a.late_tolerance_minutes IS NULL;
RESET hris.allow_closed_period_write;

-- 2. Shift bawaan.
UPDATE hris.shifts
SET late_tolerance_minutes = 0,
    name = CASE WHEN name = 'Non-Shift 09.00–18.00' THEN 'Non-Shift HQ (masuk 08.00–09.00)' ELSE name END,
    updated_at = now()
WHERE upper(code) = 'NS';

UPDATE hris.shifts SET start_time = '08:00', end_time = '16:00', break_minutes = 60, late_tolerance_minutes = 10,
  is_overnight = false, is_active = true, updated_at = now()
WHERE upper(code) = 'OPS';
INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes, is_overnight, sort_order)
SELECT 'OPS', 'Operations Sen–Jum 08.00–16.00', '08:00', '16:00', 60, 10, false, 2
WHERE NOT EXISTS (SELECT 1 FROM hris.shifts WHERE upper(code) = 'OPS');

UPDATE hris.shifts SET start_time = '08:00', end_time = '14:00', break_minutes = 0, late_tolerance_minutes = 10,
  is_overnight = false, is_active = true, updated_at = now()
WHERE upper(code) = 'OPSSAT';
INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes, is_overnight, sort_order)
SELECT 'OPSSAT', 'Operations Sabtu 08.00–14.00', '08:00', '14:00', 0, 10, false, 3
WHERE NOT EXISTS (SELECT 1 FROM hris.shifts WHERE upper(code) = 'OPSSAT');

-- 3. apply_fixed_week gives the employee the fixed week of their department
--    from p_from: per-date rows from p_from removed, weekly rows starting on
--    or after it dropped, running ones ended the day before; Warehouse →
--    Sen–Jum OPS, Sab OPSSAT, Min libur; else Sen–Jum NS, Sab–Min libur.
--    Returns the pattern label (audited by the caller).
CREATE OR REPLACE FUNCTION hris.apply_fixed_week(p_employee uuid, p_from date, p_actor text)
RETURNS text LANGUAGE plpgsql AS $$
DECLARE
  is_ops boolean;
  weekday uuid;
  saturday uuid;
BEGIN
  SELECT coalesce(lower(btrim(d.name)) = 'warehouse', false) INTO is_ops
  FROM hris.employees e LEFT JOIN hris.departments d ON d.id = e.department_id
  WHERE e.id = p_employee;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'Karyawan % tidak ditemukan', p_employee;
  END IF;
  IF is_ops THEN
    SELECT s.id INTO weekday FROM hris.shifts s WHERE upper(s.code) = 'OPS';
    SELECT s.id INTO saturday FROM hris.shifts s WHERE upper(s.code) = 'OPSSAT';
  ELSE
    SELECT s.id INTO weekday FROM hris.shifts s WHERE upper(s.code) = 'NS';
  END IF;
  IF weekday IS NULL OR (is_ops AND saturday IS NULL) THEN
    RAISE EXCEPTION 'Shift bawaan jadwal tetap (NS/OPS/OPSSAT) tidak ada';
  END IF;
  DELETE FROM hris.employee_shift_days sd WHERE sd.employee_id = p_employee AND sd.work_date >= p_from;
  DELETE FROM hris.employee_shifts es WHERE es.employee_id = p_employee AND es.effective_from >= p_from;
  UPDATE hris.employee_shifts es SET effective_to = p_from - 1
  WHERE es.employee_id = p_employee AND es.effective_from < p_from
    AND (es.effective_to IS NULL OR es.effective_to >= p_from);
  INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from, created_by_name)
  SELECT p_employee, dow, CASE WHEN dow <= 5 THEN weekday WHEN dow = 6 THEN saturday END, p_from, p_actor
  FROM generate_series(1, 7) dow;
  RETURN CASE WHEN is_ops
    THEN 'Operations: Sen–Jum OPS 08.00–16.00, Sab OPSSAT 08.00–14.00, Min libur'
    ELSE 'HQ: Sen–Jum NS (masuk 08.00–09.00), Sab–Min libur' END;
END $$;

-- 4. work_hours_rule_targets (read-only preview): active Warehouse Non-Shift
--    staff, and active Shift staff with no schedule from p_from on — except
--    store crews (department Retail Operational), shift-work titles (the
--    21-10 go-live exclusions plus event and Customer Service, which run on
--    CS shifts) and system accounts. Staff without a department are left to HR.
CREATE OR REPLACE FUNCTION hris.work_hours_rule_targets(p_from date)
RETURNS TABLE (employee_id uuid, full_name text, department text, job_title text, schedule_type text, is_ops boolean)
LANGUAGE sql STABLE AS $$
  SELECT e.id, e.full_name, d.name, p2.title, e.schedule_type, lower(btrim(d.name)) = 'warehouse'
  FROM hris.employees e
  JOIN hris.departments d ON d.id = e.department_id
  LEFT JOIN hris.positions p2 ON p2.id = e.job_title_id
  WHERE e.is_active
    AND ((lower(btrim(d.name)) = 'warehouse' AND e.schedule_type = 'non_shift')
      OR (e.schedule_type = 'shift'
        AND lower(btrim(d.name)) <> 'retail operational'
        AND coalesce(p2.title, '') !~* '(shopkeeper|pasaraya|crew experience|store supervisor|retail operation manager|event|customer service)'
        AND NOT (p2.title IS NULL AND e.full_name ~* '^(super admin|demo|paddy )')
        AND NOT EXISTS (SELECT 1 FROM hris.employee_shift_days sd
                        WHERE sd.employee_id = e.id AND sd.work_date >= p_from)
        AND NOT EXISTS (SELECT 1 FROM hris.employee_shifts es
                        WHERE es.employee_id = e.id AND (es.effective_to IS NULL OR es.effective_to >= p_from))))
  ORDER BY e.full_name
$$;

-- 5. apply_work_hours_rule applies the fixed week to every target, turning
--    Shift targets into Non-Shift, and audits both.
CREATE OR REPLACE FUNCTION hris.apply_work_hours_rule(p_from date, p_actor text)
RETURNS TABLE (employee_id uuid, full_name text, pattern text, type_changed boolean)
LANGUAGE plpgsql AS $$
DECLARE
  r record;
BEGIN
  FOR r IN SELECT * FROM hris.work_hours_rule_targets(p_from) t LOOP
    employee_id := r.employee_id;
    full_name := r.full_name;
    type_changed := r.schedule_type = 'shift';
    IF type_changed THEN
      UPDATE hris.employees e SET schedule_type = 'non_shift', updated_at = now() WHERE e.id = r.employee_id;
      INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, before, after, reason)
      VALUES (p_actor, 'employee.schedule_type', 'hris.employees', r.employee_id::text, r.full_name,
        jsonb_build_object('schedule_type', 'shift'),
        jsonb_build_object('schedule_type', 'non_shift', 'effective_from', p_from), 'Aturan jam kerja HQ/Operations (EPIC-078)');
    END IF;
    pattern := hris.apply_fixed_week(r.employee_id, p_from, p_actor);
    INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, after, reason)
    VALUES (p_actor, 'schedule.fixed_apply', 'hris.employees', r.employee_id::text, r.full_name,
      jsonb_build_object('effective_from', p_from, 'pattern', pattern), 'Aturan jam kerja HQ/Operations (EPIC-078)');
    RETURN NEXT;
  END LOOP;
END $$;

-- 6. A Non-Shift employee moved into or out of Warehouse gets the other
--    fixed week from today (WIB), whichever path changed the department.
CREATE OR REPLACE FUNCTION hris.employee_department_fixed_week()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  was_ops boolean;
  now_ops boolean;
  label text;
  today date := (now() AT TIME ZONE 'Asia/Jakarta')::date;
BEGIN
  SELECT coalesce(bool_or(lower(btrim(d.name)) = 'warehouse'), false) INTO was_ops FROM hris.departments d WHERE d.id = OLD.department_id;
  SELECT coalesce(bool_or(lower(btrim(d.name)) = 'warehouse'), false) INTO now_ops FROM hris.departments d WHERE d.id = NEW.department_id;
  IF was_ops IS DISTINCT FROM now_ops THEN
    label := hris.apply_fixed_week(NEW.id, today, 'Sistem (pindah departemen)');
    INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, after, reason)
    VALUES ('Sistem (pindah departemen)', 'schedule.fixed_apply', 'hris.employees', NEW.id::text, NEW.full_name,
      jsonb_build_object('effective_from', today, 'pattern', label), 'Pindah departemen (EPIC-078)');
  END IF;
  RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS employees_department_fixed_week ON hris.employees;
CREATE TRIGGER employees_department_fixed_week
AFTER UPDATE OF department_id ON hris.employees
FOR EACH ROW
WHEN (NEW.schedule_type = 'non_shift' AND NEW.is_active AND OLD.department_id IS DISTINCT FROM NEW.department_id)
EXECUTE FUNCTION hris.employee_department_fixed_week();

-- 7. Penerapan sekali, mulai hari ini (WIB).
SELECT count(*) AS applied
FROM hris.apply_work_hours_rule((now() AT TIME ZONE 'Asia/Jakarta')::date, 'Sistem (aturan jam kerja EPIC-078)');
