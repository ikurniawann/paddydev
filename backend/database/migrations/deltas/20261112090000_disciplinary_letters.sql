-- EPIC-094 R9/R10: Surat Teguran dan Surat Peringatan (SP1–SP3) yang
-- diterbitkan HR. Kebijakan Paddy tanpa potongan keterlambatan: tindakan
-- disipliner berjenjang diterbitkan manual oleh HR, lalu lonceng masuk ke
-- karyawan, penyetujunya, dan HR. Surat yang salah dibatalkan (alasan wajib),
-- tidak dihapus.
CREATE TABLE IF NOT EXISTS hris.disciplinary_letters (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id     uuid NOT NULL REFERENCES hris.employees(id) ON DELETE CASCADE,
  letter_type     text NOT NULL CHECK (letter_type IN ('teguran', 'sp1', 'sp2', 'sp3')),
  issued_date     date NOT NULL,
  notes           text,
  attachment_path text,
  created_by      uuid REFERENCES configuration.users(id) ON DELETE SET NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  revoked_at      timestamptz,
  revoked_by      uuid REFERENCES configuration.users(id) ON DELETE SET NULL,
  revoke_reason   text,
  CHECK ((revoked_at IS NULL) = (revoke_reason IS NULL))
);

CREATE INDEX IF NOT EXISTS disciplinary_letters_employee_idx
  ON hris.disciplinary_letters (employee_id, issued_date DESC);

COMMENT ON TABLE hris.disciplinary_letters IS
  'Surat Teguran/SP yang diterbitkan HR (EPIC-094). Dibatalkan = revoked_at terisi, baris tetap ada.';

-- Menu Kehadiran & Cuti → Surat Peringatan: read, create (terbitkan), update (batalkan).
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context)
VALUES
  ('hris.workforce.disciplinary-letters', 'Surat Peringatan', '/dashboard/hris/surat-peringatan',
   'clipboard', 'sidebar', 15, '{"actions":["read","create","update"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path, icon = EXCLUDED.icon,
  menu_type = EXCLUDED.menu_type, order_number = EXCLUDED.order_number,
  permission_context = EXCLUDED.permission_context,
  is_active = true, is_visible = true, deleted_at = NULL, updated_at = now();

UPDATE iam.menus child SET parent_id = parent.id, module = 'hris', level = 3, updated_at = now()
FROM iam.menus parent
WHERE child.code = 'hris.workforce.disciplinary-letters' AND parent.code = 'hris.workforce';

INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, '["read","create","update","delete","approve","export","import","execute"]'::jsonb
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code IN ('super_admin', 'admin') AND m.code = 'hris.workforce.disciplinary-letters'
ON CONFLICT (role_id, menu_id) DO UPDATE SET
  is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

-- admin_hris memegang semua aksi; hris_manager dan role staf HR lain
-- diturunkan oleh iam.sync_hris_staff_roles() (tanpa masker: semua role HR
-- boleh menerbitkan). Sync dilewati di database baru tanpa grant admin_hris.
DO $$
DECLARE seeded boolean;
BEGIN
  SELECT EXISTS (
    SELECT 1 FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active AND m.code <> 'hris.workforce.disciplinary-letters') INTO seeded;

  INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
  SELECT r.id, m.id, '["read","create","update"]'::jsonb
  FROM iam.roles r CROSS JOIN iam.menus m
  WHERE r.code = 'admin_hris' AND m.code = 'hris.workforce.disciplinary-letters'
  ON CONFLICT (role_id, menu_id) DO UPDATE SET
    is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

  IF seeded THEN
    PERFORM iam.sync_hris_staff_roles();
  END IF;
END
$$;
