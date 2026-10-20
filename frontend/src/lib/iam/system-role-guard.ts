/**
 * Role Management untuk route fallback Next (EPIC-061, R10): sama dengan Go
 * access.requireSuperAdmin — role sistem hanya diubah oleh Super Admin.
 */
import { ApiError, type ApiUser } from "@/lib/api/auth";
import { resolveRoleIds } from "./get-user-menus";
import { iamDbQueryOne } from "./pg-client";

export const SYSTEM_ROLE_SUPER_ADMIN_ERROR = "Role sistem hanya bisa diubah oleh Super Admin";
export const ADMIN_HRIS_EMPTY_ERROR = "admin_hris harus punya minimal satu menu: role HR lain diturunkan darinya";

// Role HR (admin_hris + role turunan hris_*) dan hak hapus HRIS di Role Management hanya lewat Super Admin;
// AdminHRIS memakai halaman Hak Akses HRIS (EPIC-061, Q-RA-2/Q-RA-5). Sama dengan Go iam.HrisStaffRoleCodes.
export const HRIS_STAFF_ROLE_CODES = ["admin_hris", "hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"];
export const HRIS_ROLE_SUPER_ADMIN_ERROR = "Role HR hanya bisa diubah oleh Super Admin; AdminHRIS memakai halaman Hak Akses HRIS";
export const HRIS_DELETE_SUPER_ADMIN_ERROR = "Hak hapus HRIS hanya bisa diberikan oleh Super Admin";

export const isHrisStaffRole = (code: string | null | undefined) => !!code && HRIS_STAFF_ROLE_CODES.includes(code);

/** Melempar 403 `message` kecuali pengguna memegang role IAM super_admin. */
export async function requireIamSuperAdmin(user: ApiUser, message: string): Promise<void> {
  const roleIds = await resolveRoleIds(user.id, user.role);
  if (roleIds.length > 0) {
    const row = await iamDbQueryOne<{ ok: boolean }>(
      `SELECT EXISTS (SELECT 1 FROM iam.roles WHERE id = ANY($1::uuid[]) AND code = 'super_admin' AND deleted_at IS NULL) AS ok`,
      [roleIds]
    );
    if (row?.ok) return;
  }
  throw ApiError.forbidden(message);
}

export async function requireSuperAdminForSystemRole(user: ApiUser): Promise<void> {
  await requireIamSuperAdmin(user, SYSTEM_ROLE_SUPER_ADMIN_ERROR);
}

/** True bila grant menambah "delete" pada menu HRIS yang belum dipegang role dengan delete (Go addsHrisDelete). */
export async function addsHrisDelete(
  roleId: string,
  permissions: { menuId: string; grantedActions: string[] }[]
): Promise<boolean> {
  const ids = permissions.filter((p) => p.grantedActions.includes("delete")).map((p) => p.menuId);
  if (ids.length === 0) return false;
  const row = await iamDbQueryOne<{ added: boolean }>(
    `SELECT EXISTS (SELECT 1 FROM iam.menus m
      WHERE m.id::text = ANY($2::text[]) AND m.code LIKE 'hris.%'
        AND NOT EXISTS (SELECT 1 FROM iam.role_menu_permissions p
          WHERE p.role_id = $1 AND p.menu_id = m.id AND p.is_active AND p.granted_actions ? 'delete')) AS added`,
    [roleId, ids]
  );
  return Boolean(row?.added);
}
