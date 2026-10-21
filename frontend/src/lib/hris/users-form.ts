/**
 * Form karyawan + akses aplikasi (halaman Add/Edit Employee): nilai form dari
 * data API, validasi sebelum kirim, dan payload untuk POST/PUT /api/users.
 */
import { APPROVAL_WORKFLOWS } from "@/lib/admin/user-management";
import { normalizeBusinessScopePayload } from "@/lib/configuration/business-scope";
import { isSelfManager } from "@/lib/hris/reporting-to";
import type { ScheduleType } from "@/lib/hris/schedule-type";
import type { UserEmployeeItem } from "@/lib/users/user-mapper";
import type { UserRole } from "@/types";

export type AccountStatus = "active" | "inactive";

export interface ApprovalPermission {
  id?: string;
  module: string;
  workflow: string;
  approval_level: "checker" | "approver" | "final_approver";
  approval_limit: number | null;
  is_active: boolean;
}

export interface UserEmployeeFormValues {
  /** Tipe jadwal (EPIC-056); dikirim hanya saat tambah karyawan. */
  schedule_type: ScheduleType;
  full_name: string;
  email: string;
  phone: string;
  join_date: string;
  employment_status: string;
  ktp: string;
  npwp: string;
  birth_date: string;
  gender: string;
  marital_status: string;
  address: string;
  city: string;
  province: string;
  postal_code: string;
  department_id: string;
  section_id: string;
  job_title_id: string;
  reporting_to: string;
  bank_name: string;
  bank_account: string;
  bpjs_tk: string;
  bpjs_kesehatan: string;
  emergency_contact_name: string;
  emergency_contact_phone: string;
  emergency_contact_relationship: string;
  notes: string;
  nip: string;
  is_active: boolean;
  end_date: string;
  is_access_app: boolean;
  password: string;
  role: UserRole;
  /** Role IAM HRIS (EPIC-061); "" = ikuti peran aplikasi. */
  hris_role: string;
  /** Nilai hris_role saat dimuat: dikirim hanya bila berubah. */
  hris_role_loaded: string;
  business_scope: "" | "holding" | "company" | "branch";
  holding_id: string;
  company_id: string;
  branch_id: string;
  warehouse_ids: string[];
  default_warehouse_id: string;
  can_switch_stall: boolean;
  can_central_checkout: boolean;
  account_status: AccountStatus;
  approval_permissions: ApprovalPermission[];
}

export const emptyUserForm: UserEmployeeFormValues = {
  schedule_type: "shift",
  full_name: "",
  email: "",
  phone: "",
  join_date: "",
  employment_status: "",
  ktp: "",
  npwp: "",
  birth_date: "",
  gender: "",
  marital_status: "",
  address: "",
  city: "",
  province: "",
  postal_code: "",
  department_id: "",
  section_id: "",
  job_title_id: "",
  reporting_to: "",
  bank_name: "",
  bank_account: "",
  bpjs_tk: "",
  bpjs_kesehatan: "",
  emergency_contact_name: "",
  emergency_contact_phone: "",
  emergency_contact_relationship: "",
  notes: "",
  nip: "",
  is_active: true,
  end_date: "",
  is_access_app: false,
  password: "",
  role: "admin",
  hris_role: "",
  hris_role_loaded: "",
  business_scope: "",
  holding_id: "",
  company_id: "",
  branch_id: "",
  warehouse_ids: [],
  default_warehouse_id: "",
  can_switch_stall: false,
  can_central_checkout: false,
  account_status: "active",
  approval_permissions: [],
};

const dateOnly = (value: string | null | undefined) => value?.slice(0, 10) ?? "";

export function userFormFromItem(detail: UserEmployeeItem): UserEmployeeFormValues {
  const account = detail.appAccount;
  return {
    schedule_type: detail.scheduleType ?? "shift",
    full_name: detail.fullName,
    email: detail.email,
    phone: detail.phone ?? "",
    join_date: dateOnly(detail.joinDate),
    employment_status: detail.employmentStatus,
    ktp: detail.ktp ?? "",
    npwp: detail.npwp ?? "",
    birth_date: dateOnly(detail.birthDate),
    gender: detail.gender ?? "",
    marital_status: detail.maritalStatus ?? "",
    address: detail.address ?? "",
    city: detail.city ?? "",
    province: detail.province ?? "",
    postal_code: detail.postalCode ?? "",
    department_id: detail.departmentId ?? "",
    section_id: detail.sectionId ?? "",
    job_title_id: detail.jobTitleId ?? "",
    // Atasan = diri sendiri (data lama) tidak dimunculkan dan tidak dikirim ulang: server menolaknya
    // (EPIC-063), dan pilihan Atasan memang tidak lagi memuat karyawan itu sendiri.
    reporting_to: isSelfManager(detail.id, detail.reportingTo) ? "" : (detail.reportingTo ?? ""),
    bank_name: detail.bankName ?? "",
    bank_account: detail.bankAccount ?? "",
    bpjs_tk: detail.bpjsTk ?? "",
    bpjs_kesehatan: detail.bpjsKesehatan ?? "",
    emergency_contact_name: detail.emergencyContactName ?? "",
    emergency_contact_phone: detail.emergencyContactPhone ?? "",
    emergency_contact_relationship: detail.emergencyContactRelationship ?? "",
    notes: detail.notes ?? "",
    nip: detail.nip ?? "",
    is_active: detail.isActive,
    end_date: dateOnly(detail.endDate),
    is_access_app: detail.isAccessApp,
    password: "",
    role: account?.role ?? "admin",
    hris_role: account?.hrisRole ?? "",
    hris_role_loaded: account?.hrisRole ?? "",
    business_scope: account?.businessScope ?? "",
    holding_id: account?.holdingId ?? "",
    company_id: account?.companyId ?? "",
    branch_id: account?.branchId ?? "",
    warehouse_ids: account?.warehouses?.map((warehouse) => warehouse.id) ?? [],
    default_warehouse_id: account?.defaultWarehouseId ?? account?.warehouses?.[0]?.id ?? "",
    can_switch_stall: account?.canSwitchStall === true,
    can_central_checkout: account?.canCentralCheckout === true,
    account_status: account?.status ?? "active",
    approval_permissions:
      account?.approvalPermissions
        ?.filter((p) => p.is_active)
        .map((p) => ({
          id: p.id,
          module: p.module,
          workflow: p.workflow,
          approval_level: p.approval_level as ApprovalPermission["approval_level"],
          approval_limit: p.approval_limit,
          is_active: true,
        })) ?? [],
  };
}

