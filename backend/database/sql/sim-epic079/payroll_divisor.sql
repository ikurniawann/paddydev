-- EPIC-079 R7 — simulasi read-only pembagi payroll (hari terjadwal periode).
-- Membandingkan, per karyawan payroll periode itu, hari ber-shift dari pola
-- mingguan saja (hris.employee_shifts, perilaku sebelum EPIC-079) dengan
-- hris.v_employee_schedule (roster per tanggal menang atas pola mingguan).
-- Meniru domain.BuildInputRange: baris terpilih per tanggal = effective_from
-- terbaru lalu prioritas terkecil; pola mingguan yang berakhir tepat pada
-- end_date karyawan resign dianggap terus berlaku; bila jadwal baru mulai di
-- tengah periode, pola pertamanya dianggap berlaku sejak awal periode (roster
-- per tanggal tidak diperpanjang). Kolom sebelum meniru kode lama: pola
-- mingguan saja, jalur jadwal setiap kali ada hari ber-shift (tanpa syarat
-- kelengkapan). Kolom sesudah: pembagi dari jadwal hanya bila ada hari
-- ber-shift dan setiap tanggal periode punya baris yang berlaku. hari_* NULL =
-- tanpa hari ber-shift (sebelum) atau tanpa jadwal / jadwal tidak lengkap
-- (sesudah): payroll memakai fallback (pembagi struktur / hari hadir / 20).
--
-- Pakai: psql "$DATABASE_URL" -v start=2026-10-21 -v end=2026-11-20 -f payroll_divisor.sql
-- Query 1 menampilkan karyawan yang pembaginya berubah, dengan jumlah per departemen.
-- Query 2 (R1) daftar staf shift di luar toko yang kehilangan tanggal merah.
-- Query 3 daftar sasaran aturan jabatan sistem shift dan penempatan utamanya.
BEGIN READ ONLY;

WITH params AS (
  SELECT :'start'::date AS s, :'end'::date AS e
), emp AS (
  SELECT e.id, e.nip, e.full_name, e.schedule_type, COALESCE(d.name, '(tanpa departemen)') AS department,
         CASE WHEN NOT e.is_active AND e.end_date >= p.s AND e.end_date < p.e THEN e.end_date END AS leaver_end
  FROM hris.employees e
  LEFT JOIN hris.departments d ON d.id = e.department_id, params p
  WHERE e.is_active = true OR e.end_date BETWEEN p.s AND p.e
), src AS (
  SELECT v.employee_id, v.day_of_week, v.shift_id, v.effective_from,
         CASE WHEN v.effective_to = emp.leaver_end AND NOT (v.priority = 0 AND v.effective_to = v.effective_from)
              THEN NULL ELSE v.effective_to END AS effective_to,
         v.priority, (v.priority = 0 AND v.effective_to = v.effective_from) AS per_date
  FROM hris.v_employee_schedule v JOIN emp ON emp.id = v.employee_id, params p
  WHERE v.effective_from <= p.e AND (v.effective_to IS NULL OR v.effective_to >= p.s)
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
), complete AS (
  -- Jadwal lengkap: setiap tanggal periode punya baris yang berlaku (hari apa pun).
  SELECT k, employee_id FROM (
    SELECT x.k, x.employee_id, count(DISTINCT dd.day) AS n
    FROM extended x JOIN days dd ON x.eff_from <= dd.day AND (x.effective_to IS NULL OR x.effective_to >= dd.day)
    GROUP BY 1, 2) c
  WHERE n = (SELECT count(*) FROM days)
), counted AS (
  SELECT e.*,
         -- Kode lama: pola mingguan saja, jalur jadwal setiap kali ada hari
         -- ber-shift (scheduled > 0), tanpa syarat jadwal lengkap.
         NULLIF((SELECT count(*) FROM resolved r WHERE r.k = 'sebelum' AND r.employee_id = e.id AND r.shift_id IS NOT NULL), 0) AS hari_sebelum,
         CASE WHEN EXISTS (SELECT 1 FROM complete c WHERE c.k = 'sesudah' AND c.employee_id = e.id)
              THEN NULLIF((SELECT count(*) FROM resolved r WHERE r.k = 'sesudah' AND r.employee_id = e.id AND r.shift_id IS NOT NULL), 0)
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

-- Query 2 — R1 blast radius: karyawan aktif bertipe Shift di luar departemen
-- Retail Operational (atau tanpa departemen) dengan jabatan di luar daftar
-- sistem shift. Mereka tidak lagi libur di tanggal merah (R1); HR perlu
-- konfirmasi sebelum 24/25-12. Kolom jadwal: pola mingguan / sel per tanggal
-- yang berlaku mulai :start.
SELECT COALESCE(d.name, '(tanpa departemen)') AS department, COALESCE(p.title, '(tanpa jabatan)') AS jabatan,
       e.nip, e.full_name,
       EXISTS (SELECT 1 FROM hris.employee_shifts es
               WHERE es.employee_id = e.id AND (es.effective_to IS NULL OR es.effective_to >= :'start'::date)) AS punya_pola_mingguan,
       EXISTS (SELECT 1 FROM hris.employee_shift_days sd
               WHERE sd.employee_id = e.id AND sd.work_date >= :'start'::date) AS punya_sel_per_tanggal
FROM hris.employees e
LEFT JOIN hris.departments d ON d.id = e.department_id
LEFT JOIN hris.positions p ON p.id = e.job_title_id
WHERE e.is_active AND e.schedule_type = 'shift'
  AND lower(btrim(COALESCE(d.name, ''))) <> 'retail operational'
  AND COALESCE(p.title, '') !~* '(host live|production|crm|shopkeeper|pasaraya|crew experience|store supervisor|retail operation manager|event|customer service)'
ORDER BY 1, 2, e.full_name;

-- Query 3 — sasaran aturan jabatan sistem shift (host live, production, crm):
-- tipe jadwal sekarang dan penempatan utama yang berlaku pada :start
-- (penempatan NULL = tidak bisa ditempatkan di grid jadwal toko).
SELECT COALESCE(d.name, '(tanpa departemen)') AS department, p.title AS jabatan, e.nip, e.full_name, e.schedule_type,
       (SELECT b.name FROM hris.employee_placements pl JOIN configuration.branches b ON b.id = pl.branch_id
        WHERE pl.employee_id = e.id AND pl.is_primary
          AND pl.start_date <= :'start'::date AND (pl.end_date IS NULL OR pl.end_date >= :'start'::date)
        LIMIT 1) AS penempatan_utama
FROM hris.employees e
LEFT JOIN hris.departments d ON d.id = e.department_id
JOIN hris.positions p ON p.id = e.job_title_id
WHERE e.is_active AND p.title ~* '(host live|production|crm)'
ORDER BY 1, 2, e.full_name;

ROLLBACK;
