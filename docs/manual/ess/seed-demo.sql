-- Data demo Manual Book ESS Karyawan (khusus DB TEST, jangan dijalankan ke DB live).
--
-- Membuat dua orang fiktif beserta riwayatnya supaya screenshot manual tidak kosong:
--   * Rina Lestari  — Sales Associate (Staff), penempatan utama Paddy Store PVJ
--                     (+ Paddy Store Kopo), atasan langsung Andi Pratama.
--   * Andi Pratama  — Store Manager (level Manager): Izin Leader muncul dan ia
--                     menjadi penyetuju kasbon & Perjalanan Dinas tim.
--
-- Password TIDAK disimpan di file ini. Kirim hash bcrypt lewat variabel psql:
--   psql "$TEST_DB" -v pw_hash="$HASH" -f seed-demo.sql
-- Tanggal dihitung relatif dari CURRENT_DATE, jadi seed bisa diulang kapan saja.
-- Semua baris demo dihapus oleh cleanup-demo.sql (jalankan cleanup dulu bila
-- seed pernah dijalankan).

\set ON_ERROR_STOP on

BEGIN;

-- ID tetap supaya cleanup dan capture.cjs mudah menemukannya.
CREATE TEMP TABLE demo_ids ON COMMIT DROP AS SELECT
  'de5a0000-0000-4000-8000-000000000001'::uuid AS rina_user,
  'de5a0000-0000-4000-8000-000000000002'::uuid AS andi_user,
  'de5a0000-0000-4000-8000-000000000011'::uuid AS rina_emp,
  'de5a0000-0000-4000-8000-000000000012'::uuid AS andi_emp,
  'de5a0000-0000-4000-8000-000000000021'::uuid AS payroll_run,
  (SELECT id FROM configuration.branches WHERE code = 'PD-PVJ') AS pvj,
  (SELECT id FROM configuration.branches WHERE code = 'PD-KOPO') AS kopo,
  (SELECT company_id FROM configuration.branches WHERE code = 'PD-PVJ') AS company,
  (SELECT id FROM hris.departments WHERE code = 'RETAIL-STORE') AS dept,
  (SELECT id FROM hris.positions WHERE title = 'Sales Associate' AND is_active ORDER BY created_at LIMIT 1) AS pos_staff,
  (SELECT id FROM hris.positions WHERE title = 'Store Manager' AND lower(level) = 'manager' AND is_active ORDER BY created_at LIMIT 1) AS pos_manager,
  (SELECT id FROM hris.shifts WHERE name = 'Shift Pagi' AND start_time = '09:00' AND is_active LIMIT 1) AS shift_pagi,
  (SELECT id FROM iam.roles WHERE code = 'employee') AS role_employee;

DO $$
DECLARE r record;
BEGIN
  SELECT * INTO r FROM demo_ids;
  IF r.pvj IS NULL OR r.kopo IS NULL OR r.dept IS NULL OR r.pos_staff IS NULL OR r.pos_manager IS NULL
     OR r.shift_pagi IS NULL OR r.role_employee IS NULL THEN
    RAISE EXCEPTION 'Master data DB test tidak lengkap (toko PVJ/Kopo, dept RETAIL-STORE, posisi, Shift Pagi 09:00, role employee): %', row_to_json(r);
  END IF;
END $$;

-- ── Akun login ───────────────────────────────────────────────────────────
INSERT INTO auth.users (id, email, password_hash, email_verified_at, raw_user_meta_data)
SELECT rina_user, 'rina.lestari.demo@example.com', :'pw_hash', now(), '{"full_name":"Rina Lestari","demo":"manual-ess"}'::jsonb FROM demo_ids
UNION ALL
SELECT andi_user, 'andi.pratama.demo@example.com', :'pw_hash', now(), '{"full_name":"Andi Pratama","demo":"manual-ess"}'::jsonb FROM demo_ids;

INSERT INTO configuration.users (id, full_name, role, email, status, company_id, branch_id)
SELECT rina_user, 'Rina Lestari', 'employee', 'rina.lestari.demo@example.com', 'active', company, pvj FROM demo_ids
UNION ALL
SELECT andi_user, 'Andi Pratama', 'employee', 'andi.pratama.demo@example.com', 'active', company, pvj FROM demo_ids;

INSERT INTO iam.user_roles (user_id, role_id, is_primary)
SELECT rina_user, role_employee, true FROM demo_ids
UNION ALL
SELECT andi_user, role_employee, true FROM demo_ids;

-- ── Data karyawan ────────────────────────────────────────────────────────
INSERT INTO hris.employees (id, user_id, full_name, nip, email, phone, gender, join_date, employment_status,
  department_id, job_title_id, reporting_to, notes, is_access_app, schedule_type)
