-- Spec §2.2 + keputusan user 19-10-2026: karyawan aktif dengan penempatan
-- utama berjalan di "Paddy HQ Bandung" / "Warehouse Paddy" menjadi
-- Non-Shift dengan jadwal tetap Sen–Jum NS 09.00–18.00, Sab–Min libur —
-- KECUALI Shopkeeper/event/Pasaraya/Crew Experience, Store Supervisor /
-- Retail Operation Manager, dan akun sistem (tetap Shift).
-- Bagian 1 = daftar (baca saja). Bagian 2 = ubah, dijalankan setelah daftar
-- disetujui; dibatalkan otomatis bila jumlahnya tidak sesuai :expected.

-- 1. Daftar
CREATE TEMP VIEW non_shift_targets AS
SELECT e.id, e.nip, e.full_name, b.name AS cabang, coalesce(p2.title, '') AS jabatan
FROM hris.employees e
JOIN hris.employee_placements p ON p.employee_id = e.id AND p.is_primary
  AND p.start_date <= current_date AND (p.end_date IS NULL OR p.end_date >= current_date)
JOIN configuration.branches b ON b.id = p.branch_id
LEFT JOIN hris.positions p2 ON p2.id = e.job_title_id
WHERE e.is_active AND b.name IN ('Paddy HQ Bandung', 'Warehouse Paddy')
  AND coalesce(p2.title, '') !~* '(shopkeeper|pasaraya|crew experience|store supervisor|retail operation manager)'
  AND NOT (p2.title IS NULL AND e.full_name ~* '^(super admin|demo|paddy )');
SELECT cabang, count(*) FROM non_shift_targets GROUP BY 1 ORDER BY 1;

-- 2. Ubah (satu transaksi; psql -v ON_ERROR_STOP=1 -v expected=105 -v from=2026-10-20)
SELECT count(*) = :expected AS target_ok, count(*) AS target_count FROM non_shift_targets \gset
\if :target_ok
\else
  \echo 'Jumlah target' :target_count 'tidak sama dengan' :expected '— dibatalkan'
  \quit
\endif
BEGIN;
UPDATE hris.employees e SET schedule_type = 'non_shift', updated_at = now()
FROM non_shift_targets t WHERE e.id = t.id AND e.schedule_type <> 'non_shift';
-- Jadwal tetap (sama dengan applyFixedSchedule): hapus jadwal per tanggal &
-- pola mulai :from, tutup pola berjalan, pasang Sen–Jum NS / Sab–Min libur.
DELETE FROM hris.employee_shift_days d USING non_shift_targets t WHERE d.employee_id = t.id AND d.work_date >= :'from'::date;
DELETE FROM hris.employee_shifts es USING non_shift_targets t WHERE es.employee_id = t.id AND es.effective_from >= :'from'::date;
UPDATE hris.employee_shifts es SET effective_to = :'from'::date - 1
FROM non_shift_targets t WHERE es.employee_id = t.id AND (es.effective_to IS NULL OR es.effective_to >= :'from'::date);
INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from, created_by_name)
SELECT t.id, d, CASE WHEN d <= 5 THEN (SELECT id FROM hris.shifts WHERE code = 'NS') END, :'from'::date, 'Go-live EPIC-056'
FROM non_shift_targets t CROSS JOIN generate_series(1, 7) d;
INSERT INTO audit.audit_log (actor_name, action, entity, entity_id, entity_label, before, after, reason)
SELECT 'Sistem (go-live EPIC-056)', 'employee.schedule_type', 'hris.employees', t.id::text, t.full_name,
  '{"schedule_type":"shift"}'::jsonb,
  jsonb_build_object('schedule_type', 'non_shift', 'effective_from', :'from', 'pattern', 'Sen–Jum NS 09.00–18.00, Sab–Min libur'),
  'Go-live: karyawan kantor & gudang HQ/Warehouse (keputusan user 19-10-2026)'
FROM non_shift_targets t;
SELECT 'non_shift' AS tipe, count(*) FROM hris.employees WHERE schedule_type = 'non_shift'
UNION ALL SELECT 'pola NS mulai ' || :'from', count(DISTINCT employee_id) FROM hris.employee_shifts WHERE effective_from = :'from'::date;
COMMIT;
