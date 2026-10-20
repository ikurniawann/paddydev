-- =============================================================================
-- EPIC-061 (Q-RA-8): riwayat perubahan hak akses. Satu baris per role × menu
-- yang aksinya berubah (actions_before/after, NULL = tidak ada grant), per
-- perubahan masker role HRIS (source hris_mask: {exclude_menu,
-- excluded_actions}), dan per penugasan role ke user (roles_before/after).
-- Ditulis aplikasi (paket platform/permaudit); migrasi dan skrip ops tidak.
-- =============================================================================
CREATE TABLE IF NOT EXISTS iam.permission_audit_logs (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id       uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  source         varchar(40) NOT NULL
                 CHECK (source IN ('role_permissions', 'hris_access', 'hris_mask', 'user_role')),
  role_id        uuid REFERENCES iam.roles(id) ON DELETE SET NULL,
  role_code      varchar(50),
  menu_code      varchar(100),
  actions_before jsonb,
  actions_after  jsonb,
  target_user_id uuid REFERENCES auth.users(id) ON DELETE SET NULL,
  roles_before   text[],
  roles_after    text[],
  created_at     timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX IF NOT EXISTS permission_audit_logs_created_idx ON iam.permission_audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS permission_audit_logs_role_idx ON iam.permission_audit_logs (role_code, created_at DESC);
CREATE INDEX IF NOT EXISTS permission_audit_logs_target_user_idx ON iam.permission_audit_logs (target_user_id, created_at DESC) WHERE target_user_id IS NOT NULL;
