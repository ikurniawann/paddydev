/**
 * Akun ber-role sistem (super_admin/admin) di Manajemen User untuk route
 * fallback Next: sama dengan Go accounts/system_roles.go. Memberi atau mencabut
 * role sistem, dan mengubah/menonaktifkan/reset password akun yang memegangnya,
 * hanya oleh pemegang role IAM super_admin (EPIC-061 Q-RA-5). Dicek sebelum
 * apa pun ditulis; fallback Next tidak menulis audit (ruling EPIC-061).
 */
import type { ApiUser } from "@/lib/api/auth";
import { requireIamSuperAdmin } from "@/lib/iam/system-role-guard";
import { iamDbQuery, iamDbQueryOne } from "@/lib/iam/pg-client";

export const SYSTEM_ACCOUNT_SUPER_ADMIN_ERROR = "Role Super Admin/Admin hanya bisa diatur oleh Super Admin";

/** Tetap role sistem walau iam.roles belum di-seed (role aplikasinya tetap berkuasa). */
export const BASE_SYSTEM_ROLES = ["super_admin", "admin"];

/** Kunci body PUT /api/users yang sampai ke akun login (termasuk email & nama yang disinkronkan). */
const LOGIN_KEYS = [
  "is_access_app",
  "password",
  "role",
  "brand_id",
  "account_status",
  "approval_permissions",
  "business_scope",
  "holding_id",
  "company_id",
  "branch_id",
  "warehouse_ids",
  "default_warehouse_id",
  "can_switch_stall",
  "can_central_checkout",
  "email",
  "full_name",
];

export const touchesLogin = (body: Record<string, unknown>) =>
  LOGIN_KEYS.some((key) => body[key] !== undefined);

/** iam.roles.is_system ditambah BASE_SYSTEM_ROLES. */
export async function systemRoleCodes(): Promise<string[]> {
  const rows = await iamDbQuery<{ code: string }>(
    `SELECT code FROM iam.roles WHERE is_system AND deleted_at IS NULL`
  );
  return [...new Set([...rows.map((r) => r.code), ...BASE_SYSTEM_ROLES])];
}

/** True bila akun memegang role sistem sebagai role aplikasi atau role IAM. */
export async function accountHoldsSystemRole(userId: string, codes: string[]): Promise<boolean> {
  const row = await iamDbQueryOne<{ held: boolean }>(
    `SELECT EXISTS (SELECT 1 FROM configuration.users WHERE id::text = $1 AND role = ANY($2))
      OR EXISTS (SELECT 1 FROM iam.user_roles ur JOIN iam.roles r ON r.id = ur.role_id
        WHERE ur.user_id::text = $1 AND r.code = ANY($2)) AS held`,
    [userId, codes]
  );
  return Boolean(row?.held);
}

/** user_id akun login milik karyawan (null bila tidak ada). */
export async function employeeUserId(employeeId: string): Promise<string | null> {
  const row = await iamDbQueryOne<{ user_id: string | null }>(
    `SELECT user_id::text AS user_id FROM hris.employees WHERE id::text = $1`,
    [employeeId]
  );
  return row?.user_id ?? null;
}

/**
 * 403 SYSTEM_ACCOUNT_SUPER_ADMIN_ERROR kecuali pemanggil super_admin, bila `role`
 * adalah role sistem atau akun `userId` memegangnya.
 */
export async function requireSuperAdminForSystemAccount(
  user: ApiUser,
  target: { role?: string | null; userId?: string | null }
): Promise<void> {
  const codes = await systemRoleCodes();
  const grants = !!target.role && codes.includes(target.role);
  const holds = !grants && !!target.userId && (await accountHoldsSystemRole(target.userId, codes));
  if (grants || holds) await requireIamSuperAdmin(user, SYSTEM_ACCOUNT_SUPER_ADMIN_ERROR);
}
