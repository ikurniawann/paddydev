-- EPIC-067 (R13) — cek hanya-baca: siapa yang boleh memutus sisi HR sebelum dan sesudah
-- aksi "approve" (migrasi 20261027090100_iam_hris_approve). Tidak menulis apa pun.
--
-- Jalankan sebelum DAN sesudah migrasi (staging, gladi salinan production):
--   psql "$DATABASE_URL" -f backend/database/sql/checks/2026-10-26_epic067_approve_access.sql
-- Gerbang: tidak ada baris daftar = 'lost'. Bila ada, berhenti dan laporkan; jangan menambah
-- grant ke role non-HR untuk menutupnya.
--
-- Satu query, kolom pertama `daftar`, urutan: lost, gained, akun.
--   lost   : sebelum ya, sesudah tidak (salah satu keputusan)
--   gained : sebelum tidak, sesudah ya
--   akun   : semua akun aktif yang sebelum atau sesudah memegang salah satu keputusan
-- Akun aktif: configuration.users.status = 'active' dan auth.users tidak diblokir.
--
-- Sebelum (kode lama): users.role super_admin/admin/hrd. Pengajuan absensi juga butuh menu
--   berprefiks hris.workforce.attendance (grant aktif apa pun aksinya).
-- Sesudah: "approve" pada daun (atau turunannya) lewat role IAM yang di-resolve, sama dengan
--   auth.ResolveRoleIDs + GrantedMenuActions + iam.HasGrantedAction:
--   iam.user_roles; bila kosong, iam.roles dengan code = users.role (tidak terhapus);
--   grant is_active; menu aktif dan tidak terhapus; granted_actions berupa array.
--   Perjalanan Dinas memakai daun Absensi (R10).
BEGIN READ ONLY;

WITH accounts AS (
  SELECT cu.id AS user_id, coalesce(cu.email, au.email) AS email, cu.role
  FROM configuration.users cu
  JOIN auth.users au ON au.id = cu.id
  WHERE cu.status = 'active'
    AND (au.banned_until IS NULL OR au.banned_until < now())
),
resolved AS (
  SELECT a.user_id, ur.role_id
  FROM accounts a
  JOIN iam.user_roles ur ON ur.user_id = a.user_id
  UNION ALL
  SELECT a.user_id, (SELECT r.id FROM iam.roles r WHERE r.code = a.role AND r.deleted_at IS NULL LIMIT 1)
  FROM accounts a
  WHERE NOT EXISTS (SELECT 1 FROM iam.user_roles ur WHERE ur.user_id = a.user_id)
),
held AS (
  SELECT ro.user_id, m.code,
         jsonb_typeof(rmp.granted_actions) = 'array' AND rmp.granted_actions ? 'approve' AS approve
  FROM resolved ro
  JOIN iam.role_menu_permissions rmp ON rmp.role_id = ro.role_id AND rmp.is_active
  JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL AND m.is_active
),
leaf AS (
  SELECT h.user_id,
    bool_or(h.code = 'hris.workforce.attendance' OR starts_with(h.code, 'hris.workforce.attendance.')) AS attendance_menu,
    bool_or(h.approve AND (h.code = 'hris.workforce.attendance' OR starts_with(h.code, 'hris.workforce.attendance.'))) AS attendance,
    bool_or(h.approve AND (h.code = 'hris.workforce.leaves' OR starts_with(h.code, 'hris.workforce.leaves.'))) AS leaves,
    bool_or(h.approve AND (h.code = 'hris.kepegawaian.overtime' OR starts_with(h.code, 'hris.kepegawaian.overtime.'))) AS overtime
  FROM held h
  GROUP BY h.user_id
),
access AS (
  SELECT a.user_id, a.email, a.role,
    a.role IN ('super_admin', 'admin', 'hrd') AND coalesce(l.attendance_menu, false) AS before_attendance,
    coalesce(l.attendance, false) AS after_attendance,
    a.role IN ('super_admin', 'admin', 'hrd') AS before_trip,
    coalesce(l.attendance, false) AS after_trip,
    a.role IN ('super_admin', 'admin', 'hrd') AS before_leave,
    coalesce(l.leaves, false) AS after_leave,
    a.role IN ('super_admin', 'admin', 'hrd') AS before_overtime,
    coalesce(l.overtime, false) AS after_overtime
  FROM accounts a
  LEFT JOIN leaf l ON l.user_id = a.user_id
)
SELECT 'lost' AS daftar, x.* FROM access x
WHERE (x.before_attendance AND NOT x.after_attendance) OR (x.before_trip AND NOT x.after_trip)
   OR (x.before_leave AND NOT x.after_leave) OR (x.before_overtime AND NOT x.after_overtime)
UNION ALL
SELECT 'gained', x.* FROM access x
WHERE (x.after_attendance AND NOT x.before_attendance) OR (x.after_trip AND NOT x.before_trip)
   OR (x.after_leave AND NOT x.before_leave) OR (x.after_overtime AND NOT x.before_overtime)
UNION ALL
SELECT 'akun', x.* FROM access x
WHERE x.before_attendance OR x.after_attendance OR x.before_trip OR x.after_trip
   OR x.before_leave OR x.after_leave OR x.before_overtime OR x.after_overtime
ORDER BY daftar DESC, email;

ROLLBACK;
