-- =============================================================================
-- Paket Approval atasan (EPIC-063, MOM AP-01..AP-04, Q-APV-3, K-13).
--   decided_as: kapasitas pemutus pengajuan karyawan — 'manager' (atasan
--     langsung) atau 'hr' (HR sebagai cadangan / override). NULL = keputusan
--     sebelum EPIC-063, konfirmasi atau pembatalan penugasan lembur.
--   decided_by_name: nama akun pemutus (HR tanpa data karyawan tetap tercatat).
--   overtime_requests.corrected_at / corrected_by_name: koreksi AdminHRIS
--     terakhir; rincian sebelum/sesudah dan alasan ada di audit.audit_log.
--   Menu ESS → Persetujuan Tim (ess.team-approvals).
-- =============================================================================

ALTER TABLE hris.leaves
  ADD COLUMN IF NOT EXISTS decided_as      text,
  ADD COLUMN IF NOT EXISTS decided_by_name text;

DO $$ BEGIN
  ALTER TABLE hris.leaves ADD CONSTRAINT leaves_decided_as_check
    CHECK (decided_as IN ('manager', 'hr'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

ALTER TABLE hris.overtime_requests
  ADD COLUMN IF NOT EXISTS decided_as        text,
  ADD COLUMN IF NOT EXISTS decided_by_name   text,
  ADD COLUMN IF NOT EXISTS corrected_at      timestamptz,
  ADD COLUMN IF NOT EXISTS corrected_by_name text;

DO $$ BEGIN
  ALTER TABLE hris.overtime_requests ADD CONSTRAINT overtime_requests_decided_as_check
    CHECK (decided_as IN ('manager', 'hr'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

COMMENT ON COLUMN hris.leaves.decided_as IS
  'EPIC-063: manager = atasan langsung, hr = HR cadangan/override (Q-APV-3).';
COMMENT ON COLUMN hris.overtime_requests.decided_as IS
  'EPIC-063: manager = atasan langsung, hr = HR cadangan/override (Q-APV-3).';

-- ── Menu ESS → Persetujuan Tim ───────────────────────────────────────────────
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context)
VALUES ('ess.team-approvals', 'Persetujuan Tim', '/dashboard/me/persetujuan-tim', 'check-circle', 'sidebar', 44,
        '{"actions":["read"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path,
  icon = EXCLUDED.icon, menu_type = EXCLUDED.menu_type,
  order_number = EXCLUDED.order_number, is_active = true, is_visible = true,
  deleted_at = NULL, updated_at = now();

UPDATE iam.menus SET module = 'ess', level = 2 WHERE code = 'ess.team-approvals';
UPDATE iam.menus child SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code = 'ess.team-approvals' AND parent.code = 'ess';

-- Hak akses mengikuti Shift Tim (menu atasan lain di ESS): role yang sama, aksi read.
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT rmp.role_id, m.id, '["read"]'::jsonb
FROM iam.role_menu_permissions rmp
JOIN iam.menus src ON src.id = rmp.menu_id AND src.code = 'ess.team-shifts'
CROSS JOIN iam.menus m
WHERE m.code = 'ess.team-approvals' AND rmp.is_active
ON CONFLICT (role_id, menu_id) DO UPDATE SET
  is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();
