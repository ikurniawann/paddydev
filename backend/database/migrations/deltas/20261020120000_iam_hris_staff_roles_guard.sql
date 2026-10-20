-- =============================================================================
-- Pengaman iam.sync_hris_staff_roles() (EPIC-057, temuan tertunda): bila
-- admin_hris tidak punya satu pun grant aktif, sync dulu menonaktifkan semua
-- grant ketiga role turunan sehingga akun HR kehilangan seluruh akses. Kini
-- sync menolak dengan error dan tidak mengubah apa pun.
-- =============================================================================
CREATE OR REPLACE FUNCTION iam.sync_hris_staff_roles() RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active) THEN
    RAISE EXCEPTION 'admin_hris tidak punya grant aktif: sync role HR dibatalkan agar akun HR tidak kehilangan akses';
  END IF;
  WITH spec(code, excluded) AS (VALUES
    ('hris_manager', ARRAY[]::text[]),
    ('hris_non_payroll', ARRAY['hris.compensation']),
    ('hris_non_rekrutmen_kpi', ARRAY['hris.recruitment', 'hris.performance', 'hris.insights.analytics'])),
  src AS (
    SELECT m.id AS menu_id, m.code,
      (SELECT jsonb_agg(DISTINCT a ORDER BY a)
         FROM jsonb_array_elements_text(rmp.granted_actions || coalesce(m.permission_context->'actions', '[]'::jsonb)) a) AS actions
    FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active),
  want AS (
    SELECT r.id AS role_id, s.menu_id, s.actions
    FROM spec
    JOIN iam.roles r ON r.code = spec.code
    JOIN src s ON NOT EXISTS (
      SELECT 1 FROM unnest(spec.excluded) p WHERE s.code = p OR s.code LIKE p || '.%')),
  upserted AS (
    INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active)
    SELECT role_id, menu_id, actions, true FROM want
    ON CONFLICT ON CONSTRAINT role_menu_permissions_unique
    DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true, updated_at = now()
    RETURNING 1)
  UPDATE iam.role_menu_permissions p SET is_active = false, updated_at = now()
  FROM iam.roles r
  WHERE r.id = p.role_id AND r.code IN (SELECT code FROM spec) AND p.is_active
    AND NOT EXISTS (SELECT 1 FROM want w WHERE w.role_id = p.role_id AND w.menu_id = p.menu_id);
END
$$;
