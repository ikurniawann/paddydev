-- =============================================================================
-- Hak akses HRIS per akun email (EPIC-057, keputusan user 19-10-2026).
-- Jalankan SETELAH migrasi 20261019100000 dan backup:
--   psql "$U" -v ON_ERROR_STOP=1 -f backend/database/sql/fixes/2026-10-21_hris_akses_per_akun.sql
-- Peran aplikasi tetap hrd. Jangan simpan ulang akun ini di Manajemen User
-- dengan peran lain: mengganti peran aplikasi mengganti role IAM (syncIamRole).
-- =============================================================================
BEGIN;
CREATE TEMP VIEW hr_akses AS
SELECT * FROM (VALUES
  ('paddyhumanresources@gmail.com', 'hris_manager'),
  ('paddy.peopledevelopment@gmail.com', 'hris_non_payroll'),
  ('paddyhroperation@gmail.com', 'hris_non_rekrutmen_kpi')) v(email, role_code);

-- Role turunan mengikuti grant admin_hris terbaru sebelum dipasang.
SELECT iam.sync_hris_staff_roles();

SELECT (SELECT count(*) FROM hr_akses a
        JOIN auth.users u ON lower(u.email) = a.email
        JOIN iam.roles r ON r.code = a.role_code AND r.deleted_at IS NULL) = 3
   AND (SELECT count(DISTINCT r.code) FROM iam.roles r
        JOIN iam.role_menu_permissions p ON p.role_id = r.id AND p.is_active
        WHERE r.code IN (SELECT role_code FROM hr_akses)) = 3 AS ok \gset
\if :ok
\else
  -- Gagal dengan kode error (psql keluar 3 dengan ON_ERROR_STOP); transaksi batal.
  DO $$ BEGIN RAISE EXCEPTION 'EPIC-057: akun atau role (dengan grant aktif) tidak lengkap — tidak ada perubahan'; END $$;
\endif

\echo 'Sebelum:'
SELECT a.email, string_agg(r.code, ', ') AS roles FROM hr_akses a
JOIN auth.users u ON lower(u.email) = a.email
LEFT JOIN iam.user_roles ur ON ur.user_id = u.id
LEFT JOIN iam.roles r ON r.id = ur.role_id GROUP BY a.email ORDER BY a.email;

DELETE FROM iam.user_roles ur USING hr_akses a, auth.users u
WHERE lower(u.email) = a.email AND ur.user_id = u.id;
INSERT INTO iam.user_roles (user_id, role_id, is_primary)
SELECT u.id, r.id, true FROM hr_akses a
JOIN auth.users u ON lower(u.email) = a.email
JOIN iam.roles r ON r.code = a.role_code;

\echo 'Sesudah:'
SELECT a.email, r.code, cu.role AS app_role FROM hr_akses a
JOIN auth.users u ON lower(u.email) = a.email
JOIN iam.user_roles ur ON ur.user_id = u.id
JOIN iam.roles r ON r.id = ur.role_id
JOIN configuration.users cu ON cu.id = u.id ORDER BY a.email;
COMMIT;