SELECT andi_emp, andi_user, 'Andi Pratama', 'DM-902', 'andi.pratama.demo@example.com', '080000000002', 'male',
  DATE '2021-07-12', 'permanent', dept, pos_manager, NULL, 'DEMO manual ESS — dihapus oleh cleanup-demo.sql', true, 'shift'
FROM demo_ids;

INSERT INTO hris.employees (id, user_id, full_name, nip, email, phone, gender, join_date, employment_status,
  department_id, job_title_id, reporting_to, notes, is_access_app, schedule_type)
SELECT rina_emp, rina_user, 'Rina Lestari', 'DM-901', 'rina.lestari.demo@example.com', '080000000001', 'female',
  DATE '2024-03-04', 'permanent', dept, pos_staff, andi_emp, 'DEMO manual ESS — dihapus oleh cleanup-demo.sql', true, 'shift'
FROM demo_ids;

INSERT INTO hris.employee_placements (employee_id, branch_id, is_primary, start_date, notes)
SELECT rina_emp, pvj, true, DATE '2024-03-04', 'DEMO manual ESS' FROM demo_ids
UNION ALL SELECT rina_emp, kopo, false, DATE '2025-01-06', 'DEMO manual ESS' FROM demo_ids
UNION ALL SELECT andi_emp, pvj, true, DATE '2021-07-12', 'DEMO manual ESS' FROM demo_ids;

-- Jadwal: Senin–Sabtu Shift Pagi 09:00–17:00, Minggu libur (tanpa baris).
INSERT INTO hris.employee_shifts (employee_id, day_of_week, shift_id, effective_from, created_by_name)
SELECT e, d, shift_pagi, (CURRENT_DATE - 60), 'DEMO manual ESS'
FROM demo_ids, unnest(ARRAY[rina_emp, andi_emp]) AS e, generate_series(1, 6) AS d;

-- Gaji aktif (dipakai cek batas kasbon 50% THP).
INSERT INTO hris.employee_salary (employee_id, base_salary, effective_date, transport_allowance, meal_allowance, is_active)
SELECT rina_emp, 3500000, DATE '2024-03-04', 300000, 400000, true FROM demo_ids
UNION ALL SELECT andi_emp, 7500000, DATE '2021-07-12', 500000, 500000, true FROM demo_ids;

-- ── Riwayat Rina (hari kerja terbuka dalam 24 hari terakhir) ──────────────────────────────────────
-- Hari kerja (Senin–Sabtu) sebelum hari ini, diberi nomor urut dari yang terlama.
CREATE TEMP TABLE demo_days ON COMMIT DROP AS
SELECT d::date AS day, row_number() OVER (ORDER BY d) AS n
FROM generate_series(CURRENT_DATE - 24, CURRENT_DATE - 1, interval '1 day') AS d
WHERE extract(isodow FROM d) < 7
  -- Periode absensi yang sudah di-closing tidak bisa diisi (trigger guard).
  AND NOT EXISTS (SELECT 1 FROM hris.attendance_periods ap
                  WHERE ap.status = 'closed' AND d::date BETWEEN ap.start_date AND ap.end_date);

-- Peran tiap hari (dipilih berurutan supaya cuti & dinas jatuh di Senin–Jumat):
--   leave = Cuti Tahunan 2 hari (disetujui), overtime = lembur 2 jam,
--   late = terlambat 23 menit, trip = Perjalanan Dinas 2 hari (disetujui),
--   sick = izin Sakit yang ditolak (tidak hadir). Hari lain: hadir tepat waktu.
ALTER TABLE demo_days ADD COLUMN role text;
DO $$
DECLARE d_leave date; d_ot date; d_late date; d_trip date; d_sick date;
BEGIN
  SELECT a.day INTO d_leave FROM demo_days a JOIN demo_days b ON b.day = a.day + 1
   WHERE a.n >= 2 AND extract(isodow FROM a.day) <= 4 ORDER BY a.day LIMIT 1;
  SELECT day INTO d_ot FROM demo_days WHERE day > d_leave + 2 ORDER BY day LIMIT 1;
  SELECT day INTO d_late FROM demo_days WHERE day > d_ot + 1 ORDER BY day LIMIT 1;
  SELECT a.day INTO d_trip FROM demo_days a JOIN demo_days b ON b.day = a.day + 1
   WHERE a.day > d_late + 1 AND extract(isodow FROM a.day) <= 4 ORDER BY a.day LIMIT 1;
  SELECT day INTO d_sick FROM demo_days WHERE day > d_trip + 2 ORDER BY day LIMIT 1;
  IF d_sick IS NULL THEN
    RAISE EXCEPTION 'Hari kerja terbuka sebelum hari ini kurang (ada %). Jalankan seed setelah tanggal ±17, atau buka periode absensi bulan lalu di DB test.', (SELECT count(*) FROM demo_days);
  END IF;
  UPDATE demo_days SET role = 'leave' WHERE day IN (d_leave, d_leave + 1);
  UPDATE demo_days SET role = 'overtime' WHERE day = d_ot;
  UPDATE demo_days SET role = 'late' WHERE day = d_late;
  UPDATE demo_days SET role = 'trip' WHERE day IN (d_trip, d_trip + 1);
  UPDATE demo_days SET role = 'sick' WHERE day = d_sick;
