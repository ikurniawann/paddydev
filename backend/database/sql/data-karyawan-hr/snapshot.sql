-- EPIC-096: snapshot read-only untuk generate.py, satu dokumen JSON.
--   psql -X -tA -v today=YYYY-MM-DD -f snapshot.sql > snapshot.json
SET default_transaction_read_only = on;
SELECT json_build_object(
  'employees', (SELECT coalesce(json_agg(x ORDER BY x.nip), '[]') FROM (
    SELECT e.id, e.nip, e.full_name, e.is_active, e.join_date, e.employment_status, e.schedule_type,
           e.department_id, d.name AS department, e.job_title_id, p.title AS position, e.reporting_to,
           (SELECT b.code FROM hris.employee_placements pl JOIN configuration.branches b ON b.id = pl.branch_id
             WHERE pl.employee_id = e.id AND pl.is_primary AND pl.start_date <= :'today'::date
               AND (pl.end_date IS NULL OR pl.end_date >= :'today'::date)
             ORDER BY pl.start_date DESC LIMIT 1) AS branch_code,
           (SELECT max(sd.work_date) FROM hris.employee_shift_days sd
             WHERE sd.employee_id = e.id AND sd.work_date >= :'today'::date) AS roster_last
    FROM hris.employees e
    LEFT JOIN hris.departments d ON d.id = e.department_id
    LEFT JOIN hris.positions p ON p.id = e.job_title_id) x),
  'branches', (SELECT json_agg(json_build_object('code', upper(btrim(code)), 'active', is_active)) FROM configuration.branches
               WHERE code IS NOT NULL AND btrim(code) <> ''),
  'departments', (SELECT json_agg(json_build_object('id', id, 'name', name, 'active', coalesce(is_active, true))) FROM hris.departments),
  'positions', (SELECT json_agg(json_build_object('id', id, 'title', title, 'level', level, 'department', department,
                'active', coalesce(is_active, true))) FROM hris.positions),
  'delegate_branches', (SELECT coalesce(json_agg(json_build_object('employee_id', sdb.employee_id, 'code', upper(btrim(b.code)))), '[]')
               FROM hris.schedule_delegate_branches sdb JOIN configuration.branches b ON b.id = sdb.branch_id)
);
