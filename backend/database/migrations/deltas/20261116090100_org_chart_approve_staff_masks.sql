-- =============================================================================
-- EPIC-102 R8a (keputusan user 11-11-2026): menyetujui versi bagan resmi hanya
-- untuk Admin HR (super_admin, admin, hrd, admin_hris) dan HR Manager
-- (hris_manager). hris_non_payroll dan hris_non_rekrutmen_kpi tetap bisa
-- membaca Struktur Org, tanpa "approve".
--
-- Lewat masker Hak Akses HRIS supaya bertahan setiap sync. Baris masker baru
-- pada daun hris.organization.org-chart mewarisi masker yang sedang berlaku
-- untuk daun itu (prefix terpanjang: menu tersembunyi tetap tersembunyi, aksi
-- yang sudah dikurangi tetap dikurangi), lalu menambah "approve" — sama
-- dengan cara halaman Hak Akses HRIS menulis masker.
--
-- Idempoten. HR bisa memberi "approve" lagi per role lewat Hak Akses HRIS.
-- Rollback: hapus baris masker daun ini untuk kedua role, lalu
-- SELECT iam.sync_hris_staff_roles().
-- =============================================================================
INSERT INTO iam.hris_staff_role_masks (role_code, menu_prefix, exclude_menu, excluded_actions)
SELECT r.code, 'hris.organization.org-chart', coalesce(w.exclude_menu, false),
  (SELECT array_agg(DISTINCT a ORDER BY a) FROM unnest(coalesce(w.excluded_actions, '{}'::text[]) || '{approve}'::text[]) a)
FROM iam.roles r
LEFT JOIN LATERAL (
  SELECT k.exclude_menu, k.excluded_actions
  FROM iam.hris_staff_role_masks k
  WHERE k.role_code = r.code
    AND ('hris.organization.org-chart' = k.menu_prefix OR 'hris.organization.org-chart' LIKE k.menu_prefix || '.%')
  ORDER BY length(k.menu_prefix) DESC
  LIMIT 1) w ON true
WHERE r.code IN ('hris_non_payroll', 'hris_non_rekrutmen_kpi') AND r.deleted_at IS NULL
ON CONFLICT (role_code, menu_prefix) DO UPDATE SET
  excluded_actions = (SELECT array_agg(DISTINCT a ORDER BY a)
                      FROM unnest(iam.hris_staff_role_masks.excluded_actions || '{approve}'::text[]) a),
  updated_at = now()
WHERE NOT ('approve' = ANY (iam.hris_staff_role_masks.excluded_actions));

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
