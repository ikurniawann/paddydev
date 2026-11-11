-- =============================================================================
-- EPIC-102 (R5, R8): persetujuan versi bagan organisasi.
--
-- 1) hris.org_chart_versions mencatat siapa dan kapan menyetujui. Indeks unik
--    parsial menjaga hanya satu versi berstatus 'approved'. Data tidak diubah:
--    Draft 6 tetap 'draft' sampai HR menyetujuinya di aplikasi.
-- 2) Menu Struktur Org (hris.organization.org-chart) menawarkan "approve".
--    Baris grant yang SUDAH ADA milik super_admin, admin, hrd, dan admin_hris
--    mendapat "approve" (pola 20261027090100_iam_hris_approve); baris baru
--    tidak dibuat. Role HR turunan admin_hris disinkronkan ulang.
--
-- Idempoten. Rollback: DROP INDEX uq_org_chart_versions_one_approved; kolom
-- baru boleh tetap ada; hapus "approve" dari menu dan grant di atas.
-- =============================================================================
ALTER TABLE hris.org_chart_versions
  ADD COLUMN IF NOT EXISTS approved_at      timestamptz,
  ADD COLUMN IF NOT EXISTS approved_by_name text;

CREATE UNIQUE INDEX IF NOT EXISTS uq_org_chart_versions_one_approved
  ON hris.org_chart_versions ((true)) WHERE status = 'approved';

UPDATE iam.menus m
SET permission_context = jsonb_set(m.permission_context, '{actions}',
      coalesce(m.permission_context->'actions', '["read"]'::jsonb) || '["approve"]'::jsonb),
    updated_at = now()
WHERE m.deleted_at IS NULL
  AND m.code = 'hris.organization.org-chart'
  AND NOT coalesce(m.permission_context->'actions', '[]'::jsonb) ? 'approve';

UPDATE iam.role_menu_permissions p
SET granted_actions = p.granted_actions || '["approve"]'::jsonb, updated_at = now()
FROM iam.roles r, iam.menus m
WHERE r.id = p.role_id AND m.id = p.menu_id
  AND r.code IN ('super_admin', 'admin', 'hrd', 'admin_hris')
  AND m.code = 'hris.organization.org-chart'
  AND jsonb_typeof(p.granted_actions) = 'array'
  AND NOT p.granted_actions ? 'approve';

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
