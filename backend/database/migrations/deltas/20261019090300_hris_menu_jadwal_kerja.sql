-- Menu Kepegawaian → Jadwal Kerja (bulk upload jadwal per cabang). Hak
-- aksesnya disalin dari Shift Kerja, sehingga peran yang sudah mengelola
-- Shift Kerja otomatis bisa membuka Jadwal Kerja.
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context)
VALUES ('hris.kepegawaian.schedules', 'Jadwal Kerja', '/dashboard/hris/schedules', 'calendar',
        'sidebar', 31, '{"actions":["read","create","update","import","export"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path,
  icon = EXCLUDED.icon, menu_type = EXCLUDED.menu_type,
  order_number = EXCLUDED.order_number, is_active = true, is_visible = true,
  deleted_at = NULL, updated_at = now();

UPDATE iam.menus SET module = 'hris', level = 3 WHERE code = 'hris.kepegawaian.schedules';
UPDATE iam.menus child SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code = 'hris.kepegawaian.schedules' AND parent.code = 'hris.kepegawaian';

INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT rmp.role_id, m.id, rmp.granted_actions
FROM iam.role_menu_permissions rmp
JOIN iam.menus s ON s.id = rmp.menu_id AND s.code = 'hris.kepegawaian.shifts'
CROSS JOIN iam.menus m
WHERE m.code = 'hris.kepegawaian.schedules'
ON CONFLICT ON CONSTRAINT role_menu_permissions_unique DO NOTHING;
