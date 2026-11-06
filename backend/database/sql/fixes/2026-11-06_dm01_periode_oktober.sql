-- DM-01 lanjutan (keputusan user 06-11-2026): setelah data absensi dan pengajuan ≤ 20 Oktober
-- 2026 dihapus (tools/ops/dm01-cleanup.sh execute), ringkasan closing periode absensi Oktober
-- 2026 (1–20 Okt, tertutup) disamakan dengan Jan–Sep: close_summary dinolkan dan riwayat
-- closing-nya (hris.attendance_period_events) dihapus, supaya tab Periode Absensi tidak
-- menampilkan angka dari data yang sudah tidak ada. Baris periodenya TETAP ada dan tetap tertutup
-- (re-materialisasi cutoff akan bentrok dengan periode 1–20 Okt yang beku).
-- Nilai sebelumnya disimpan di audit.audit_log (data_cleanup.dm01_periods, kolom before).
--
-- Pemakaian (sesudah execute DM-01 berhasil, di jam yang sama):
--   psql -1 -v ON_ERROR_STOP=1 -f 2026-11-06_dm01_periode_oktober.sql
\set ON_ERROR_STOP on
SET LOCAL hris.allow_closed_period_write = 'on';

DO $$
DECLARE
  p hris.attendance_periods;
  n int;
BEGIN
  IF current_setting('hris.allow_closed_period_write', true) IS DISTINCT FROM 'on' THEN
    RAISE EXCEPTION 'Jalankan dalam satu transaksi: psql -1 -v ON_ERROR_STOP=1 -f 2026-11-06_dm01_periode_oktober.sql';
  END IF;
  SELECT * INTO p FROM hris.attendance_periods WHERE period_year = 2026 AND period_month = 10 FOR UPDATE;
  IF NOT FOUND OR p.start_date <> DATE '2026-10-01' OR p.end_date <> DATE '2026-10-20' OR p.status <> 'closed' THEN
    RAISE EXCEPTION 'Dibatalkan: periode absensi 2026-10 harus ada, 1–20 Okt, dan tertutup';
  END IF;
  SELECT count(*) INTO n FROM hris.attendance WHERE date BETWEEN p.start_date AND p.end_date;
  IF n <> 0 THEN
    RAISE EXCEPTION 'Dibatalkan: masih ada % baris absensi 1–20 Okt; jalankan cleanup DM-01 dulu', n;
  END IF;

  INSERT INTO audit.audit_log (actor_id, actor_name, action, entity, entity_id, entity_label, before, after, reason)
  SELECT NULL, 'Ops (Claude Code atas persetujuan user)', 'data_cleanup.dm01_periods', 'hris', 'dm01',
         'Ringkasan closing periode Oktober 2026 (1–20 Okt)',
         jsonb_build_object(
           'periods', jsonb_build_array(jsonb_build_object('period', '2026-10', 'close_summary', p.close_summary)),
           'events', COALESCE((SELECT jsonb_agg(to_jsonb(e) ORDER BY e.created_at)
                               FROM hris.attendance_period_events e WHERE e.period_id = p.id), '[]'::jsonb)),
         jsonb_build_object('events', 'dihapus', 'close_summary', 'nol'),
         'Data absensi dan pengajuan <= 20 Okt sudah dihapus (DM-01, keputusan user 06-11: payroll mulai 21 Okt); ringkasan closing periode Oktober disamakan (0) dan riwayat closing-nya dihapus';

  UPDATE hris.attendance_periods
  SET close_summary = (SELECT jsonb_object_agg(k, 0) FROM jsonb_object_keys(COALESCE(p.close_summary, '{}'::jsonb)) AS k),
      updated_at = now()
  WHERE id = p.id;
  DELETE FROM hris.attendance_period_events WHERE period_id = p.id;
END $$;

SET LOCAL hris.allow_closed_period_write = 'off';
