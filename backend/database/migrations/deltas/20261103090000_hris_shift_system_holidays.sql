-- EPIC-079: jam kerja sistem shift, libur nasional & cuti bersama.
--   Staf Shift (hris.employees.schedule_type = 'shift') tidak mengenal tanggal
--   merah; jadwalnya mengikuti roster. Staf Non-Shift (HQ dan Operations)
--   libur pada nasional dan perusahaan, sedangkan cuti bersama baru menjadi
--   libur bila perusahaan menetapkannya (kolom is_day_off). Cuti bersama yang
--   tidak ditetapkan libur adalah hari kerja biasa.

-- 1. Cuti bersama yang ditetapkan libur oleh perusahaan. Default false: semua
--    cuti bersama yang sudah ada tetap hari kerja sampai HR menetapkannya.
ALTER TABLE hris.public_holidays ADD COLUMN IF NOT EXISTS is_day_off boolean NOT NULL DEFAULT false;
COMMENT ON COLUMN hris.public_holidays.is_day_off IS
  'Hanya untuk type = cuti_bersama (EPIC-079): true = perusahaan menetapkan hari ini libur bagi staf Non-Shift; false = hari kerja biasa';
