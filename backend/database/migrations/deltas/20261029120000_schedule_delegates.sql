-- EPIC-071 R1/R2: pengatur jadwal toko. HR menunjuk karyawan biasa (mis. kepala
-- toko) yang boleh mengatur jadwal kerja dan, bila can_place, penempatan toko
-- karyawan di toko-toko tertentu. Tanpa menu HRIS: aksesnya lewat menu ESS
-- "Jadwal Toko" dan penunjukan ini.

CREATE TABLE IF NOT EXISTS hris.schedule_delegates (
  employee_id uuid PRIMARY KEY REFERENCES hris.employees(id) ON DELETE CASCADE,
  can_place   boolean NOT NULL DEFAULT false,
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  uuid,
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS hris.schedule_delegate_branches (
  employee_id uuid NOT NULL REFERENCES hris.schedule_delegates(employee_id) ON DELETE CASCADE,
  branch_id   uuid NOT NULL REFERENCES configuration.branches(id) ON DELETE CASCADE,
  PRIMARY KEY (employee_id, branch_id)
);

CREATE INDEX IF NOT EXISTS idx_schedule_delegate_branches_branch
  ON hris.schedule_delegate_branches (branch_id);

-- Menu ESS → Jadwal Toko (/dashboard/me/jadwal-toko). Di-grant ke SEMUA role
-- (pola ess_shift_tim_menu): API-nya memeriksa penunjukan, karyawan yang bukan
-- pengatur hanya melihat penjelasan, bukan data orang lain.
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context)
VALUES ('ess.store-schedule', 'Jadwal Toko', '/dashboard/me/jadwal-toko', 'calendar-days', 'sidebar', 46,
        '{"actions":["read","update"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path,
  icon = EXCLUDED.icon, menu_type = EXCLUDED.menu_type,
  order_number = EXCLUDED.order_number, is_active = true, is_visible = true,
  deleted_at = NULL, updated_at = now();

UPDATE iam.menus SET module = 'ess', level = 2 WHERE code = 'ess.store-schedule';
UPDATE iam.menus child SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code = 'ess.store-schedule' AND parent.code = 'ess';

INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, COALESCE(m.permission_context->'actions', '["read"]'::jsonb)
FROM iam.roles r CROSS JOIN iam.menus m
WHERE m.code = 'ess.store-schedule' AND r.deleted_at IS NULL
ON CONFLICT (role_id, menu_id) DO UPDATE SET
  is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();
