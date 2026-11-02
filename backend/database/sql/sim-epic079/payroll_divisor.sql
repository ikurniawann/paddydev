-- EPIC-079 R7 — simulasi read-only pembagi payroll (hari terjadwal periode).
-- Membandingkan, per karyawan payroll periode itu, hari ber-shift dari pola
-- mingguan saja (hris.employee_shifts, perilaku sebelum EPIC-079) dengan
-- hris.v_employee_schedule (roster per tanggal menang atas pola mingguan).
-- Meniru domain.BuildInputRange: baris terpilih per tanggal = effective_from
-- terbaru lalu prioritas terkecil; bila jadwal baru mulai di tengah periode,
-- pola pertamanya dianggap berlaku sejak awal periode (roster per tanggal
-- tidak diperpanjang). Pembukaan jadwal karyawan resign (end_date) tidak
-- ditiru. hari_sebelum NULL = tanpa jadwal: payroll memakai fallback
-- (pembagi struktur / hari hadir / 20).
--
-- Pakai: psql "$DATABASE_URL" -v start=2026-10-21 -v end=2026-11-20 -f payroll_divisor.sql
-- Hanya menampilkan karyawan yang pembaginya berubah, dengan jumlah per departemen.
BEGIN READ ONLY;

WITH params AS (
  SELECT :'start'::date AS s, :'end'::date AS e
), emp AS (
  SELECT e.id, e.nip, e.full_name, e.schedule_type, COALESCE(d.name, '(tanpa departemen)') AS department
  FROM hris.employees e
  LEFT JOIN hris.departments d ON d.id = e.department_id, params p
  WHERE e.is_active = true OR e.end_date BETWEEN p.s AND p.e
), src AS (
  SELECT v.employee_id, v.day_of_week, v.shift_id, v.effective_from, v.effective_to, v.priority,
         (v.priority = 0 AND v.effective_to = v.effective_from) AS per_date
  FROM hris.v_employee_schedule v, params p
  WHERE v.employee_id IN (SELECT id FROM emp)
    AND v.effective_from <= p.e AND (v.effective_to IS NULL OR v.effective_to >= p.s)
), variant AS (
  SELECT 'sebelum' AS k, src.* FROM src WHERE priority = 1
  UNION ALL
  SELECT 'sesudah', src.* FROM src
), first_from AS (
  SELECT k, employee_id, min(effective_from) AS f FROM variant GROUP BY 1, 2
), extended AS (
  SELECT v.k, v.employee_id, v.day_of_week, v.shift_id, v.effective_to, v.priority,
         CASE WHEN v.effective_from = ff.f AND ff.f > p.s AND NOT v.per_date THEN p.s ELSE v.effective_from END AS eff_from
  FROM variant v JOIN first_from ff USING (k, employee_id), params p
), days AS (
  SELECT d::date AS day FROM params p, generate_series(p.s, p.e, interval '1 day') d
), resolved AS (
  SELECT DISTINCT ON (x.k, x.employee_id, dd.day) x.k, x.employee_id, dd.day, x.shift_id
  FROM extended x
  JOIN days dd ON x.day_of_week = EXTRACT(ISODOW FROM dd.day)::int
              AND x.eff_from <= dd.day AND (x.effective_to IS NULL OR x.effective_to >= dd.day)
  ORDER BY x.k, x.employee_id, dd.day, x.eff_from DESC, x.priority
), counted AS (
  SELECT e.*,
         CASE WHEN EXISTS (SELECT 1 FROM first_from f WHERE f.k = 'sebelum' AND f.employee_id = e.id)
              THEN (SELECT count(*) FROM resolved r WHERE r.k = 'sebelum' AND r.employee_id = e.id AND r.shift_id IS NOT NULL)
         END AS hari_sebelum,
         CASE WHEN EXISTS (SELECT 1 FROM first_from f WHERE f.k = 'sesudah' AND f.employee_id = e.id)
              THEN (SELECT count(*) FROM resolved r WHERE r.k = 'sesudah' AND r.employee_id = e.id AND r.shift_id IS NOT NULL)
         END AS hari_sesudah,
         (SELECT count(*) FROM src s WHERE s.employee_id = e.id AND s.per_date) AS baris_roster
  FROM emp e
), summary AS (
  SELECT c.*,
         hari_sebelum IS DISTINCT FROM hari_sesudah AS berubah,
         count(*) FILTER (WHERE hari_sebelum IS DISTINCT FROM hari_sesudah) OVER (PARTITION BY department) AS berubah_di_departemen,
         count(*) OVER (PARTITION BY department) AS karyawan_di_departemen
  FROM counted c
)
SELECT department, berubah_di_departemen, karyawan_di_departemen, nip, full_name, schedule_type,
       hari_sebelum, hari_sesudah, baris_roster
FROM summary
WHERE berubah
ORDER BY department, full_name;

ROLLBACK;
