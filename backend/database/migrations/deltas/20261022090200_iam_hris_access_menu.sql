-- =============================================================================
-- EPIC-061 (RA-03, Q-RA-5): menu HRIS → Hak Akses HRIS untuk AdminHRIS.
-- Diberikan ke admin_hris (→ hris_manager lewat sync) dan super_admin;
-- disembunyikan dari hris_non_payroll dan hris_non_rekrutmen_kpi lewat masker
-- (tetap tanpa "delete", seperti baris masker EPIC-061 lainnya).
-- =============================================================================
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, module, level, permission_context)
VALUES ('hris.access', 'Hak Akses HRIS', '/dashboard/hris/hak-akses', 'identification', 'sidebar', 70, 'hris', 2,
        '{"actions":["read","update"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path, icon = EXCLUDED.icon,
  menu_type = EXCLUDED.menu_type, order_number = EXCLUDED.order_number, module = EXCLUDED.module,
  level = EXCLUDED.level, permission_context = EXCLUDED.permission_context,
  is_active = true, is_visible = true, deleted_at = NULL, updated_at = now();

UPDATE iam.menus child SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code = 'hris.access' AND parent.code = 'hris';

INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, '["read","update"]'::jsonb
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code IN ('admin_hris', 'super_admin') AND m.code = 'hris.access'
ON CONFLICT ON CONSTRAINT role_menu_permissions_unique
DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true, updated_at = now();

INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions) VALUES
  ('hris_non_payroll', 'hris.access', true, '{delete}'),
  ('hris_non_rekrutmen_kpi', 'hris.access', true, '{delete}')
ON CONFLICT (role_code, menu_prefix) DO UPDATE SET
  exclude_menu = true, excluded_actions = EXCLUDED.excluded_actions, updated_at = now();

-- admin_hris grants come from a seeder: on a fresh/CI database there is
-- nothing to derive from yet, and the sync refuses an empty source.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active) THEN
    PERFORM iam.sync_hris_staff_roles();
  END IF;
END
$$;
