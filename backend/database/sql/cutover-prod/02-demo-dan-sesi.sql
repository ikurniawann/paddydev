-- Cutover production: buang karyawan dummy demo@paddy.id (DEMOPADDY, tanpa akun
-- login) beserta data turunannya (cascade: onboarding, riwayat, saldo cuti,
-- penempatan, KPI), lalu kosongkan sesi login supaya semua user login ulang.
\set ON_ERROR_STOP on
BEGIN;
-- Sama dengan sesi aplikasi (backend/internal/platform/database.SearchPath): trigger
-- memakai nama tabel tanpa schema dan role DB production tidak menyetel search_path.
SET LOCAL search_path = public, iam, configuration, hris, performance, recruitment, item, purchasing, inventory, manufacturing, pos, crm, accounting, auth;
SET LOCAL timezone = 'Asia/Jakarta';

CREATE TEMP TABLE cutover_demo ON COMMIT DROP AS
SELECT id, user_id FROM hris.employees WHERE nip = 'DEMOPADDY' AND lower(email) = 'demo@paddy.id';

DO $$
BEGIN
  IF (SELECT count(*) FROM cutover_demo) <> 1 THEN
    RAISE EXCEPTION 'karyawan demo@paddy.id (DEMOPADDY) harus tepat 1, ditemukan %', (SELECT count(*) FROM cutover_demo);
  END IF;
END $$;

DELETE FROM hris.employees WHERE id IN (SELECT id FROM cutover_demo);
DELETE FROM iam.user_roles WHERE user_id IN (SELECT user_id FROM cutover_demo WHERE user_id IS NOT NULL);
DELETE FROM configuration.users WHERE id IN (SELECT user_id FROM cutover_demo WHERE user_id IS NOT NULL)
   OR lower(email) = 'demo@paddy.id';
DELETE FROM auth.users WHERE id IN (SELECT user_id FROM cutover_demo WHERE user_id IS NOT NULL)
   OR lower(email) = 'demo@paddy.id';

DELETE FROM auth.sessions;
DELETE FROM auth.login_attempts;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM hris.employees WHERE nip = 'DEMOPADDY' OR lower(email) = 'demo@paddy.id')
     OR EXISTS (SELECT 1 FROM auth.users WHERE lower(email) = 'demo@paddy.id') THEN
    RAISE EXCEPTION 'demo@paddy.id masih ada';
  END IF;
  RAISE NOTICE 'cutover 02 OK';
END $$;
COMMIT;
