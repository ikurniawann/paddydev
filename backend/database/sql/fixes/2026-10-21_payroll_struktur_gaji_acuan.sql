-- =============================================================================
-- EPIC-059: struktur gaji aktif karyawan bulanan ke acuan payslip Paddy:
-- pay_basis monthly → monthly_prorated; transport/makan/jabatan → per_day.
-- Upah harian (daily) tidak disentuh. Jalankan HANYA setelah user menyetujui
-- pratinjau dan simulasi payroll.
-- Pratinjau (transaksi read-only + ROLLBACK):
--   psql "$U" -v ON_ERROR_STOP=1 -v preview=1 -f backend/database/sql/fixes/2026-10-21_payroll_struktur_gaji_acuan.sql
-- Terapkan (setelah backup):
--   psql "$U" -v ON_ERROR_STOP=1 -f backend/database/sql/fixes/2026-10-21_payroll_struktur_gaji_acuan.sql
-- =============================================================================
BEGIN;
CREATE TEMP VIEW struktur_usulan AS
SELECT s.id, e.full_name, s.pay_basis, s.transport_mode, s.meal_mode, s.position_mode, s.working_days_divisor,
  (SELECT count(*) FROM hris.attendance a
    WHERE a.employee_id = e.id AND a.date >= (current_date - 30) AND a.status <> 'absent') AS absensi_30_hari,
  EXISTS (SELECT 1 FROM hris.employee_shifts sh
    WHERE sh.employee_id = e.id AND sh.effective_from <= current_date
      AND (sh.effective_to IS NULL OR sh.effective_to >= current_date)) AS punya_jadwal
FROM hris.employee_salary s
JOIN hris.employees e ON e.id = s.employee_id AND e.is_active
WHERE s.is_active AND coalesce(s.pay_basis, 'monthly') IN ('monthly', 'monthly_prorated')
  AND (coalesce(s.pay_basis, 'monthly') <> 'monthly_prorated'
       OR coalesce(s.transport_mode, 'flat') <> 'per_day'
       OR coalesce(s.meal_mode, 'flat') <> 'per_day'
       OR coalesce(s.position_mode, 'flat') <> 'per_day');

\if :{?preview}
  SET TRANSACTION READ ONLY;
\endif

\echo 'Struktur yang akan diubah:'
SELECT count(*) AS struktur, count(*) FILTER (WHERE absensi_30_hari = 0) AS tanpa_absensi_30_hari,
  count(*) FILTER (WHERE NOT punya_jadwal) AS tanpa_jadwal FROM struktur_usulan;
\echo 'Daftar struktur yang akan diubah (nilai sekarang → monthly_prorated / per_day / per_day / per_day):'
SELECT full_name, pay_basis, transport_mode, meal_mode, position_mode, absensi_30_hari, punya_jadwal
FROM struktur_usulan ORDER BY full_name;
\echo 'PERINGATAN — karyawan tanpa absensi 30 hari terakhir (gaji pokok prorated akan 0 tanpa absensi/cuti):'
SELECT full_name, pay_basis, absensi_30_hari FROM struktur_usulan WHERE absensi_30_hari = 0 ORDER BY full_name;
\echo 'PERINGATAN — karyawan TANPA jadwal (tidak ada baris hris.employee_shifts yang berlaku hari ini):'
\echo '  pembagi = pembagi struktur (bila kosong: hari hadir), bukan hari terjadwal; atur jadwal sebelum menerapkan.'
SELECT full_name, pay_basis, working_days_divisor FROM struktur_usulan WHERE NOT punya_jadwal ORDER BY full_name;

\if :{?preview}
\else
  UPDATE hris.employee_salary s SET pay_basis = 'monthly_prorated', transport_mode = 'per_day',
    meal_mode = 'per_day', position_mode = 'per_day', updated_at = now()
  FROM struktur_usulan u WHERE u.id = s.id;
\endif

\if :{?preview}
  ROLLBACK;
\else
  COMMIT;
\endif
