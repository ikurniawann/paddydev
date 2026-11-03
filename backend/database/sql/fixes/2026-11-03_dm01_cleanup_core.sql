-- EPIC-083 / DM-01 — HAPUS PERMANEN data transaksi HRIS bertanggal ≤ 30 September 2026.
-- PERUBAHAN 03-11-2026 (keputusan user): batas dimundurkan ke ≤ 30 September 2026 (semula 20 Oktober);
--   pinjaman pending ikut dihapus ("oke delete aja"). Data 1 Oktober ke atas tidak disentuh.
--
-- Dasar: MOM 29 Oktober 2026 (WIT × Paddy) §2 No. 6, DM-01 ("database cleanup per tanggal
-- 20 Oktober 2026 … didahului backup") dan K-22. Jawaban user 02-11-2026:
--   - cakupan: data bertanggal ≤ 30 September 2026 (absensi, pengajuan absensi, cuti & izin,
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
-- Pemakaian: lewat tools/ops/dm01-cleanup.sh execute (runbook docs/hris/runbook-dm01-cleanup.md).
-- Runner menjalankannya dalam SATU transaksi (psql -1; tanpa -1 skrip berhenti di pemeriksaan
-- pertama) dan lebih dulu menyetel angka yang disetujui:
--   psql -1 -v ON_ERROR_STOP=1 -v expect_<tabel>=<n> … -f <pembungkus: set_config('dm01.expect_<tabel>',
--        :'expect_<tabel>', true) untuk keenam tabel, lalu \i 2026-11-03_dm01_cleanup_core.sql>
--
-- Isi (ruling EPIC-083):
--   R1 batas inklusif 30 Sep: attendance.date, attendance_requests.date, overtime_requests.date;
--      leaves.end_date, business_trips.end_date (yang melintasi 21 Okt tetap ada);
--      loans.requested_date (timestamptz, dibaca sebagai tanggal WIB).
--   R2 pinjaman dihapus hanya bila status pending / manager_approved (masih menunggu HR, sama
--      dengan pending) / rejected / cancelled / paid_off (lunas lewat payroll), atau approved dengan
--      sisa ≤ 0. Approved dengan sisa > 0, status lain, dan requested_date kosong TIDAK dihapus.
--   R3 leave_balances, leave_carry_over_*, setting, payroll, periode absensi, audit.audit_log,
--      notifikasi lama dan hris.attendance_adjustments (log, sengaja tanpa FK) tidak disentuh.
--   R4 trigger notifikasi absensi dan cuti dimatikan lalu dinyalakan lagi di transaksi yang sama
--      (skrip berhenti bila salah satunya tidak ada atau tidak sedang aktif, supaya ENABLE di akhir
--      tidak mengubah keadaan yang sengaja dibuat orang lain); penjaga periode tertutup dilewati hanya selama DELETE. Tanpa session_replication_role,
--      supaya FK tetap berjalan (attendance_requests.attendance_id → attendance: ON DELETE SET NULL,
--      satu-satunya FK yang merujuk enam tabel ini).
--   R5 satu transaksi; satu baris audit data_cleanup.dm01 berisi jumlah per tabel.
--   Angka yang disetujui: jumlah terhapus per tabel harus SAMA dengan dm01.expect_<tabel> (angka
--      HAPUS pratinjau yang disetujui HR, disetel runner tools/ops/dm01-cleanup.sh); beda → RAISE
--      sebelum COMMIT sehingga tidak ada yang terhapus. Tanpa angka skrip berhenti, kecuali
--      dm01.expect_skip = 'on' (hanya untuk tes).
-- Urutan: attendance_requests → attendance → leaves → overtime_requests → business_trips → loans.
-- ALTER TABLE … DISABLE/ENABLE TRIGGER memegang SHARE ROW EXCLUSIVE atas hris.attendance dan
-- hris.leaves sampai COMMIT: penulisan (clock-in, pengajuan cuti) tertahan, pembacaan tidak. Jalankan
-- di jam sepi. lock_timeout membuat skrip gagal cepat alih-alih mengantrikan request aplikasi.

-- Sama dengan sesi aplikasi (backend/internal/platform/database.SearchPath): role DB production
-- tidak menyetel search_path, sedangkan fungsi trigger memakai nama tabel tanpa schema.
SET LOCAL search_path = public, iam, configuration, hris, performance, recruitment, item, purchasing, inventory, manufacturing, pos, crm, accounting, auth;
SET LOCAL lock_timeout = '10s';
SET LOCAL dm01.cutoff = '2026-09-30';

-- Di luar transaksi (psql tanpa -1) SET LOCAL tidak berlaku: berhenti sebelum apa pun berubah.
DO $$
BEGIN
  IF COALESCE(current_setting('dm01.cutoff', true), '') = '' THEN
    RAISE EXCEPTION 'DM-01 harus berjalan dalam satu transaksi: psql -1 -v ON_ERROR_STOP=1 -f 2026-11-03_dm01_cleanup_core.sql';
  END IF;
END $$;

-- Kedua trigger notifikasi harus ada dan aktif ('O'); ENABLE di akhir selalu menyetel 'O'.
DO $$
DECLARE
  n int;
BEGIN
  SELECT count(*) INTO n FROM pg_trigger
   WHERE NOT tgisinternal AND tgenabled = 'O'
     AND ((tgrelid = 'hris.attendance'::regclass AND tgname = 'trg_notify_attendance_changes')
       OR (tgrelid = 'hris.leaves'::regclass AND tgname = 'trg_notify_leave_changes'));
  IF n <> 2 THEN
    RAISE EXCEPTION 'DM-01 dibatalkan: trigger trg_notify_attendance_changes (hris.attendance) dan trg_notify_leave_changes (hris.leaves) harus ada dan aktif sebelum cleanup; periksa pg_trigger.tgenabled dan pulihkan dulu';
  END IF;
END $$;

-- Angka yang disetujui wajib ada (bilangan bulat) untuk keenam tabel.
DO $$
DECLARE
  t text;
BEGIN
  IF current_setting('dm01.expect_skip', true) IS NOT DISTINCT FROM 'on' THEN
    RETURN;
  END IF;
  FOREACH t IN ARRAY ARRAY['attendance_requests', 'attendance', 'leaves', 'overtime_requests', 'business_trips', 'loans'] LOOP
    IF COALESCE(current_setting('dm01.expect_' || t, true), '') !~ '^[0-9]+$' THEN
      RAISE EXCEPTION 'DM-01 dibatalkan: angka yang disetujui untuk % tidak ada (dm01.expect_%); jalankan lewat tools/ops/dm01-cleanup.sh execute', t, t;
    END IF;
  END LOOP;
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
    AND (status IN ('pending', 'manager_approved', 'rejected', 'cancelled', 'paid_off')
         OR (status = 'approved' AND COALESCE(remaining_balance, 0) <= 0))
  RETURNING 1
)
SELECT set_config('dm01.n_loans', count(*)::text, true) AS loans_dihapus FROM d;

-- Jumlah terhapus harus sama dengan angka yang disetujui; beda → seluruh transaksi dibatalkan.
DO $$
DECLARE
  t text;
  beda text := '';
BEGIN
  IF current_setting('dm01.expect_skip', true) IS NOT DISTINCT FROM 'on' THEN
    RETURN;
  END IF;
  FOREACH t IN ARRAY ARRAY['attendance_requests', 'attendance', 'leaves', 'overtime_requests', 'business_trips', 'loans'] LOOP
    IF current_setting('dm01.n_' || t)::bigint <> current_setting('dm01.expect_' || t)::bigint THEN
      beda := beda || format(' %s terhapus %s, disetujui %s;', t, current_setting('dm01.n_' || t), current_setting('dm01.expect_' || t));
    END IF;
  END LOOP;
  IF beda <> '' THEN
    RAISE EXCEPTION 'DM-01 dibatalkan: jumlah terhapus beda dari angka yang disetujui —% tidak ada yang terhapus; ulangi pratinjau dan minta persetujuan lagi', beda;
  END IF;
END $$;

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
       'MOM 29 Okt 2026 DM-01/K-22; jawaban user 02-11-2026 (hapus permanen ≤ 30 Sep, pinjaman berjalan dan saldo cuti tetap)';
