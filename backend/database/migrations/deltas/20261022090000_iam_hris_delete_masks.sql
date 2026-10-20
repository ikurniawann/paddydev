-- =============================================================================
-- EPIC-061 (RA-01/RA-02, Q-RA-1/2/6): hak hapus HRIS per menu daun dan
-- masker per role untuk role HR turunan admin_hris.
--
-- 1) Aksi "delete" ditawarkan di semua menu daun HRIS yang punya endpoint
--    hapus/timpa, lalu diberikan ke admin_hris dan super_admin (grant aktif
--    yang sudah ada). Role admin sudah memegang semua aksi; hrd tidak diubah.
-- 2) iam.hris_staff_role_masks: per role turunan, prefix menu yang
--    disembunyikan (exclude_menu) atau aksinya dikurangi (excluded_actions).
--    Prefix terpanjang yang cocok menang; "read" tidak bisa dimasker.
-- 3) iam.sync_hris_staff_roles() membaca masker itu. hris_manager penuh,
--    hris_non_payroll dan hris_non_rekrutmen_kpi tanpa "delete". Halaman
--    Hak Akses HRIS menulis masker, jadi perubahannya bertahan setiap sync.
-- =============================================================================
UPDATE iam.menus m
SET permission_context = jsonb_set(m.permission_context, '{actions}',
      coalesce(m.permission_context->'actions', '["read"]'::jsonb) || '["delete"]'::jsonb),
    updated_at = now()
WHERE m.deleted_at IS NULL
  AND m.code IN ('hris.kepegawaian.users', 'hris.kepegawaian.contracts', 'hris.kepegawaian.shifts',
                 'hris.kepegawaian.schedules', 'hris.master.departments', 'hris.master.positions',
                 'hris.master.employment-statuses', 'hris.workforce.attendance', 'hris.workforce.leaves',
                 'hris.compensation.payroll', 'hris.compensation.salary', 'hris.performance.kpi-scorecard',
                 'hris.recruitment.candidates', 'hris.recruitment.psikotes', 'hris.recruitment.job-portal')
  AND NOT coalesce(m.permission_context->'actions', '[]'::jsonb) ? 'delete';

UPDATE iam.role_menu_permissions p
SET granted_actions = p.granted_actions || '["delete"]'::jsonb, updated_at = now()
FROM iam.roles r, iam.menus m
WHERE r.id = p.role_id AND m.id = p.menu_id AND p.is_active
  AND r.code IN ('admin_hris', 'super_admin')
  AND m.code IN ('hris.kepegawaian.users', 'hris.kepegawaian.contracts', 'hris.kepegawaian.shifts',
                 'hris.kepegawaian.schedules', 'hris.kepegawaian.holidays', 'hris.kepegawaian.announcements',
                 'hris.master.placements', 'hris.master.departments', 'hris.master.positions',
                 'hris.master.employment-statuses', 'hris.workforce.attendance', 'hris.workforce.leaves',
                 'hris.compensation.payroll', 'hris.compensation.salary', 'hris.performance.kpi-scorecard',
                 'hris.recruitment.candidates', 'hris.recruitment.psikotes', 'hris.recruitment.job-portal')
  AND NOT p.granted_actions ? 'delete';

CREATE TABLE IF NOT EXISTS iam.hris_staff_role_masks (
  role_code        varchar(50)  NOT NULL REFERENCES iam.roles(code) ON UPDATE CASCADE ON DELETE CASCADE,
  menu_prefix      varchar(100) NOT NULL,
  exclude_menu     boolean      NOT NULL DEFAULT false,
  excluded_actions text[]       NOT NULL DEFAULT '{}',
  updated_at       timestamptz  NOT NULL DEFAULT now(),
  updated_by       uuid         REFERENCES auth.users(id) ON DELETE SET NULL,
  PRIMARY KEY (role_code, menu_prefix),
  CONSTRAINT hris_staff_role_masks_read_kept CHECK (NOT ('read' = ANY (excluded_actions)))
);

INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions) VALUES
  ('hris_manager', 'hris', false, '{}'),
  ('hris_non_payroll', 'hris', false, '{delete}'),
  ('hris_non_payroll', 'hris.compensation', true, '{delete}'),
  ('hris_non_rekrutmen_kpi', 'hris', false, '{delete}'),
  ('hris_non_rekrutmen_kpi', 'hris.recruitment', true, '{delete}'),
  ('hris_non_rekrutmen_kpi', 'hris.performance', true, '{delete}'),
  ('hris_non_rekrutmen_kpi', 'hris.insights.analytics', true, '{delete}')
ON CONFLICT (role_code, menu_prefix) DO NOTHING;

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
  WITH derived AS (
    SELECT DISTINCT r.id AS role_id, r.code
    FROM iam.hris_staff_role_masks k
    JOIN iam.roles r ON r.code = k.role_code AND r.deleted_at IS NULL),
  src AS (
    SELECT m.id AS menu_id, m.code,
      (SELECT jsonb_agg(DISTINCT a ORDER BY a)
         FROM jsonb_array_elements_text(rmp.granted_actions || coalesce(m.permission_context->'actions', '[]'::jsonb)) a) AS actions
    FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active),
  resolved AS (
    SELECT d.role_id, s.menu_id, s.code, s.actions,
      coalesce(mk.exclude_menu, false) AS exclude_menu,
      coalesce(mk.excluded_actions, '{}'::text[]) AS excluded
    FROM derived d
    CROSS JOIN src s
    LEFT JOIN LATERAL (
      SELECT x.exclude_menu, x.excluded_actions
      FROM iam.hris_staff_role_masks x
      WHERE x.role_code = d.code AND (s.code = x.menu_prefix OR s.code LIKE x.menu_prefix || '.%')
      ORDER BY length(x.menu_prefix) DESC
      LIMIT 1) mk ON true),
  kept AS (
    SELECT role_id, menu_id, code,
      (SELECT coalesce(jsonb_agg(a ORDER BY a), '[]'::jsonb)
         FROM jsonb_array_elements_text(actions) a WHERE NOT (a = ANY (excluded))) AS actions
    FROM resolved
    WHERE NOT exclude_menu),
  ancestors AS (
    SELECT DISTINCT k.role_id, s.menu_id, '["read"]'::jsonb AS actions
    FROM kept k
    JOIN src s ON k.code LIKE s.code || '.%'
    WHERE NOT EXISTS (SELECT 1 FROM kept k2 WHERE k2.role_id = k.role_id AND k2.menu_id = s.menu_id)),
  want AS (
    SELECT role_id, menu_id, actions FROM kept
    UNION ALL
    SELECT role_id, menu_id, actions FROM ancestors),
  upserted AS (
    INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions, is_active)
    SELECT role_id, menu_id, actions, true FROM want
    ON CONFLICT ON CONSTRAINT role_menu_permissions_unique
    DO UPDATE SET granted_actions = EXCLUDED.granted_actions, is_active = true, updated_at = now()
    RETURNING 1)
  UPDATE iam.role_menu_permissions p SET is_active = false, updated_at = now()
  FROM derived d
  WHERE p.role_id = d.role_id AND p.is_active
    AND NOT EXISTS (SELECT 1 FROM want w WHERE w.role_id = p.role_id AND w.menu_id = p.menu_id);
END
$$;

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
