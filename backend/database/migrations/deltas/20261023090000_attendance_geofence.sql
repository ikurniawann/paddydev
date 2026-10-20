-- =============================================================================
-- Geofence absensi (EPIC-062, MOM AB-01/AB-03, K-14).
--   hris.attendance.clock_in_geofence / clock_out_geofence:
--     valid | invalid | unchecked (Belum dicek) | dinas (Perjalanan Dinas).
--     NULL = absen sebelum EPIC-062, dicatat HRD untuk karyawan lain, atau
--     absen manual/koreksi HRD.
--   *_distance_m: jarak ke toko absensi (meter, dibulatkan), NULL bila tidak
--     bisa diukur.
--   configuration.branches.geofence_radius_m: override radius per toko
--     (NULL = 100 m, nilai MOM).
-- =============================================================================

ALTER TABLE hris.attendance
  ADD COLUMN IF NOT EXISTS clock_in_geofence    text,
  ADD COLUMN IF NOT EXISTS clock_in_distance_m  integer,
  ADD COLUMN IF NOT EXISTS clock_out_geofence   text,
  ADD COLUMN IF NOT EXISTS clock_out_distance_m integer;

DO $$ BEGIN
  ALTER TABLE hris.attendance ADD CONSTRAINT attendance_clock_in_geofence_check
    CHECK (clock_in_geofence IN ('valid', 'invalid', 'unchecked', 'dinas'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE hris.attendance ADD CONSTRAINT attendance_clock_out_geofence_check
    CHECK (clock_out_geofence IN ('valid', 'invalid', 'unchecked', 'dinas'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE hris.attendance ADD CONSTRAINT attendance_geofence_distance_check
    CHECK ((clock_in_distance_m IS NULL OR clock_in_distance_m >= 0)
       AND (clock_out_distance_m IS NULL OR clock_out_distance_m >= 0));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE INDEX IF NOT EXISTS idx_attendance_geofence_invalid ON hris.attendance (date)
  WHERE clock_in_geofence = 'invalid' OR clock_out_geofence = 'invalid';

ALTER TABLE configuration.branches
  ADD COLUMN IF NOT EXISTS geofence_radius_m integer;

DO $$ BEGIN
  ALTER TABLE configuration.branches ADD CONSTRAINT branches_geofence_radius_check
    CHECK (geofence_radius_m IS NULL OR geofence_radius_m BETWEEN 25 AND 1000);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

COMMENT ON COLUMN configuration.branches.geofence_radius_m IS
  'Radius absensi toko dalam meter (EPIC-062); NULL = 100 m sesuai MOM AB-01.';
