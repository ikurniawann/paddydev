-- =============================================================================
-- IAM Role Permissions — operational roles (non admin)
-- Run: npm run db:seed:iam-roles
-- Prasyarat: npm run db:seed:iam-menus
-- =============================================================================

INSERT INTO iam.roles (code, name, description, is_system, is_active) VALUES
  ('hrd',                'HRD',                'HR & Recruitment',              false, true),
  ('hiring_manager',     'Hiring Manager',     'Rekrutmen / interview',         false, true),
  ('direksi',            'Direksi',            'Eksekutif / overview',          false, true),
  ('purchasing_admin',   'Purchasing Admin',   'Administrasi pembelian',        false, true),
  ('purchasing_manager', 'Purchasing Manager', 'Manajemen pembelian',           false, true),
  ('purchasing_staff',   'Purchasing Staff',   'Staf pembelian',                false, true),
  ('finance_staff',      'Finance Staff',      'Keuangan & akuntansi',          false, true),
  ('warehouse_staff',    'Warehouse Staff',    'Gudang',                        false, true),
  ('warehouse_admin',    'Warehouse Admin',    'Administrasi gudang',           false, true),
  ('pos',                'POS Cashier',        'Kasir POS',                     false, true),
  ('pos_supervisor',     'POS Supervisor',     'Supervisor POS',                false, true),
  ('qc_staff',           'QC Staff',           'Quality control',               false, true),
  ('mizu_bandung_demo',  'Paddy Bandung Demo',  'Demo cabang Paddy Bandung: Items, Inventory, POS', false, true)
ON CONFLICT (code) DO UPDATE SET
  name        = EXCLUDED.name,
  description = EXCLUDED.description,
  is_active   = true,
  updated_at  = now();

-- hrd -> HRIS + Settings. EPIC-061 (Q-RA-2/Q-RA-5): tanpa "delete" di menu daun HRIS yang hak hapusnya
-- ditambahkan migrasi 20261022090000 dan tanpa Hak Akses HRIS (hris.access) — keduanya milik
-- admin_hris / super_admin. Hari Libur, Pengumuman, Penempatan tetap seperti sebelumnya.
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, CASE WHEN m.code IN ('hris.kepegawaian.users', 'hris.kepegawaian.contracts', 'hris.kepegawaian.shifts',
      'hris.kepegawaian.schedules', 'hris.master.departments', 'hris.master.positions', 'hris.master.employment-statuses',
      'hris.workforce.attendance', 'hris.workforce.leaves', 'hris.compensation.payroll', 'hris.compensation.salary',
      'hris.performance.kpi-scorecard', 'hris.recruitment.candidates', 'hris.recruitment.psikotes', 'hris.recruitment.job-portal')
    THEN COALESCE(m.permission_context->'actions', '["read"]'::jsonb) - 'delete'
    ELSE COALESCE(m.permission_context->'actions', '["read"]'::jsonb) END
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code = 'hrd' AND m.deleted_at IS NULL AND m.is_active = true
  AND (m.code = 'dashboard' OR m.module IN ('hris', 'settings'))
  AND m.code <> 'hris.access'
ON CONFLICT (role_id, menu_id) DO UPDATE SET is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

-- hiring_manager -> subset HRIS (EPIC-061: tanpa "delete" di menu daun Rekrutmen, sama dengan hrd)
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, CASE WHEN m.code IN ('hris.recruitment.candidates', 'hris.recruitment.psikotes', 'hris.recruitment.job-portal')
    THEN COALESCE(m.permission_context->'actions', '["read"]'::jsonb) - 'delete'
    ELSE COALESCE(m.permission_context->'actions', '["read"]'::jsonb) END
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code = 'hiring_manager' AND m.deleted_at IS NULL AND m.is_active = true
  AND (
    m.code IN ('dashboard', 'hris')
    OR m.code LIKE 'hris.recruitment%'
    OR m.code = 'hris.insights.analytics'
  )
