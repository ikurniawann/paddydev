-- =============================================================================
-- Kurang jam / pulang lebih cepat (EPIC-065, MOM AB-05, Q-AB-1 → ruling R7–R8).
--   configuration.branches.min_work_hours: jam hadir minimum cabang, dihitung
--     clock-in s.d. clock-out (istirahat termasuk). NULL = tanpa aturan.
--     HQ = 9 diisi HR lewat HRIS → Penempatan → Lokasi Toko, bukan oleh migrasi.
--   hris.attendance.min_work_hours / is_short_hours: label tersimpan di baris
--     absensi, dihitung trigger setiap kali jam masuk/pulang/cabang berubah.
--   Notifikasi HR "Jam Kerja Kurang" (< 7 jam, semua cabang) TIDAK diubah:
--     public.fn_notify_attendance_changes dan triggernya tetap seperti semula.
-- Aman dijalankan ulang dan di DB baru.
-- =============================================================================

ALTER TABLE configuration.branches ADD COLUMN IF NOT EXISTS min_work_hours numeric(4,2);

DO $$ BEGIN
  ALTER TABLE configuration.branches ADD CONSTRAINT branches_min_work_hours_check
    CHECK (min_work_hours IS NULL OR (min_work_hours >= 1 AND min_work_hours <= 24));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

COMMENT ON COLUMN configuration.branches.min_work_hours IS
  'EPIC-065 AB-05: jam hadir minimum (clock-in s.d. clock-out); di bawahnya absen berlabel "Kurang jam / pulang lebih cepat". NULL = tanpa aturan.';

ALTER TABLE hris.attendance
  ADD COLUMN IF NOT EXISTS min_work_hours numeric(4,2),
  ADD COLUMN IF NOT EXISTS is_short_hours boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN hris.attendance.is_short_hours IS
  'EPIC-065 AB-05: clock_out - clock_in < jam minimum cabang absen (snapshot di min_work_hours).';

CREATE OR REPLACE FUNCTION hris.attendance_short_hours()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$
BEGIN
  -- Baris yang jam & cabangnya tidak berubah mempertahankan labelnya,
  -- termasuk pelabelan ulang eksplisit saat HR mengubah jam minimum cabang.
  IF TG_OP = 'UPDATE'
     AND NEW.clock_in IS NOT DISTINCT FROM OLD.clock_in
     AND NEW.clock_out IS NOT DISTINCT FROM OLD.clock_out
     AND NEW.branch_id IS NOT DISTINCT FROM OLD.branch_id THEN
    RETURN NEW;
  END IF;
  NEW.min_work_hours := NULL;
  NEW.is_short_hours := false;
  IF NEW.clock_in IS NOT NULL AND NEW.clock_out IS NOT NULL AND NEW.branch_id IS NOT NULL THEN
    NEW.min_work_hours := (SELECT b.min_work_hours FROM configuration.branches b WHERE b.id = NEW.branch_id);
    NEW.is_short_hours := NEW.min_work_hours IS NOT NULL
      AND NEW.clock_out - NEW.clock_in < make_interval(secs => (NEW.min_work_hours * 3600)::double precision);
  END IF;
  RETURN NEW;
END;
$function$;

DROP TRIGGER IF EXISTS attendance_short_hours ON hris.attendance;
CREATE TRIGGER attendance_short_hours BEFORE INSERT OR UPDATE ON hris.attendance
  FOR EACH ROW EXECUTE FUNCTION hris.attendance_short_hours();
