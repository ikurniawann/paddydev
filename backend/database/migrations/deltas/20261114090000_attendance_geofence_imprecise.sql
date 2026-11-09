-- Geofence absensi: status 'imprecise' (Lokasi tidak akurat). HP yang mengirim
-- lokasi perkiraan (Android "Perkiraan", iPhone Lokasi Akurat mati) memberi titik
-- yang dipatok ke kisi ±1 km dengan akurasi ±2000 m, sehingga jaraknya tidak bisa
-- menentukan dalam/luar area. Sama seperti 'invalid', status ini wajib keterangan.
-- Data lama tidak diubah di sini (lihat sql/fixes/2026-11-09_geofence_imprecise.sql).
ALTER TABLE hris.attendance DROP CONSTRAINT IF EXISTS attendance_clock_in_geofence_check;
ALTER TABLE hris.attendance ADD CONSTRAINT attendance_clock_in_geofence_check
  CHECK (clock_in_geofence IN ('valid', 'invalid', 'imprecise', 'unchecked', 'dinas'));

ALTER TABLE hris.attendance DROP CONSTRAINT IF EXISTS attendance_clock_out_geofence_check;
ALTER TABLE hris.attendance ADD CONSTRAINT attendance_clock_out_geofence_check
  CHECK (clock_out_geofence IN ('valid', 'invalid', 'imprecise', 'unchecked', 'dinas'));
