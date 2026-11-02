-- EPIC-083 / DM-01 — HAPUS PERMANEN data transaksi HRIS bertanggal ≤ 20 Oktober 2026.
--
-- Dasar: MOM 29 Oktober 2026 (WIT × Paddy) §2 No. 6, DM-01 ("database cleanup per tanggal
-- 20 Oktober 2026 … didahului backup") dan K-22. Jawaban user 02-11-2026:
--   - cakupan: data bertanggal ≤ 20 Oktober 2026 (absensi, pengajuan absensi, cuti & izin,
--     lembur, perjalanan dinas, pinjaman/kasbon);
--   - hapus permanen, didahului backup penuh database dan file lampiran;
--   - pinjaman yang masih berjalan (approved, sisa > 0) tidak dihapus;
--   - saldo cuti (hris.leave_balances) tidak diubah.
--
-- WAJIB sebelum menjalankan: backup pg_dump -Fc + tar lampiran, pratinjau
-- (2026-11-03_dm01_preview.sql), dan — di production — persetujuan tertulis HR atas angka
-- pratinjau serta izin user untuk jam eksekusi (R8). File lampiran dihapus terpisah oleh runner
-- dari daftar FILE pratinjau; skrip ini hanya menyentuh database.
--
-- Pemakaian (SATU transaksi; tanpa -1 skrip berhenti di pemeriksaan pertama):
--   psql "$DATABASE_URL" -1 -v ON_ERROR_STOP=1 -f backend/database/sql/fixes/2026-11-03_dm01_cleanup_core.sql
--
-- Isi (ruling EPIC-083):
--   R1 batas inklusif 20 Okt: attendance.date, attendance_requests.date, overtime_requests.date;
--      leaves.end_date, business_trips.end_date (yang melintasi 21 Okt tetap ada);
--      loans.requested_date (timestamptz, dibaca sebagai tanggal WIB).
--   R2 pinjaman dihapus hanya bila status pending / rejected / cancelled / paid_off (lunas lewat
--      payroll), atau approved dengan sisa ≤ 0. Approved dengan sisa > 0, manager_approved (masih
--      menunggu HR), status lain, dan requested_date kosong TIDAK dihapus.
--   R3 leave_balances, leave_carry_over_*, setting, payroll, periode absensi, audit.audit_log,
--      notifikasi lama dan hris.attendance_adjustments (log, sengaja tanpa FK) tidak disentuh.
--   R4 trigger notifikasi absensi dan cuti dimatikan lalu dinyalakan lagi di transaksi yang sama;
--      penjaga periode tertutup dilewati hanya selama DELETE. Tanpa session_replication_role,
--      supaya FK tetap berjalan (attendance_requests.attendance_id → attendance: ON DELETE SET NULL,
--      satu-satunya FK yang merujuk enam tabel ini).
--   R5 satu transaksi; satu baris audit data_cleanup.dm01 berisi jumlah per tabel.
-- Urutan: attendance_requests → attendance → leaves → overtime_requests → business_trips → loans.
-- ALTER TABLE mengunci hris.attendance dan hris.leaves (ACCESS EXCLUSIVE) sampai COMMIT: jalankan
-- di jam sepi. lock_timeout membuat skrip gagal cepat alih-alih mengantrikan request aplikasi.

-- Sama dengan sesi aplikasi (backend/internal/platform/database.SearchPath): role DB production
-- tidak menyetel search_path, sedangkan fungsi trigger memakai nama tabel tanpa schema.
SET LOCAL search_path = public, iam, configuration, hris, performance, recruitment, item, purchasing, inventory, manufacturing, pos, crm, accounting, auth;
SET LOCAL lock_timeout = '10s';
SET LOCAL dm01.cutoff = '2026-10-20';

-- Di luar transaksi (psql tanpa -1) SET LOCAL tidak berlaku: berhenti sebelum apa pun berubah.
DO $$
BEGIN
  IF COALESCE(current_setting('dm01.cutoff', true), '') = '' THEN
    RAISE EXCEPTION 'DM-01 harus berjalan dalam satu transaksi: psql -1 -v ON_ERROR_STOP=1 -f 2026-11-03_dm01_cleanup_core.sql';
  END IF;
END $$;

SET LOCAL hris.allow_closed_period_write = 'on';
ALTER TABLE hris.attendance DISABLE TRIGGER trg_notify_attendance_changes;
ALTER TABLE hris.leaves DISABLE TRIGGER trg_notify_leave_changes;

WITH d AS (
  DELETE FROM hris.attendance_requests
  WHERE date <= current_setting('dm01.cutoff')::date
  RETURNING 1
)
SELECT set_config('dm01.n_attendance_requests', count(*)::text, true) AS attendance_requests_dihapus FROM d;

WITH d AS (
  DELETE FROM hris.attendance
  WHERE date <= current_setting('dm01.cutoff')::date
  RETURNING 1
)
SELECT set_config('dm01.n_attendance', count(*)::text, true) AS attendance_dihapus FROM d;

WITH d AS (
  DELETE FROM hris.leaves
  WHERE end_date <= current_setting('dm01.cutoff')::date
  RETURNING 1
)
SELECT set_config('dm01.n_leaves', count(*)::text, true) AS leaves_dihapus FROM d;

WITH d AS (
  DELETE FROM hris.overtime_requests
  WHERE date <= current_setting('dm01.cutoff')::date
  RETURNING 1
)
SELECT set_config('dm01.n_overtime_requests', count(*)::text, true) AS overtime_requests_dihapus FROM d;

WITH d AS (
  DELETE FROM hris.business_trips
  WHERE end_date <= current_setting('dm01.cutoff')::date
  RETURNING 1
)
SELECT set_config('dm01.n_business_trips', count(*)::text, true) AS business_trips_dihapus FROM d;

WITH d AS (
  DELETE FROM hris.loans
  WHERE (requested_date AT TIME ZONE 'Asia/Jakarta')::date <= current_setting('dm01.cutoff')::date
    AND (status IN ('pending', 'rejected', 'cancelled', 'paid_off')
         OR (status = 'approved' AND COALESCE(remaining_balance, 0) <= 0))
  RETURNING 1
)
SELECT set_config('dm01.n_loans', count(*)::text, true) AS loans_dihapus FROM d;

ALTER TABLE hris.attendance ENABLE TRIGGER trg_notify_attendance_changes;
ALTER TABLE hris.leaves ENABLE TRIGGER trg_notify_leave_changes;
SET LOCAL hris.allow_closed_period_write = 'off';

INSERT INTO audit.audit_log (actor_id, actor_name, action, entity, entity_id, entity_label, after, reason)
SELECT NULL, 'Sistem (cleanup DM-01)', 'data_cleanup.dm01', 'hris', 'dm01',
       'Cleanup data HRIS ≤ ' || current_setting('dm01.cutoff'),
       jsonb_build_object(
         'cutoff', current_setting('dm01.cutoff'),
         'attendance_requests', current_setting('dm01.n_attendance_requests')::int,
         'attendance', current_setting('dm01.n_attendance')::int,
         'leaves', current_setting('dm01.n_leaves')::int,
         'overtime_requests', current_setting('dm01.n_overtime_requests')::int,
         'business_trips', current_setting('dm01.n_business_trips')::int,
         'loans', current_setting('dm01.n_loans')::int),
       'MOM 29 Okt 2026 DM-01/K-22; jawaban user 02-11-2026 (hapus permanen ≤ 20 Okt, pinjaman berjalan dan saldo cuti tetap)';
