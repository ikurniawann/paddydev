-- =============================================================================
-- EPIC-103 R9: aksi "import" pada Struktur Org untuk impor versi bagan dan
-- hapus versi draft — role yang sama dengan hak setujui (EPIC-102 R8a).
--
-- 1) Menu hris.organization.org-chart menawarkan "import"; grant yang SUDAH
--    ADA milik super_admin, admin, hrd, admin_hris mendapatkannya.
-- 2) Masker hris_non_payroll dan hris_non_rekrutmen_kpi pada daun itu
--    (dibuat 20261116090100) ditambah "import"; bila belum ada, dibuat
--    dengan mewarisi masker yang berlaku. hris_manager ikut admin_hris.
-- 3) Role HR turunan admin_hris disinkronkan ulang.
--
-- Idempoten. Rollback: hapus "import" dari menu, grant, dan masker di atas,
-- lalu SELECT iam.sync_hris_staff_roles().
-- =============================================================================
UPDATE iam.menus m
SET permission_context = jsonb_set(m.permission_context, '{actions}',
      coalesce(m.permission_context->'actions', '["read"]'::jsonb) || '["import"]'::jsonb),
    updated_at = now()
WHERE m.deleted_at IS NULL
  AND m.code = 'hris.organization.org-chart'
  AND NOT coalesce(m.permission_context->'actions', '[]'::jsonb) ? 'import';

UPDATE iam.role_menu_permissions p
SET granted_actions = p.granted_actions || '["import"]'::jsonb, updated_at = now()
FROM iam.roles r, iam.menus m
WHERE r.id = p.role_id AND m.id = p.menu_id
  AND r.code IN ('super_admin', 'admin', 'hrd', 'admin_hris')
  AND m.code = 'hris.organization.org-chart'
  AND jsonb_typeof(p.granted_actions) = 'array'
  AND NOT p.granted_actions ? 'import';

INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions)
SELECT r.code, 'hris.organization.org-chart', coalesce(w.exclude_menu, false),
  (SELECT array_agg(DISTINCT a ORDER BY a) FROM unnest(coalesce(w.excluded_actions, '{}'::text[]) || '{approve,import}'::text[]) a)
FROM iam.roles r
LEFT JOIN LATERAL (
  SELECT k.exclude_menu, k.excluded_actions
  FROM iam.hris_staff_role_masks k
  WHERE k.role_code = r.code
    AND ('hris.organization.org-chart' = k.menu_prefix OR 'hris.organization.org-chart' LIKE k.menu_prefix || '.%')
  ORDER BY length(k.menu_prefix) DESC
  LIMIT 1) w ON true
WHERE r.code IN ('hris_non_payroll', 'hris_non_rekrutmen_kpi') AND r.deleted_at IS NULL
ON CONFLICT (role_code, menu_prefix) DO UPDATE SET
  excluded_actions = (SELECT array_agg(DISTINCT a ORDER BY a)
                      FROM unnest(iam.hris_staff_role_masks.excluded_actions || '{import}'::text[]) a),
  updated_at = now()
WHERE NOT ('import' = ANY (iam.hris_staff_role_masks.excluded_actions));

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
