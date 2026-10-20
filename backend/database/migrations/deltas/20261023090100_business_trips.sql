-- =============================================================================
-- Perjalanan Dinas (EPIC-062, MOM AB-02): pengajuan karyawan dengan rentang
-- tanggal, diputus atasan langsung (reporting_to) atau HRD. Tanggal yang
-- disetujui membuat status geofence absensi 'dinas'.
-- Aturan dijaga constraint: selesai >= mulai, maksimal 31 hari, dan tidak ada
-- dua pengajuan pending/approved yang beririsan untuk karyawan yang sama.
-- =============================================================================

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE IF NOT EXISTS hris.business_trips (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  employee_id      uuid NOT NULL REFERENCES hris.employees(id) ON DELETE CASCADE,
  start_date       date NOT NULL,
  end_date         date NOT NULL,
  destination      text NOT NULL,
  purpose          text NOT NULL,
  status           text NOT NULL DEFAULT 'pending',
  requested_by     uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  decided_by       uuid REFERENCES hris.employees(id) ON DELETE SET NULL,
  decided_by_name  text,
  decided_at       timestamptz,
  rejection_reason text,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT business_trips_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
  CONSTRAINT business_trips_dates_check CHECK (end_date >= start_date AND end_date - start_date <= 30),
  CONSTRAINT business_trips_text_check CHECK (length(btrim(destination)) > 0 AND length(btrim(purpose)) > 0),
  CONSTRAINT business_trips_no_overlap EXCLUDE USING gist (
    employee_id WITH =,
    daterange(start_date, end_date, '[]') WITH &&
  ) WHERE (status IN ('pending', 'approved'))
);

CREATE INDEX IF NOT EXISTS idx_business_trips_employee ON hris.business_trips (employee_id, start_date DESC);
CREATE INDEX IF NOT EXISTS idx_business_trips_status ON hris.business_trips (status, start_date);

DROP TRIGGER IF EXISTS business_trips_updated_at ON hris.business_trips;
CREATE TRIGGER business_trips_updated_at BEFORE UPDATE ON hris.business_trips
  FOR EACH ROW EXECUTE FUNCTION public.update_hris_updated_at();

COMMENT ON TABLE hris.business_trips IS
  'Perjalanan Dinas (EPIC-062): rentang tanggal yang disetujui dikecualikan dari geofence absensi.';

-- ── Menu ESS → Perjalanan Dinas ──────────────────────────────────────────────
INSERT INTO iam.menus (code, menu_name, route_path, icon, menu_type, order_number, permission_context)
VALUES ('ess.business-trip', 'Perjalanan Dinas', '/dashboard/me/perjalanan-dinas', 'briefcase', 'sidebar', 25,
        '{"actions":["read"]}'::jsonb)
ON CONFLICT (code) DO UPDATE SET
  menu_name = EXCLUDED.menu_name, route_path = EXCLUDED.route_path,
  icon = EXCLUDED.icon, menu_type = EXCLUDED.menu_type,
  order_number = EXCLUDED.order_number, is_active = true, is_visible = true,
  deleted_at = NULL, updated_at = now();

-- Sejajar dengan Lembur: di bawah induk "Pengajuan" (ess.requests) bila Lembur
-- ada di sana, atau langsung di bawah Area Karyawan (ess) bila tidak.
UPDATE iam.menus SET module = 'ess', level = 2 WHERE code = 'ess.business-trip';
UPDATE iam.menus child SET parent_id = parent.id
FROM iam.menus parent
WHERE child.code = 'ess.business-trip' AND parent.code = 'ess';
UPDATE iam.menus child SET parent_id = sibling.parent_id, level = sibling.level
FROM iam.menus sibling
WHERE child.code = 'ess.business-trip' AND sibling.code = 'ess.overtime'
  AND sibling.deleted_at IS NULL AND sibling.parent_id IS NOT NULL;

-- Hak akses mengikuti menu ESS Lembur: role yang sama, aksi read.
INSERT INTO iam.role_menu_permissions (role_id, menu_id, granted_actions)
SELECT rmp.role_id, m.id, '["read"]'::jsonb
FROM iam.role_menu_permissions rmp
JOIN iam.menus src ON src.id = rmp.menu_id AND src.code = 'ess.overtime'
CROSS JOIN iam.menus m
WHERE m.code = 'ess.business-trip' AND rmp.is_active
ON CONFLICT (role_id, menu_id) DO UPDATE SET
  is_active = true, granted_actions = EXCLUDED.granted_actions, updated_at = now();
