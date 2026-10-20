import {
  ADMIN_USER_ROLES,
  APPROVAL_LEVELS,
  APPROVAL_MODULES,
  APPROVAL_WORKFLOWS,
} from "@/lib/admin/user-management";
import { formatRupiah } from "@/lib/format";
import { crudDetailPath, crudEditPath, crudInsertPath } from "@/lib/routes/crud-routes";
import type { UserRole } from "@/types";
import type { ApprovalPermission, UserEmployeeFormValues } from "@/lib/hris/users-form";

const EMPLOYEES_BASE_PATH = "/dashboard/employees";

export const EMPLOYEES_ROUTES = {
  list: EMPLOYEES_BASE_PATH,
  insert: crudInsertPath(EMPLOYEES_BASE_PATH),
  detail: (id: string) => crudDetailPath(EMPLOYEES_BASE_PATH, id),
  edit: (id: string) => crudEditPath(EMPLOYEES_BASE_PATH, id),
} as const;

export const ROLE_LABELS: Record<UserRole, string> = {
  super_admin: "Super Admin",
  admin: "Admin",
  hrd: "HRD",
  hiring_manager: "Hiring Manager",
  direksi: "Executive",
  purchasing_admin: "Purchasing Admin",
  purchasing_manager: "Purchasing Manager",
  purchasing_staff: "Purchasing Staff",
  finance_staff: "Finance Staff",
  warehouse_staff: "Warehouse Staff",
  warehouse_admin: "Warehouse Admin",
  pos: "POS",
  pos_supervisor: "POS Supervisor",
  qc_staff: "QC Staff",
  employee: "Karyawan (ESS)",
  sales: "Sales",
  marketing: "Marketing",
};

export const ROLE_OPTIONS = ADMIN_USER_ROLES.map((role) => ({
  value: role,
  label: ROLE_LABELS[role],
}));

/**
 * Role sistem: hanya Super Admin yang bisa memberi/mencabutnya atau mengubah akun yang
 * memegangnya (temuan lanjutan EPIC-061). UI menyembunyikannya; server tetap penentu.
 */
export const SYSTEM_ROLES: readonly string[] = ["super_admin", "admin"];

export const isSystemRole = (role: string | null | undefined) => !!role && SYSTEM_ROLES.includes(role);

export const SYSTEM_ACCOUNT_LOCKED_NOTICE =
  "Akun ini memegang role Super Admin/Admin; hanya Super Admin yang bisa mengubahnya.";

/**
 * Karyawan yang akun loginnya ber-role sistem tidak bisa disimpan selain oleh Super Admin
 * (server menolak 403 karena simpan form selalu menyentuh akun login). Role IAM-saja tidak
 * terlihat di sini; untuk itu server tetap penentu.
 */
export const isLockedSystemAccount = (accountRole: string | null | undefined, canAssignSystemRoles: boolean) =>
  !canAssignSystemRoles && isSystemRole(accountRole);

/** Opsi role untuk form: tanpa role sistem bagi selain Super Admin, kecuali `current` (agar labelnya tetap tampil). */
export function roleOptionsFor(canAssignSystemRoles: boolean, current?: string) {
  if (canAssignSystemRoles) return ROLE_OPTIONS;
  return ROLE_OPTIONS.filter((o) => !isSystemRole(o.value) || o.value === current);
}

/**
 * Bagi selain Super Admin, role sistem bawaan form akun baru (emptyUserForm: "admin") dikosongkan
 * agar role dipilih dari opsi yang boleh. Pakai hanya untuk form tanpa akun login.
 */
export function withAssignableRole<T extends Pick<UserEmployeeFormValues, "role">>(form: T, canAssignSystemRoles: boolean): T {
  if (canAssignSystemRoles || !isSystemRole(form.role)) return form;
  return { ...form, role: "" as UserRole };
}

export const STATUS_LABELS: Record<string, string> = {
  probation: "Probation",
  contract: "Contract",
  permanent: "Permanent",
  internship: "Internship",
  resigned: "Resigned",
  terminated: "Terminated",
  suspended: "Suspended",
};

export const STATUS_COLORS: Record<string, string> = {
  probation: "bg-yellow-100 text-yellow-700",
  contract: "bg-blue-100 text-blue-700",
  permanent: "bg-green-100 text-green-700",
  internship: "bg-purple-100 text-purple-700",
  resigned: "bg-red-100 text-red-600",
  terminated: "bg-red-200 text-red-700",
  suspended: "bg-orange-100 text-orange-700",
};

export const GENDER_OPTIONS = [
  { value: "male", label: "Male" },
  { value: "female", label: "Female" },
];

export const MARITAL_OPTIONS = [
  { value: "single", label: "Single" },
  { value: "married", label: "Married" },
  { value: "divorced", label: "Divorced" },
  { value: "widowed", label: "Widowed" },
];

export const emptyApprovalPermission: ApprovalPermission = {
  module: "purchasing",
  workflow: "purchase_request",
  approval_level: "approver",
  approval_limit: null,
  is_active: true,
};

export { emptyUserForm } from "@/lib/hris/users-form";

export function workflowsForModule(module: string) {
  return APPROVAL_WORKFLOWS.filter((workflow) => workflow.module === module);
}

export function workflowLabel(value: string) {
  return APPROVAL_WORKFLOWS.find((item) => item.value === value)?.label ?? value;
}

export function moduleLabel(value: string) {
  return APPROVAL_MODULES.find((item) => item.value === value)?.label ?? value;
}

export function levelLabel(value: string) {
  return APPROVAL_LEVELS.find((item) => item.value === value)?.label ?? value;
}

export function formatCurrency(value: number | null) {
  return value == null ? "Tanpa limit" : formatRupiah(value);
}

/** Role IAM HRIS untuk akun peran hrd (EPIC-061). "auto" = ikuti peran aplikasi. */
export const HRIS_ROLE_OPTIONS = [
  { value: "auto", label: "Ikuti peran aplikasi (hrd)" },
  { value: "admin_hris", label: "Admin HRIS" },
  { value: "hris_manager", label: "HR Manager" },
  { value: "hris_non_payroll", label: "HR Staff (Non-Payroll)" },
  { value: "hris_non_rekrutmen_kpi", label: "HR Staff (Non-Rekrutmen & KPI)" },
];
