-- =============================================================================
-- Kode shift untuk file upload Jadwal Kerja (nama shift live ada yang kembar)
-- + shift bawaan DEF (Default 08.00–17.00) dan NS (Non-Shift 09.00–18.00).
-- Spec: docs/superpowers/specs/2026-10-19-hris-karyawan-jadwal-golive-design.md §2.1
-- =============================================================================

ALTER TABLE hris.shifts
  ADD COLUMN IF NOT EXISTS code text,
  ADD COLUMN IF NOT EXISTS is_default boolean NOT NULL DEFAULT false;

-- Kode mudah dibaca untuk shift yang sudah ada di live (nama + jam masuk).
UPDATE hris.shifts s SET code = m.code
FROM (VALUES
  ('CS Shift 1 - 06.00-14.00', '06:00', 'CS1'),
  ('CS Shift 2 - 11.00-19.00', '11:00', 'CS2'),
  ('CS Shift 3 - 16.00-00.00', '16:00', 'CS3'),
  ('CS Shift 4 - 07.00-15.00', '07:00', 'CS4'),
  ('SPG Store 1 - 08.00-16.00', '08:00', 'SPG1'),
  ('SPG Store 2 - 11.00-19.00', '11:00', 'SPG2'),
  ('SPG Store 3 - 13.30-21.30', '13:30', 'SPG3'),
  ('SPG Store 4 - 14.00-22.00', '14:00', 'SPG4'),
  ('SPG Store 7 - 12.00-20.00', '12:00', 'SPG7'),
  ('SPG Store 11 - 09.00-17.00', '09:00', 'SPG11'),
  ('SPG 13 - 14.30-22.30', '14:30', 'SPG13'),
  ('PERFUMER Shift 1 - 06.00-14.00', '06:00', 'PRF1'),
  ('Shift Pagi', '08:00', 'PAGI8'),
  ('Shift Pagi', '09:00', 'PAGI9'),
  ('Shift Siang', '13:00', 'SIANG13'),
  ('Shift Siang', '14:00', 'SIANG14'),
  ('Shift Malam', '22:00', 'MALAM')
) AS m(name, start_at, code)
WHERE s.code IS NULL AND s.name = m.name AND s.start_time = m.start_at::time
  AND NOT EXISTS (SELECT 1 FROM hris.shifts x WHERE upper(x.code) = m.code);

-- Shift lain (lingkungan non-live): SHIFT1, SHIFT2, … berurutan.
WITH n AS (
  SELECT id, row_number() OVER (ORDER BY sort_order NULLS LAST, start_time, name) AS rn
  FROM hris.shifts WHERE code IS NULL
)
UPDATE hris.shifts s SET code = 'SHIFT' || n.rn FROM n WHERE s.id = n.id;

INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes, is_overnight, sort_order)
SELECT 'DEF', 'Default 08.00–17.00', '08:00', '17:00', 60, 15, false, 0
WHERE NOT EXISTS (SELECT 1 FROM hris.shifts WHERE upper(code) = 'DEF');

INSERT INTO hris.shifts (code, name, start_time, end_time, break_minutes, late_tolerance_minutes, is_overnight, sort_order)
SELECT 'NS', 'Non-Shift 09.00–18.00', '09:00', '18:00', 60, 15, false, 1
WHERE NOT EXISTS (SELECT 1 FROM hris.shifts WHERE upper(code) = 'NS');

UPDATE hris.shifts SET is_default = (upper(code) = 'DEF');

ALTER TABLE hris.shifts ALTER COLUMN code SET NOT NULL;
ALTER TABLE hris.shifts DROP CONSTRAINT IF EXISTS shifts_code_format;
ALTER TABLE hris.shifts ADD CONSTRAINT shifts_code_format CHECK (code ~ '^[A-Z0-9]{1,12}$');
CREATE UNIQUE INDEX IF NOT EXISTS shifts_code_key ON hris.shifts (upper(code));
CREATE UNIQUE INDEX IF NOT EXISTS shifts_single_default ON hris.shifts (is_default) WHERE is_default;
