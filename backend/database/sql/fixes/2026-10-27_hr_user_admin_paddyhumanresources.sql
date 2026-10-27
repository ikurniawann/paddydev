-- Beri paddyhumanresources@gmail.com (hris_manager) role tambahan hr_user_admin:
-- Manajemen User baca + ubah, supaya bisa Buat Akun Login dan Reset Password
-- karyawan, seperti paddy.peopledevelopment (23-10-2026). Tanpa "create":
-- tidak menambah karyawan baru dari Manajemen User.
--
-- Efek samping: akun dengan role tambahan bukan lagi "akun HR" di halaman
-- Hak Akses HRIS, jadi role HRIS-nya hanya bisa diganti Super Admin.
-- iam.sync_hris_staff_roles() tidak menyentuh iam.user_roles.
--
-- Pakai:
--   psql "$U" -v ON_ERROR_STOP=1 -v preview=1 -f <file>   -- pratinjau, tidak menulis
--   psql "$U" -v ON_ERROR_STOP=1 -f <file>                -- terapkan
-- Rollback:
--   DELETE FROM iam.user_roles WHERE is_primary = false
--     AND user_id = (SELECT id FROM auth.users WHERE lower(email) = 'paddyhumanresources@gmail.com')
--     AND role_id = (SELECT id FROM iam.roles WHERE code = 'hr_user_admin');

BEGIN;
\if :{?preview}
SET TRANSACTION READ ONLY;
\endif

DO $$
DECLARE
  v_user uuid;
  v_role uuid;
  v_app_role text;
  v_actions jsonb;
  v_before text[];
  v_after text[];
BEGIN
  SELECT id INTO v_user FROM auth.users WHERE lower(email) = 'paddyhumanresources@gmail.com';
  IF v_user IS NULL THEN
    RAISE EXCEPTION 'Akun paddyhumanresources@gmail.com tidak ditemukan';
  END IF;

  SELECT role INTO v_app_role FROM configuration.users WHERE id = v_user;
  IF v_app_role IS DISTINCT FROM 'hrd' THEN
    RAISE EXCEPTION 'Peran aplikasi akun = %, harus hrd', v_app_role;
  END IF;

  SELECT id INTO v_role FROM iam.roles WHERE code = 'hr_user_admin' AND deleted_at IS NULL;
  IF v_role IS NULL THEN
    RAISE EXCEPTION 'Role hr_user_admin tidak ditemukan';
  END IF;

  -- Role-nya harus persis Manajemen User baca + ubah, tidak lebih.
  SELECT jsonb_agg(m.code || ':' || p.granted_actions::text ORDER BY m.code) INTO v_actions
    FROM iam.role_menu_permissions p JOIN iam.menus m ON m.id = p.menu_id
   WHERE p.role_id = v_role AND p.is_active;
  IF v_actions IS DISTINCT FROM '["settings.users:[\"read\", \"update\"]"]'::jsonb THEN
    RAISE EXCEPTION 'Grant hr_user_admin tidak sesuai: %', v_actions;
  END IF;

  SELECT array_agg(r.code ORDER BY ur.is_primary DESC, r.code) INTO v_before
    FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
   WHERE ur.user_id = v_user;
  IF v_before IS DISTINCT FROM ARRAY['hris_manager'] THEN
    IF v_before = ARRAY['hris_manager', 'hr_user_admin'] THEN
      RAISE NOTICE 'Sudah punya hr_user_admin; tidak ada yang diubah';
      RETURN;
    END IF;
    RAISE EXCEPTION 'Role akun saat ini %, diharapkan {hris_manager}', v_before;
  END IF;

  v_after := array_append(v_before, 'hr_user_admin');
  RAISE NOTICE 'Role: % -> %', v_before, v_after;
  IF current_setting('transaction_read_only') = 'on' THEN
    RAISE NOTICE 'Pratinjau: tidak ada yang ditulis';
    RETURN;
  END IF;

  INSERT INTO iam.user_roles (user_id, role_id, is_primary) VALUES (v_user, v_role, false);
  INSERT INTO iam.permission_audit_logs (source, target_user_id, roles_before, roles_after)
  VALUES ('user_role', v_user, v_before, v_after);
END $$;

SELECT u.email, r.code AS role, ur.is_primary
  FROM iam.user_roles ur
  JOIN iam.roles r ON r.id = ur.role_id
  JOIN auth.users u ON u.id = ur.user_id
 WHERE lower(u.email) = 'paddyhumanresources@gmail.com'
 ORDER BY ur.is_primary DESC, r.code;

\if :{?preview}
ROLLBACK;
\else
COMMIT;
\endif
