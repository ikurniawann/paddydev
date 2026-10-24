-- Hapus SEMUA data demo Manual Book ESS dari DB TEST (lihat seed-demo.sql).
--   psql "$TEST_DB" -f cleanup-demo.sql
-- Aman diulang. Di akhir mencetak jumlah baris yang masih merujuk orang demo
-- (harus 0 semua) dan mengembalikan koordinat Paddy Store PVJ ke NULL.

\set ON_ERROR_STOP on

BEGIN;

CREATE TEMP TABLE demo_people ON COMMIT DROP AS
SELECT unnest(ARRAY[
  'de5a0000-0000-4000-8000-000000000001',  -- user Rina
  'de5a0000-0000-4000-8000-000000000002',  -- user Andi
  'de5a0000-0000-4000-8000-000000000011',  -- karyawan Rina
  'de5a0000-0000-4000-8000-000000000012',  -- karyawan Andi
  'de5a0000-0000-4000-8000-000000000021'   -- run payroll demo
]::uuid[]) AS id;

-- 1. Tabel yang diketahui (urutan anak → induk).
DELETE FROM hris.payroll_details WHERE payroll_run_id IN (SELECT id FROM demo_people) OR employee_id IN (SELECT id FROM demo_people);
DELETE FROM hris.payroll_runs WHERE id IN (SELECT id FROM demo_people);
DELETE FROM hris.attendance WHERE employee_id IN (SELECT id FROM demo_people);
DELETE FROM hris.leaves WHERE employee_id IN (SELECT id FROM demo_people);
DELETE FROM hris.leave_balances WHERE employee_id IN (SELECT id FROM demo_people);
DELETE FROM hris.loans WHERE employee_id IN (SELECT id FROM demo_people);
DELETE FROM hris.business_trips WHERE employee_id IN (SELECT id FROM demo_people);
DELETE FROM hris.overtime_requests WHERE employee_id IN (SELECT id FROM demo_people);
DELETE FROM hris.employee_salary WHERE employee_id IN (SELECT id FROM demo_people);
DELETE FROM hris.employee_shifts WHERE employee_id IN (SELECT id FROM demo_people);
DELETE FROM hris.employee_placements WHERE employee_id IN (SELECT id FROM demo_people);

-- 2. Sapu bersih: setiap kolom uuid di skema aplikasi yang berisi id demo
--    (log audit, sesi login, notifikasi, outbox, dsb. yang tercipta saat capture).
--    Diulang beberapa putaran karena urutan FK antar-tabel tidak diketahui.
DO $$
DECLARE
  c record;
  n bigint;
  pass int;
  left_over bigint;
BEGIN
  FOR pass IN 1..4 LOOP
    left_over := 0;
    FOR c IN
      SELECT col.table_schema, col.table_name, col.column_name
      FROM information_schema.columns col
      JOIN information_schema.tables t ON t.table_schema = col.table_schema AND t.table_name = col.table_name
      WHERE col.data_type = 'uuid' AND t.table_type = 'BASE TABLE'
        AND col.table_schema NOT IN ('pg_catalog', 'information_schema')
        AND NOT (col.table_schema = 'hris' AND col.table_name = 'employees')
        AND NOT (col.table_schema = 'auth' AND col.table_name = 'users')
        AND NOT (col.table_schema = 'configuration' AND col.table_name = 'users')
    LOOP
      BEGIN
        EXECUTE format('DELETE FROM %I.%I WHERE %I IN (SELECT id FROM demo_people)', c.table_schema, c.table_name, c.column_name);
        GET DIAGNOSTICS n = ROW_COUNT;
        IF n > 0 THEN RAISE NOTICE 'hapus % baris dari %.%(%)', n, c.table_schema, c.table_name, c.column_name; END IF;
      EXCEPTION WHEN foreign_key_violation THEN
        left_over := left_over + 1;
      END;
    END LOOP;
    EXIT WHEN left_over = 0;
  END LOOP;
END $$;

-- 3. Orang demo (karyawan dulu: Rina merujuk Andi lewat reporting_to).
DELETE FROM hris.employees WHERE id IN (SELECT id FROM demo_people) AND reporting_to IS NOT NULL;
DELETE FROM hris.employees WHERE id IN (SELECT id FROM demo_people);
DELETE FROM iam.user_roles WHERE user_id IN (SELECT id FROM demo_people);
DELETE FROM configuration.users WHERE id IN (SELECT id FROM demo_people);
DELETE FROM auth.users WHERE id IN (SELECT id FROM demo_people);
DELETE FROM auth.login_attempts WHERE account_key ILIKE '%.demo@example.com';

-- 4. Notifikasi yang dikirim trigger DB ke akun HR (karyawan baru, cuti,
--    absen, dihapus, …) — termasuk yang baru dibuat oleh penghapusan di atas.
--    Dibatasi ke tautan HRIS/karyawan supaya notifikasi lain tidak tersentuh.
DELETE FROM public.notifications
WHERE link ILIKE '%de5a0000-0000-4000-8000-%'
   OR ((message ILIKE '%Rina Lestari%' OR message ILIKE '%Andi Pratama%' OR title ILIKE '%Rina Lestari%' OR title ILIKE '%Andi Pratama%')
       AND (link ILIKE '/dashboard/hris/%' OR link ILIKE '/dashboard/employees%'));

-- 5. Koordinat toko yang hanya diisi sementara untuk foto popup "di luar area".
UPDATE configuration.branches SET lat = NULL, lng = NULL, geofence_radius_m = NULL WHERE code = 'PD-PVJ';

COMMIT;

-- 6. Verifikasi: tabel yang masih menyimpan id demo (harus kosong) dan total.
DO $$
DECLARE c record; n bigint; total bigint := 0;
BEGIN
  FOR c IN
    SELECT col.table_schema, col.table_name, col.column_name
    FROM information_schema.columns col
    JOIN information_schema.tables t ON t.table_schema = col.table_schema AND t.table_name = col.table_name
    WHERE col.data_type = 'uuid' AND t.table_type = 'BASE TABLE'
      AND col.table_schema NOT IN ('pg_catalog', 'information_schema')
  LOOP
    EXECUTE format('SELECT count(*) FROM %I.%I WHERE %I::text LIKE %L', c.table_schema, c.table_name, c.column_name, 'de5a0000-0000-4000-8000-%') INTO n;
    IF n > 0 THEN RAISE WARNING 'masih ada % baris demo di %.%(%)', n, c.table_schema, c.table_name, c.column_name; END IF;
    total := total + n;
  END LOOP;
  RAISE NOTICE 'Sisa baris demo (kolom uuid): %', total;
END $$;

SELECT
  (SELECT count(*) FROM auth.users WHERE email ILIKE '%.demo@example.com') AS akun_demo,
  (SELECT count(*) FROM hris.employees WHERE nip IN ('DM-901', 'DM-902')) AS karyawan_demo,
  (SELECT count(*) FROM hris.payroll_runs WHERE run_name = 'DEMO Manual ESS') AS payroll_demo,
  (SELECT count(*) FROM public.notifications WHERE link ILIKE '%de5a0000-0000-4000-8000-%'
     OR ((message ILIKE '%Rina Lestari%' OR message ILIKE '%Andi Pratama%')
         AND (link ILIKE '/dashboard/hris/%' OR link ILIKE '/dashboard/employees%'))) AS notifikasi_demo,
  (SELECT count(*) FROM configuration.branches WHERE code = 'PD-PVJ' AND (lat IS NOT NULL OR lng IS NOT NULL OR geofence_radius_m IS NOT NULL)) AS pvj_berkoordinat;
