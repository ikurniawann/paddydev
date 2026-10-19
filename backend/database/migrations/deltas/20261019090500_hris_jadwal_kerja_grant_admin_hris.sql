-- =============================================================================
-- Admin HRIS mengelola Jadwal Kerja (upload jadwal, jadwal tetap). Hak menu
-- Jadwal Kerja disalin dari Shift Kerja, tempat admin_hris hanya "read";
-- route tulis Jadwal Kerja memeriksa aksi "update" (EPIC-056 review I3).
-- =============================================================================
CREATE OR REPLACE FUNCTION hris.grant_jadwal_kerja_admin_hris() RETURNS void
LANGUAGE sql AS $$
  INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
  SELECT r.id, m.id, '["read", "create", "update", "import", "export"]'::jsonb
  FROM iam.roles r CROSS JOIN iam.menus m
  WHERE r.code = 'admin_hris' AND m.code = 'hris.kepegawaian.schedules'
  ON CONFLICT ON CONSTRAINT role_menu_permissions_unique
  DO UPDATE SET granted_actions = EXCLUDED.granted_actions;
$$;

SELECT hris.grant_jadwal_kerja_admin_hris();
