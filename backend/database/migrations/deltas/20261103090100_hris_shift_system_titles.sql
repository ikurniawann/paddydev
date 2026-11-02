-- EPIC-079 R8: Host Live, Production dan CRM bekerja dengan sistem shift.
--   Karyawan aktif bertipe Non-Shift dengan jabatan cocok
--   `host live|production|crm` diubah menjadi Shift mulai hari deploy (WIB);
--   pola tetapnya dihentikan sehari sebelumnya (sama dengan Go
--   stopWeeklyPattern). Setelah migrasi mereka tanpa jadwal sampai HR/pengatur
--   membuat roster. Jabatan yang sama dikecualikan dari penerapan jadwal tetap
--   EPIC-078 (preview sasaran dan trigger pindah departemen) supaya tidak
--   dikembalikan ke jadwal tetap.

-- 1. Sasaran aturan jam kerja HQ/Operations: tambahkan jabatan sistem shift.
CREATE OR REPLACE FUNCTION hris.work_hours_rule_targets(p_from date)
RETURNS TABLE (employee_id uuid, full_name text, department text, job_title text, schedule_type text, is_ops boolean)
LANGUAGE sql STABLE AS $$
  SELECT e.id, e.full_name, d.name, p2.title, e.schedule_type, lower(btrim(d.name)) = 'warehouse'
  FROM hris.employees e
  JOIN hris.departments d ON d.id = e.department_id
  LEFT JOIN hris.positions p2 ON p2.id = e.job_title_id
  WHERE e.is_active
    AND ((lower(btrim(d.name)) = 'warehouse' AND e.schedule_type = 'non_shift'
        AND coalesce(p2.title, '') !~* '(host live|production|crm)')
      OR (e.schedule_type = 'shift'
        AND lower(btrim(d.name)) <> 'retail operational'
        AND coalesce(p2.title, '') !~* '(shopkeeper|pasaraya|crew experience|store supervisor|retail operation manager|event|customer service|host live|production|crm)'
        AND NOT (p2.title IS NULL AND e.full_name ~* '^(super admin|demo|paddy )')
        AND NOT EXISTS (SELECT 1 FROM hris.employee_shift_days sd
                        WHERE sd.employee_id = e.id AND sd.work_date >= p_from)
        AND NOT EXISTS (SELECT 1 FROM hris.employee_shifts es
                        WHERE es.employee_id = e.id AND (es.effective_to IS NULL OR es.effective_to >= p_from))))
  ORDER BY e.full_name
$$;

-- 2. Pindah departemen tidak memberi jadwal tetap pada jabatan sistem shift.
CREATE OR REPLACE FUNCTION hris.employee_department_fixed_week()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  was_ops boolean;
  now_ops boolean;
  label text;
  today date := (now() AT TIME ZONE 'Asia/Jakarta')::date;
BEGIN
  IF EXISTS (SELECT 1 FROM hris.positions p WHERE p.id = NEW.job_title_id AND p.title ~* '(host live|production|crm)') THEN
    RETURN NULL;
  END IF;
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

-- 3. Ubah ke Shift mulai p_from: pola mingguan dari p_from dihapus, yang masih
--    berjalan berakhir sehari sebelumnya; diaudit. Mengembalikan jumlah yang diubah.
CREATE OR REPLACE FUNCTION hris.apply_shift_system_titles(p_from date, p_actor text)
RETURNS integer LANGUAGE plpgsql AS $$
DECLARE
  r record;
  changed integer := 0;
BEGIN
  FOR r IN
    SELECT e.id, e.full_name
    FROM hris.employees e
    JOIN hris.positions p ON p.id = e.job_title_id
    WHERE e.is_active AND e.schedule_type = 'non_shift' AND p.title ~* '(host live|production|crm)'
    ORDER BY e.full_name
  LOOP
    UPDATE hris.employees e SET schedule_type = 'shift', updated_at = now() WHERE e.id = r.id;
    DELETE FROM hris.employee_shifts es WHERE es.employee_id = r.id AND es.effective_from >= p_from;
    UPDATE hris.employee_shifts es SET effective_to = p_from - 1
    WHERE es.employee_id = r.id AND (es.effective_to IS NULL OR es.effective_to >= p_from);
    INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, before, after, reason)
    VALUES (p_actor, 'employee.schedule_type', 'hris.employees', r.id::text, r.full_name,
      jsonb_build_object('schedule_type', 'non_shift'),
      jsonb_build_object('schedule_type', 'shift', 'effective_from', p_from),
      'Sistem shift: Host Live/Produksi/CRM (EPIC-079)');
    changed := changed + 1;
  END LOOP;
  RETURN changed;
END $$;

-- 4. Penerapan sekali, mulai hari ini (WIB).
SELECT hris.apply_shift_system_titles((now() AT TIME ZONE 'Asia/Jakarta')::date, 'Sistem (sistem shift EPIC-079)') AS converted;
