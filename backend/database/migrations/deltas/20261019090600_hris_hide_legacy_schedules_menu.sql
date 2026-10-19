-- =============================================================================
-- Menu lama "Schedules" (Kehadiran & Cuti) menuju route yang sama dengan menu
-- baru Kepegawaian → Jadwal Kerja (EPIC-056). Disembunyikan atas keputusan
-- user 19-10-2026; hak akses peran tidak diubah.
-- =============================================================================
UPDATE iam.menus SET is_visible = false, updated_at = now()
WHERE code = 'hris.workforce.schedules' AND route_path = '/dashboard/hris/schedules';
