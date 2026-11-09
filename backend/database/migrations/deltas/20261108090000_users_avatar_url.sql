-- =============================================================================
-- Foto profil akun (EPIC-087 R1): URL /api/files/profile-photos/<user_id>/…
-- (bucket staff). NULL = avatar inisial. Untuk akun yang terhubung ke
-- karyawan, nilainya juga disalin ke hris.employees.photo_url bila kolom itu
-- kosong atau masih berisi foto profil sebelumnya (foto HR tidak ditimpa).
-- =============================================================================

ALTER TABLE configuration.users ADD COLUMN IF NOT EXISTS avatar_url text;

COMMENT ON COLUMN configuration.users.avatar_url IS
  'EPIC-087: foto profil akun (URL /api/files/profile-photos/...), NULL = inisial';
