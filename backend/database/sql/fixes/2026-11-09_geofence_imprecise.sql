-- Relabel absensi lama "Di luar area" yang sebenarnya lokasi perkiraan menjadi
-- "Lokasi tidak akurat" (status 'imprecise'), dengan aturan yang sama seperti
-- domain.Geofence: akurasi > 500 m dan jarak dikurangi 2× akurasi tidak
-- melebihi radius toko (bawaan 100 m). Periode absensi yang sudah closing
-- tidak disentuh (sama seperti relabel perjalanan dinas). Keterangan karyawan
-- dan jarak tidak diubah.
--
-- Prasyarat: migrasi 20261114090000_attendance_geofence_imprecise sudah
-- diterapkan dan backup DB diambil. Jalankan atas izin user:
--   psql -v ON_ERROR_STOP=1 -v expected=<jumlah pratinjau> -f 2026-11-09_geofence_imprecise.sql
-- Tanpa -v expected skrip berhenti setelah pratinjau (ROLLBACK).
\set ON_ERROR_STOP on
BEGIN;
SET LOCAL search_path = hris, public;

CREATE TEMP TABLE geo_fix ON COMMIT DROP AS
SELECT a.id, side
FROM hris.attendance a
LEFT JOIN configuration.branches b ON b.id = a.branch_id
CROSS JOIN LATERAL (VALUES
  ('in',  a.clock_in_geofence,  a.clock_in_distance_m,  (a.clock_in_location->>'accuracy')),
  ('out', a.clock_out_geofence, a.clock_out_distance_m, (a.clock_out_location->>'accuracy'))
) v(side, status, distance_m, accuracy)
WHERE v.status = 'invalid'
  AND v.accuracy ~ '^[0-9.]+$' AND v.accuracy::numeric > 500
  AND v.distance_m - 2 * v.accuracy::numeric <= coalesce(b.geofence_radius_m, 100)
  AND NOT EXISTS (SELECT 1 FROM hris.attendance_periods p
                  WHERE p.status = 'closed' AND a.date BETWEEN p.start_date AND p.end_date);

SELECT side, count(*) FROM geo_fix GROUP BY side ORDER BY side;
SELECT count(*) AS total FROM geo_fix \gset

\if :{?expected}
\else
  \echo 'Pratinjau saja: ulangi dengan -v expected=' :total ' untuk menjalankan.'
  ROLLBACK;
  \quit
\endif

SELECT (:total = :expected) AS same_count \gset
\if :same_count
\else
  \echo 'Jumlah berubah sejak pratinjau: ' :total ' (pratinjau ' :expected '). Tidak ada yang diubah.'
  ROLLBACK;
  \quit
\endif

UPDATE hris.attendance a SET clock_in_geofence = 'imprecise'
  FROM geo_fix f WHERE f.id = a.id AND f.side = 'in';
UPDATE hris.attendance a SET clock_out_geofence = 'imprecise'
  FROM geo_fix f WHERE f.id = a.id AND f.side = 'out';

INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, reason, after)
SELECT 'Ops (Claude Code atas persetujuan user)', 'data_fix.geofence_imprecise', 'hris', 'attendance',
       'Absen dengan lokasi perkiraan (akurasi > 500 m) dilabel ulang dari Di luar area menjadi Lokasi tidak akurat (keputusan user 09-11-2026)',
       jsonb_build_object('rows', count(*), 'clock_in', count(*) FILTER (WHERE side = 'in'),
                          'clock_out', count(*) FILTER (WHERE side = 'out'))
FROM geo_fix;

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM geo_fix f JOIN hris.attendance a ON a.id = f.id
             WHERE (f.side = 'in' AND a.clock_in_geofence <> 'imprecise')
                OR (f.side = 'out' AND a.clock_out_geofence <> 'imprecise')) THEN
    RAISE EXCEPTION 'verifikasi gagal';
  END IF;
END $$;

COMMIT;