END $$;

INSERT INTO hris.attendance (employee_id, date, clock_in, clock_out, work_hours, status, is_late, late_minutes,
  shift_id, scheduled_start, scheduled_end, branch_id, clock_in_geofence, clock_out_geofence, notes,
  is_overtime, overtime_hours)
SELECT i.rina_emp, dd.day,
  (dd.day + time '09:00' + CASE WHEN dd.role = 'late' THEN interval '23 minutes' ELSE interval '-6 minutes' + (dd.n % 5) * interval '1 minute' END) AT TIME ZONE 'Asia/Jakarta',
  (dd.day + time '17:00' + CASE WHEN dd.role = 'overtime' THEN interval '2 hours 4 minutes' ELSE (dd.n % 4) * interval '3 minutes' END) AT TIME ZONE 'Asia/Jakarta',
  CASE WHEN dd.role = 'overtime' THEN 9.0 ELSE 7.0 END,
  CASE WHEN dd.role = 'late' THEN 'late'::attendance_status ELSE 'present'::attendance_status END,
  coalesce(dd.role = 'late', false), CASE WHEN dd.role = 'late' THEN 23 ELSE 0 END,
  i.shift_pagi,
  (dd.day + time '09:00') AT TIME ZONE 'Asia/Jakarta', (dd.day + time '17:00') AT TIME ZONE 'Asia/Jakarta',
  i.pvj,
  CASE WHEN dd.role = 'trip' THEN 'dinas' ELSE 'unchecked' END,
  CASE WHEN dd.role = 'trip' THEN 'dinas' ELSE 'unchecked' END,
  'DEMO manual ESS', coalesce(dd.role = 'overtime', false), CASE WHEN dd.role = 'overtime' THEN 2 ELSE 0 END
FROM demo_ids i, demo_days dd
WHERE dd.role IS DISTINCT FROM 'leave' AND dd.role IS DISTINCT FROM 'sick';

-- Izin & cuti
INSERT INTO hris.leaves (employee_id, leave_type, start_date, end_date, total_days, reason, status, approved_by, approved_at, rejection_reason)
SELECT rina_emp, 'annual'::leave_type, (SELECT min(day) FROM demo_days WHERE role = 'leave'), (SELECT max(day) FROM demo_days WHERE role = 'leave'), 2,
  'Acara keluarga di Garut', 'approved'::leave_status, andi_emp, now() - interval '20 days', NULL FROM demo_ids
UNION ALL
SELECT rina_emp, 'sick'::leave_type, (SELECT day FROM demo_days WHERE role = 'sick'), (SELECT day FROM demo_days WHERE role = 'sick'), 1,
  'Demam, sudah ke klinik (surat dokter menyusul)', 'rejected'::leave_status, andi_emp, now() - interval '5 days',
  'Surat dokter belum dilampirkan, mohon ajukan ulang dengan lampiran' FROM demo_ids
UNION ALL
-- Izin Pribadi: hari kerja berikutnya setelah besok (hari Minggu dilewati).
SELECT rina_emp, 'personal'::leave_type, x.d, x.d, 1, 'Mengurus perpanjangan SIM di Polrestabes', 'pending'::leave_status, NULL, NULL, NULL
FROM demo_ids, LATERAL (SELECT CASE WHEN extract(isodow FROM CURRENT_DATE + 2) = 7 THEN CURRENT_DATE + 3 ELSE CURRENT_DATE + 2 END AS d) x;


INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total, annual_leave_used)
SELECT rina_emp, extract(year FROM CURRENT_DATE)::int, 12, 2 FROM demo_ids
UNION ALL SELECT andi_emp, extract(year FROM CURRENT_DATE)::int, 12, 3 FROM demo_ids;