ON CONFLICT (role_id, menu_id) DO UPDATE SET is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

-- Purchasing / warehouse / QC -> Items (+ purchasing reports subtree)
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, COALESCE(m.permission_context->'actions', '["read"]'::jsonb)
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code IN ('purchasing_admin', 'purchasing_manager', 'purchasing_staff', 'warehouse_staff', 'warehouse_admin', 'qc_staff')
  AND m.deleted_at IS NULL AND m.is_active = true
  AND (m.code = 'dashboard' OR m.module IN ('items', 'purchasing'))
  -- Pengaturan Pembelian (aturan PO wajib dari PR) hanya untuk pengelola pembelian.
  AND (m.code NOT LIKE 'items.%.purchasing.settings' OR r.code IN ('purchasing_admin', 'purchasing_manager'))
ON CONFLICT (role_id, menu_id) DO UPDATE SET is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

-- finance_staff -> dashboard only
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, COALESCE(m.permission_context->'actions', '["read"]'::jsonb)
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code = 'finance_staff' AND m.deleted_at IS NULL AND m.is_active = true
  AND m.code = 'dashboard'
ON CONFLICT (role_id, menu_id) DO UPDATE SET is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

-- pos roles
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, COALESCE(m.permission_context->'actions', '["read"]'::jsonb)
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code IN ('pos', 'pos_supervisor') AND m.deleted_at IS NULL AND m.is_active = true
  AND (m.code = 'dashboard' OR m.module = 'pos')
ON CONFLICT (role_id, menu_id) DO UPDATE SET is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

-- direksi -> overview lintas modul
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, COALESCE(m.permission_context->'actions', '["read"]'::jsonb)
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code = 'direksi' AND m.deleted_at IS NULL AND m.is_active = true
  AND (
    m.code IN ('dashboard', 'hris', 'hris.insights.analytics', 'hris.insights.reports', 'pos', 'pos.reports.dashboard', 'pos.reports.profit', 'pos.reports.transactions', 'pos.reports.rush-hour', 'pos.reports.product-sales', 'pos.reports.voids', 'pos.reports.closing')
    OR m.module = 'crm'
  )
ON CONFLICT (role_id, menu_id) DO UPDATE SET is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

-- mizu_bandung_demo -> Items + POS (tanpa purchasing reports)
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT r.id, m.id, COALESCE(m.permission_context->'actions', '["read"]'::jsonb)
FROM iam.roles r CROSS JOIN iam.menus m
WHERE r.code = 'mizu_bandung_demo' AND m.deleted_at IS NULL AND m.is_active = true
  AND (
    m.code = 'dashboard'
    OR m.module = 'pos'
    OR (m.module = 'items' AND m.code NOT LIKE 'items.reports%' AND m.code NOT LIKE 'items.%.purchasing.settings')
  )
ON CONFLICT (role_id, menu_id) DO UPDATE SET is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();

-- Migrate legacy mizu_dago_demo assignments
DO $$
DECLARE
    v_old_id uuid;
    v_new_id uuid;
BEGIN
    SELECT id INTO v_old_id FROM iam.roles WHERE code = 'mizu_dago_demo' LIMIT 1;
    SELECT id INTO v_new_id FROM iam.roles WHERE code = 'mizu_bandung_demo' LIMIT 1;

    IF v_old_id IS NULL OR v_new_id IS NULL OR v_old_id = v_new_id THEN
        RETURN;
    END IF;

    INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active)
    SELECT v_new_id, menu_id, granted_actions, is_active
    FROM iam.role_menu_permissions
    WHERE role_id = v_old_id
    ON CONFLICT (role_id, menu_id) DO UPDATE
        SET is_active = EXCLUDED.is_active,
            granted_actions = EXCLUDED.granted_actions,
            updated_at = now();

    UPDATE iam.user_roles
    SET role_id = v_new_id
    WHERE role_id = v_old_id;

    UPDATE iam.roles
    SET is_active = false, updated_at = now()
    WHERE id = v_old_id;
END $$;
