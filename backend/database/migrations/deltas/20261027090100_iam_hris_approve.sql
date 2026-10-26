-- =============================================================================
-- EPIC-067 (R10/R12): aksi "approve" pada daun HRIS untuk keputusan HR.
--
-- Keputusan HR (pengajuan absensi, Perjalanan Dinas, cuti, lembur) memeriksa
-- "approve" pada daunnya sendiri: hris.workforce.attendance (absensi dan
-- Perjalanan Dinas), hris.workforce.leaves, hris.kepegawaian.overtime.
--
-- 1) Menu Absensi dan Cuti & Izin menawarkan "approve". Lembur sudah
--    menawarkannya sejak 20260717170000.
-- 2) Baris grant yang SUDAH ADA milik super_admin, admin, hrd, dan admin_hris
--    pada ketiga daun mendapat "approve" (aktif maupun nonaktif, supaya grant
--    yang diaktifkan lagi tidak kehilangan keputusan). Baris baru tidak dibuat:
--    "approve" tanpa akses menu tidak berguna.
-- 3) Role HR turunan admin_hris disinkronkan ulang dengan penjaga yang sama
--    seperti 20261022090000; sync mengambil aksi dari permission_context.
--
-- Idempoten: setiap UPDATE hanya menyentuh baris yang belum memuat "approve".
-- Cek sebelum/sesudah: database/sql/checks/2026-10-26_epic067_approve_access.sql.
-- =============================================================================
UPDATE iam.menus m
SET permission_context = jsonb_set(m.permission_context, '{actions}',
      coalesce(m.permission_context->'actions', '["read"]'::jsonb) || '["approve"]'::jsonb),
    updated_at = now()
WHERE m.deleted_at IS NULL
  AND m.code IN ('hris.workforce.attendance', 'hris.workforce.leaves')
  AND NOT coalesce(m.permission_context->'actions', '[]'::jsonb) ? 'approve';

UPDATE iam.role_menu_permissions p
SET granted_actions = p.granted_actions || '["approve"]'::jsonb, updated_at = now()
FROM iam.roles r, iam.menus m
WHERE r.id = p.role_id AND m.id = p.menu_id
  AND r.code IN ('super_admin', 'admin', 'hrd', 'admin_hris')
  AND m.code IN ('hris.workforce.attendance', 'hris.workforce.leaves', 'hris.kepegawaian.overtime')
  AND jsonb_typeof(p.granted_actions) = 'array'
  AND NOT p.granted_actions ? 'approve';

-- admin_hris grants come from a seeder: on a fresh/CI database there is
-- nothing to derive from yet, and the sync refuses an empty source.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM iam.role_menu_permissions rmp
    JOIN iam.roles r ON r.id = rmp.role_id AND r.code = 'admin_hris'
    JOIN iam.menus m ON m.id = rmp.menu_id AND m.deleted_at IS NULL
    WHERE rmp.is_active) THEN
    PERFORM iam.sync_hris_staff_roles();
  END IF;
END
$$;