-- Pinjaman & kasbon: satu kasbon berjalan, satu kasbon menunggu persetujuan Andi.
INSERT INTO hris.loans (employee_id, loan_type, principal_amount, interest_rate, tenor_months, requested_date, created_at,
  approved_by, approved_at, disbursed_at, disbursed_amount, monthly_installment, first_installment_month,
  first_installment_year, remaining_balance, paid_amount, status, purpose, notes,
  manager_approver_id, manager_approved_by, manager_approved_at)
SELECT rina_emp, 'kasbon', 1000000, 0, 2, now() - interval '45 days', now() - interval '45 days',
  NULL::uuid, now() - interval '42 days', now() - interval '41 days', 1000000, 500000,
  extract(month FROM CURRENT_DATE - 30)::int, extract(year FROM CURRENT_DATE - 30)::int, 500000, 500000,
  'approved', 'Biaya daftar ulang sekolah adik', 'DEMO manual ESS', andi_emp, andi_emp, now() - interval '44 days'
FROM demo_ids
UNION ALL
SELECT rina_emp, 'kasbon', 600000, 0, 1, now() - interval '1 day', now() - interval '1 day',
  NULL, NULL, NULL, NULL, 600000, NULL, NULL, 600000, 0,
  'pending', 'Servis motor untuk berangkat kerja', 'DEMO manual ESS', andi_emp, NULL, NULL
FROM demo_ids;

-- Perjalanan Dinas: satu disetujui (hari 'trip'), satu menunggu (minggu depan).
INSERT INTO hris.business_trips (employee_id, start_date, end_date, destination, purpose, status, requested_by,
  decided_by, decided_by_name, decided_at)
SELECT rina_emp, (SELECT min(day) FROM demo_days WHERE role = 'trip'), (SELECT max(day) FROM demo_days WHERE role = 'trip'),
  'Gudang Pusat Cikarang', 'Membantu stock opname gudang pusat', 'approved', rina_emp, andi_emp, 'Andi Pratama',
  now() - interval '12 days' FROM demo_ids
UNION ALL
SELECT rina_emp, CURRENT_DATE + 7, CURRENT_DATE + 8, 'Paddy Store Kopo', 'Membantu persiapan promo akhir bulan',
  'pending', rina_emp, NULL, NULL, NULL FROM demo_ids;

-- Lembur: satu disetujui (hari 'overtime'), satu penugasan dari perusahaan menunggu konfirmasi.
INSERT INTO hris.overtime_requests (employee_id, date, start_time, end_time, hours, source, status, reason,
  requested_by, decided_by, decided_at)
SELECT rina_emp, (SELECT day FROM demo_days WHERE role = 'overtime'), time '17:00', time '19:00', 2, 'employee', 'approved',
  'Closing stok bulanan', rina_emp, andi_emp, now() - interval '15 days' FROM demo_ids
UNION ALL
SELECT rina_emp, CURRENT_DATE + 3, time '17:00', time '20:00', 3, 'company', 'pending',
  'Persiapan display koleksi baru', andi_emp, NULL, NULL FROM demo_ids;

-- Slip gaji: satu run payroll "paid" khusus demo (bulan lalu, entitas toko).
INSERT INTO hris.payroll_runs (id, run_name, period_month, period_year, status, processed_at, paid_at,
  total_employees, total_gross, total_deductions, total_net, notes, company_id)
SELECT payroll_run, 'DEMO Manual ESS',
  extract(month FROM CURRENT_DATE - interval '1 month')::int, extract(year FROM CURRENT_DATE - interval '1 month')::int,
  'paid', now() - interval '20 days', now() - interval '18 days', 1, 4200000, 640000, 3560000,
  'DEMO manual ESS — dihapus oleh cleanup-demo.sql', company
FROM demo_ids;

INSERT INTO hris.payroll_details (payroll_run_id, employee_id, base_salary, transport_allowance, meal_allowance,
  overtime_pay, gross_salary, bpjs_tk_jht_deduction, bpjs_tk_jp_deduction, bpjs_kes_deduction, loan_deduction,
  total_deductions, net_salary, working_days, present_days, late_days, overtime_hours, status, notes, full_base_salary)
SELECT payroll_run, rina_emp, 3500000, 300000, 400000, 0, 4200000, 70000, 35000, 35000, 500000,
  640000, 3560000, 26, 25, 1, 0, 'paid', 'DEMO manual ESS', 3500000
FROM demo_ids;

COMMIT;

SELECT e.full_name, e.nip, u.email, p.title AS jabatan, p.level
FROM hris.employees e JOIN auth.users u ON u.id = e.user_id LEFT JOIN hris.positions p ON p.id = e.job_title_id
WHERE e.id IN ('de5a0000-0000-4000-8000-000000000011', 'de5a0000-0000-4000-8000-000000000012');
