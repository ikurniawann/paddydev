-- EPIC-068 (R7) — cek hanya-baca: siapa yang boleh melihat dan mengubah saldo cuti sebelum dan
-- sesudah menu "Saldo Cuti" (migrasi 20261028090000_leave_balance_allocation). Tidak menulis apa pun.
--
-- Jalankan sebelum DAN sesudah migrasi (staging, gladi salinan production):
--   psql "$DATABASE_URL" -f backend/database/sql/checks/2026-10-28_epic068_leave_balance_access.sql
--
-- Run SEBELUM migrasi hanya informasi, BUKAN gerbang: kolom after_* membaca grant saat ini, jadi
--   sebelum menu ada semua akun HR muncul sebagai 'lost'. Simpan hasilnya sebagai pembanding.
--
-- Gerbang 1: run SESUDAH migrasi menghasilkan NOL baris daftar = 'lost'. Bila ada, berhenti dan
--   laporkan; jangan menambah grant untuk menutupnya. Pengecualian yang DIHARAPKAN oleh R7: akun
--   users.role = 'hrd' kehilangan "ubah saldo" (cek role dihapus, hak ada di daun baru) bila role
--   IAM-nya tidak memegang "update" di daun; tampilkan ke user, jangan putuskan sendiri.
-- Gerbang 2: setiap baris daftar = 'gained' wajib ditinjau. Diharapkan: super_admin dan admin;
--   admin_hris dan hris_manager (read, update, import, export); hris_non_payroll dan
--   hris_non_rekrutmen_kpi hanya read. Tunjukkan daftar 'gained' ke user dan tunggu persetujuan.
--
-- Satu query, kolom pertama `daftar`, urutan: lost, gained, akun.
--   lost   : sebelum ya, sesudah tidak (melihat saldo karyawan lain atau mengubah saldo)
--   gained : sebelum tidak, sesudah ya (juga import dan export, yang belum ada sebelumnya)
--   akun   : semua akun aktif yang sebelum atau sesudah memegang salah satu hak
-- Akun aktif: configuration.users.status = 'active' dan auth.users tidak diblokir.
--
-- Sebelum (kode lama):
--   lihat saldo karyawan lain : users.role hrd atau hiring_manager (pemilik selalu boleh)
--   ubah saldo                : users.role hrd
-- Sesudah (R7), role IAM yang di-resolve sama dengan auth.ResolveRoleIDs + GrantedMenuActions:
--   iam.user_roles; bila kosong, iam.roles dengan code = users.role (tidak terhapus);
--   grant is_active; menu aktif dan tidak terhapus; granted_actions berupa array.
--   lihat saldo karyawan lain : "read" di hris.workforce.leave-balances, atau menu berprefiks
--                               hris.kepegawaian.users (grant aktif apa pun aksinya)
--   ubah / import / export    : "update" / "import" / "export" di hris.workforce.leave-balances
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
  SELECT ro.user_id, m.code, rmp.granted_actions,
         jsonb_typeof(rmp.granted_actions) = 'array' AS is_array
  FROM resolved ro
  JOIN iam.role_menu_permissions rmp ON rmp.role_id = ro.role_id AND rmp.is_active
  JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL AND m.is_active
),
leaf AS (
  SELECT h.user_id,
    bool_or(h.code = 'hris.kepegawaian.users' OR starts_with(h.code, 'hris.kepegawaian.users.')) AS karyawan_menu,
    bool_or(h.is_array AND h.granted_actions ? 'read'
            AND (h.code = 'hris.workforce.leave-balances' OR starts_with(h.code, 'hris.workforce.leave-balances.'))) AS leaf_read,
    bool_or(h.is_array AND h.granted_actions ? 'update'
            AND (h.code = 'hris.workforce.leave-balances' OR starts_with(h.code, 'hris.workforce.leave-balances.'))) AS leaf_update,
    bool_or(h.is_array AND h.granted_actions ? 'import'
            AND (h.code = 'hris.workforce.leave-balances' OR starts_with(h.code, 'hris.workforce.leave-balances.'))) AS leaf_import,
    bool_or(h.is_array AND h.granted_actions ? 'export'
            AND (h.code = 'hris.workforce.leave-balances' OR starts_with(h.code, 'hris.workforce.leave-balances.'))) AS leaf_export
  FROM held h
  GROUP BY h.user_id
),
access AS (
  SELECT a.user_id, a.email, a.role,
    a.role IN ('hrd', 'hiring_manager') AS before_view,
    coalesce(l.leaf_read, false) OR coalesce(l.karyawan_menu, false) AS after_view,
    a.role = 'hrd' AS before_update,
    coalesce(l.leaf_update, false) AS after_update,
    false AS before_import,
    coalesce(l.leaf_import, false) AS after_import,
    false AS before_export,
    coalesce(l.leaf_export, false) AS after_export
  FROM accounts a
  LEFT JOIN leaf l ON l.user_id = a.user_id
)
SELECT 'lost' AS daftar, x.* FROM access x
WHERE (x.before_view AND NOT x.after_view) OR (x.before_update AND NOT x.after_update)
UNION ALL
SELECT 'gained', x.* FROM access x
WHERE (x.after_view AND NOT x.before_view) OR (x.after_update AND NOT x.before_update)
   OR (x.after_import AND NOT x.before_import) OR (x.after_export AND NOT x.before_export)
UNION ALL
SELECT 'akun', x.* FROM access x
WHERE x.before_view OR x.after_view OR x.before_update OR x.after_update
   OR x.after_import OR x.after_export
ORDER BY daftar DESC, email;

ROLLBACK;
