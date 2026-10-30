-- EPIC-074 R6: menu ESS → Absensi Tim (/dashboard/me/absensi-tim). Atasan
-- melihat presensi harian seluruh timnya (bawahan langsung dan tidak langsung).
-- Di-grant ke SEMUA role (pola ess.store-schedule): API-nya hanya memuat tim
-- akun itu, dan menunya disembunyikan bagi akun tanpa bawahan aktif.
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context)
VALUES ('ess.team-attendance', 'Absensi Tim', '/dashboard/me/absensi-tim', 'users', 'sidebar', 44,
        '{"actions":["read"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path,
  icon = EXCLUDED.icon, menu_type = EXCLUDED.menu_type,
  order_number = EXCLUDED.order_number, permission_context = EXCLUDED.permission_context,
  is_active = true, is_visible = true, deleted_at = NULL, updated_at = now();

UPDATE iam.menus SET module = 'ess', level = 2 WHERE code = 'ess.team-attendance';
UPDATE iam.menus child SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code = 'ess.team-attendance' AND parent.code = 'ess';

INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, COALESCE(m.permission_context->'actions', '["read"]'::jsonb)
FROM iam.roles r CROSS JOIN iam.menus m
WHERE m.code = 'ess.team-attendance' AND r.deleted_at IS NULL
ON CONFLICT (role_id, menu_id) DO UPDATE SET
  is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();