/** Pesan galat pertama yang menghalangi simpan; null bila form siap dikirim. */
export function validateUserForm(
  form: UserEmployeeFormValues,
  opts: { isEdit: boolean; hasExistingAppAccount: boolean }
): string | null {
  if (!form.full_name || !form.email || !form.join_date || !form.employment_status) {
    return "Full name, email, join date, and employment status are required";
  }
  if (!opts.isEdit && !form.schedule_type) return "Tipe jadwal wajib dipilih";

  if (form.is_access_app) {
    const needsNewPassword = !opts.isEdit || !opts.hasExistingAppAccount;
    if (needsNewPassword && form.password.length < 8) {
      return "Password must be at least 8 characters for app access";
    }
    if (!form.role) return "Role is required for app access";

    if (form.role !== "super_admin") {
      const scope = form.business_scope;
      if (!scope) return "Data access scope is required";
      if (scope === "holding" && !form.holding_id) return "Holding is required";
      if (scope === "company" && (!form.holding_id || !form.company_id)) {
        return "Holding and company are required";
      }
      if (scope === "branch" && (!form.holding_id || !form.company_id || !form.branch_id)) {
        return "Holding, company, and branch are required";
      }
      if (scope === "branch" && !form.default_warehouse_id) {
        return "Pilih outlet default untuk scope branch";
      }
    }
  }

  const invalidWorkflow = form.approval_permissions.some(
    (permission) =>
      !APPROVAL_WORKFLOWS.some(
        (w) => w.module === permission.module && w.value === permission.workflow
      )
  );
  if (invalidWorkflow) return "Approval workflow must match the selected module";

  return null;
}

/** Field akses aplikasi: hanya dikirim saat is_access_app aktif. */
const APP_ACCESS_FIELDS = new Set<keyof UserEmployeeFormValues>([
  "is_access_app",
  "password",
  "role",
  "hris_role",
  "hris_role_loaded",
  "business_scope",
  "holding_id",
  "company_id",
  "branch_id",
  "warehouse_ids",
  "default_warehouse_id",
  "can_switch_stall",
  "can_central_checkout",
  "account_status",
  "approval_permissions",
]);

/**
 * Payload POST/PUT /api/users: string kosong → null; data akun hanya bila akses aktif.
 * `canSetHrisRole` (default true) = pemanggil super_admin; false menghilangkan key hris_role.
 */
export function buildUserPayload(
  form: UserEmployeeFormValues,
  isEdit: boolean,
  opts: { canSetHrisRole?: boolean } = {}
): Record<string, unknown> {
  const payload: Record<string, unknown> = {};

  for (const [key, value] of Object.entries(form)) {
    if (APP_ACCESS_FIELDS.has(key as keyof UserEmployeeFormValues)) continue;
    if (typeof value === "boolean") payload[key] = value;
    else if (typeof value === "string") payload[key] = value === "" ? null : value;
  }

  // Tipe jadwal hanya saat tambah; perubahan lewat dialog "Ubah tipe jadwal".
  if (isEdit) delete payload.schedule_type;

  payload.is_access_app = form.is_access_app;
  if (!form.is_access_app) return payload;

  Object.assign(payload, {
    role: form.role,
    account_status: form.account_status,
    approval_permissions: form.approval_permissions,
    warehouse_ids: form.default_warehouse_id ? [form.default_warehouse_id] : form.warehouse_ids,
    default_warehouse_id: form.default_warehouse_id || null,
    can_switch_stall: form.can_switch_stall,
    can_central_checkout: form.can_central_checkout,
    ...normalizeBusinessScopePayload(
      form.role === "super_admin" ? null : form.business_scope || null,
      form.holding_id || null,
      form.company_id || null,
      form.branch_id || null
    ),
  });

  // Role HRIS hanya untuk peran hrd dan hanya bila diubah: simpan ulang tidak menimpa role IAM.
  // Server menolak (403) body yang memuat key hris_role dari non-super_admin, jadi key
  // dihilangkan sama sekali kecuali pemanggil boleh mengaturnya.
  if (opts.canSetHrisRole !== false && form.role === "hrd" && form.hris_role !== form.hris_role_loaded) {
    payload.hris_role = form.hris_role || null;
  }

  // edit: password kosong = tidak diganti
  if (form.password || !isEdit) payload.password = form.password;
  return payload;
}
