-- =============================================================================
-- EPIC-080: checkout otomatis per jabatan dan 2 shift dalam sehari.
--   R1  hris.auto_checkout_rules: jabatan (position_id) + cabang (branch_id,
--       NULL = semua lokasi) yang absennya ditutup otomatis di jam selesai
--       shift. Unik per (jabatan, cabang); NULL dihitung satu nilai lewat
--       COALESCE ke uuid nol. Diisi untuk jabatan Host Live dan CRM di semua
--       lokasi; Shopkeeper dipilih HR/pengatur per toko, tidak diisi migrasi.
--   R3/R4  hris.attendance.clock_out_auto: penanda clock-out yang dibuat
--       sistem. Notifikasi HR "Jam Kerja Kurang" dilewati untuk baris ini
--       (public.fn_notify_attendance_changes disalin persis dari bootstrap,
--       hanya ditambah pengecualian itu).
--   R6  shift2_id (opsional) di employee_shift_days dan employee_shifts untuk
--       hari 2 shift (1 absen gabungan). Shift kedua hanya sah bila ada shift
--       pertama. hris.v_employee_schedule membawa shift2_id sebagai kolom
--       terakhir supaya pembaca lama tidak bergeser. Aturan "shift kedua mulai
--       setelah shift pertama selesai" divalidasi di aplikasi.
-- Aman dijalankan ulang dan di DB baru.
-- =============================================================================

-- R1: aturan checkout otomatis.
CREATE TABLE IF NOT EXISTS hris.auto_checkout_rules (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  position_id uuid NOT NULL REFERENCES hris.positions(id) ON DELETE CASCADE,
  branch_id   uuid REFERENCES configuration.branches(id) ON DELETE CASCADE,
  is_active   boolean NOT NULL DEFAULT true,
  created_by  uuid,
  updated_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS auto_checkout_rules_position_branch_uniq
  ON hris.auto_checkout_rules (position_id, COALESCE(branch_id, '00000000-0000-0000-0000-000000000000'::uuid));

CREATE INDEX IF NOT EXISTS auto_checkout_rules_branch_idx
  ON hris.auto_checkout_rules (branch_id) WHERE branch_id IS NOT NULL;

COMMENT ON TABLE hris.auto_checkout_rules IS
  'EPIC-080 R1: absen terbuka jabatan ini (di cabang ini; NULL = semua lokasi) ditutup otomatis di jam selesai shift.';

-- Seed: Host Live dan CRM, semua lokasi.
INSERT INTO hris.auto_checkout_rules (position_id, branch_id)
SELECT p.id, NULL FROM hris.positions p
WHERE p.is_active AND p.title ~* '(host live|crm)'
ON CONFLICT DO NOTHING;

-- R3/R4: penanda checkout otomatis.
ALTER TABLE hris.attendance
  ADD COLUMN IF NOT EXISTS clock_out_auto boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN hris.attendance.clock_out_auto IS
  'EPIC-080 R3: clock_out diisi sistem (= jam selesai shift terjadwal), bukan oleh karyawan.';

-- R4: checkout otomatis tidak memicu notifikasi "Jam Kerja Kurang".
-- Salinan persis public.fn_notify_attendance_changes (bootstrap), kecuali
-- cabang UPDATE clock-out yang dilewati bila NEW.clock_out_auto.
CREATE OR REPLACE FUNCTION public.fn_notify_attendance_changes()
 RETURNS trigger
 LANGUAGE plpgsql
 SECURITY DEFINER
AS $function$
DECLARE
  v_employee_name text;
  v_employee_user_id uuid;
BEGIN
  SELECT full_name, user_id INTO v_employee_name, v_employee_user_id
  FROM employees WHERE id = COALESCE(NEW.employee_id, OLD.employee_id);

  -- Clock-in baru (terlambat saja yang di-notify agar tidak noisy)
  IF TG_OP = 'INSERT' AND NEW.clock_in IS NOT NULL AND NEW.is_late = true THEN
    PERFORM notify_hrd(
      '⏰ Karyawan Terlambat',
      v_employee_name || ' clock-in terlambat ' || COALESCE(NEW.late_minutes, 0) || ' menit (' ||
        to_char(NEW.clock_in AT TIME ZONE 'Asia/Jakarta', 'HH24:MI') || ' WIB).',
      'alert',
      '/dashboard/hris/attendance'
    );

  -- Clock-out update
  ELSIF TG_OP = 'UPDATE' AND OLD.clock_out IS NULL AND NEW.clock_out IS NOT NULL THEN
    -- Hanya notify jika pulang terlalu cepat (work_hours < 7), kecuali
    -- checkout otomatis sistem (EPIC-080 R4).
    IF NEW.work_hours IS NOT NULL AND NEW.work_hours < 7 AND NOT NEW.clock_out_auto THEN
      PERFORM notify_hrd(
        '⚠️ Jam Kerja Kurang: ' || v_employee_name,
        v_employee_name || ' bekerja hanya ' || round(NEW.work_hours::numeric, 1) || ' jam hari ini.',
        'alert',
        '/dashboard/hris/attendance'
      );
    END IF;

  -- DELETE absensi → notify HRD
  ELSIF TG_OP = 'DELETE' THEN
    PERFORM notify_hrd(
      '🗑️ Data Absensi Dihapus',
      'Data absensi ' || v_employee_name || ' tanggal ' ||
        to_char(OLD.date::date, 'DD Mon YYYY') || ' telah dihapus.',
      'alert',
      '/dashboard/hris/attendance'
    );
  END IF;

  RETURN COALESCE(NEW, OLD);
END;
$function$;

-- R6: shift kedua dalam sehari.
ALTER TABLE hris.employee_shift_days
  ADD COLUMN IF NOT EXISTS shift2_id uuid REFERENCES hris.shifts(id);
ALTER TABLE hris.employee_shifts
  ADD COLUMN IF NOT EXISTS shift2_id uuid REFERENCES hris.shifts(id);

DO $$ BEGIN
  ALTER TABLE hris.employee_shift_days ADD CONSTRAINT employee_shift_days_shift2_needs_shift
    CHECK (shift2_id IS NULL OR shift_id IS NOT NULL);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
  ALTER TABLE hris.employee_shifts ADD CONSTRAINT employee_shifts_shift2_needs_shift
    CHECK (shift2_id IS NULL OR shift_id IS NOT NULL);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

COMMENT ON COLUMN hris.employee_shift_days.shift2_id IS
  'EPIC-080 R6: shift kedua di hari yang sama (1 absen gabungan); hanya sah bila shift_id terisi.';
COMMENT ON COLUMN hris.employee_shifts.shift2_id IS
  'EPIC-080 R6: shift kedua di hari yang sama (1 absen gabungan); hanya sah bila shift_id terisi.';

CREATE OR REPLACE VIEW hris.v_employee_schedule AS
  SELECT d.employee_id,
         EXTRACT(ISODOW FROM d.work_date)::int AS day_of_week,
         d.shift_id,
         d.work_date AS effective_from,
         d.work_date AS effective_to,
         0 AS priority,
         d.shift2_id
  FROM hris.employee_shift_days d
  UNION ALL
  SELECT es.employee_id, es.day_of_week, es.shift_id, es.effective_from, es.effective_to, 1, es.shift2_id
  FROM hris.employee_shifts es;
