/**
 * Role Management untuk route fallback Next (EPIC-061, R10): sama dengan Go
 * access.requireSuperAdmin — role sistem hanya diubah oleh Super Admin.
 */
import { ApiError, type ApiUser } from "@/lib/api/auth";
import { resolveRoleIds } from "./get-user-menus";
import { iamDbQueryOne } from "./pg-client";

export const SYSTEM_ROLE_SUPER_ADMIN_ERROR = "Role sistem hanya bisa diubah oleh Super Admin";
export const ADMIN_HRIS_EMPTY_ERROR = "admin_hris harus punya minimal satu menu: role HR lain diturunkan darinya";

export async function requireSuperAdminForSystemRole(user: ApiUser): Promise<void> {
  const roleIds = await resolveRoleIds(user.id, user.role);
  if (roleIds.length > 0) {
    const row = await iamDbQueryOne<{ ok: boolean }>(
      `SELECT EXISTS (SELECT 1 FROM iam.roles WHERE id = ANY($1::uuid[]) AND code = 'super_admin' AND deleted_at IS NULL) AS ok`,
      [roleIds]
    );
    if (row?.ok) return;
  }
  throw ApiError.forbidden(SYSTEM_ROLE_SUPER_ADMIN_ERROR);
}
