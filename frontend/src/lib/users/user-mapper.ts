import type { UserRole } from "@/types";
import type { BusinessScopeLevel } from "@/lib/configuration/business-scope";

export type EmployeeUserRow = {
  id: string;
  user_id: string | null;
  full_name: string;
  nip: string;
  email: string;
  phone: string;
  join_date: string;
  end_date: string | null;
  employment_status: string;
  is_active: boolean;
  is_access_app: boolean;
  schedule_type?: "shift" | "non_shift" | null;
  department_id: string | null;
  section_id: string | null;
  job_title_id: string | null;
  reporting_to: string | null;
  ktp: string | null;
  npwp: string | null;
  birth_date: string | null;
  gender: string | null;
  marital_status: string | null;
  address: string | null;
  city: string | null;
  province: string | null;
  postal_code: string | null;
  bank_name: string | null;
  bank_account: string | null;
  bpjs_tk: string | null;
  bpjs_kesehatan: string | null;
  emergency_contact_name: string | null;
  emergency_contact_phone: string | null;
  emergency_contact_relationship: string | null;
  photo_url: string | null;
  notes: string | null;
  created_at: string;
  updated_at: string | null;
  department?: { id: string; name: string; code?: string } | null;
  section?: { id: string; name: string; code?: string } | null;
  job_title?: { id: string; title: string; department?: string } | null;
  manager?: { id: string; full_name: string; nip: string } | null;
  app_user?: {
    id: string;
    role: UserRole;
    status: "active" | "inactive";
    brand_id: string | null;
    business_scope: BusinessScopeLevel | null;
    holding_id: string | null;
    company_id: string | null;
    branch_id: string | null;
    holding?: { id: string; name: string } | null;
    company?: { id: string; name: string } | null;
    branch?: { id: string; name: string } | null;
    brands?: { id: string; name: string } | null;
    last_sign_in_at?: string | null;
    can_switch_stall?: boolean | null;
    can_central_checkout?: boolean | null;
    default_warehouse_id?: string | null;
    user_approval_permissions?: Array<{
      id: string;
      module: string;
      workflow: string;
      approval_level: string;
      approval_limit: number | null;
      is_active: boolean;
    }>;
    user_warehouses?: Array<{
      id: string;
      warehouse_id: string;
      name: string;
      code: string;
      branch_id: string;
    }>;
  } | null;
};

export function mapEmployeeUserRow(row: EmployeeUserRow) {
  return {
    id: row.id,
    userId: row.user_id,
    fullName: row.full_name,
    nip: row.nip,
    email: row.email,
    phone: row.phone,
    joinDate: row.join_date,
    endDate: row.end_date,
    employmentStatus: row.employment_status,
    isActive: row.is_active,
    isAccessApp: row.is_access_app,
    scheduleType: (row.schedule_type ?? "shift") as "shift" | "non_shift",
    departmentId: row.department_id,
    sectionId: row.section_id,
    jobTitleId: row.job_title_id,
    reportingTo: row.reporting_to,
    ktp: row.ktp,
    npwp: row.npwp,
    birthDate: row.birth_date,
    gender: row.gender,
    maritalStatus: row.marital_status,
    address: row.address,
    city: row.city,
    province: row.province,
    postalCode: row.postal_code,
    bankName: row.bank_name,
    bankAccount: row.bank_account,
    bpjsTk: row.bpjs_tk,
    bpjsKesehatan: row.bpjs_kesehatan,
    emergencyContactName: row.emergency_contact_name,
    emergencyContactPhone: row.emergency_contact_phone,
    emergencyContactRelationship: row.emergency_contact_relationship,
    photoUrl: row.photo_url,
    notes: row.notes,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
    department: row.department ?? null,
    section: row.section ?? null,
    jobTitle: row.job_title ?? null,
    manager: row.manager ?? null,
    appAccount: row.app_user
      ? {
          id: row.app_user.id,
          role: row.app_user.role,
          status: row.app_user.status,
          brandId: row.app_user.brand_id,
          brandName: row.app_user.brands?.name ?? null,
          businessScope: row.app_user.business_scope,
          holdingId: row.app_user.holding_id,
          companyId: row.app_user.company_id,
          branchId: row.app_user.branch_id,
          holdingName: row.app_user.holding?.name ?? null,
          companyName: row.app_user.company?.name ?? null,
          branchName: row.app_user.branch?.name ?? null,
          lastSignInAt: row.app_user.last_sign_in_at ?? null,
          canSwitchStall: row.app_user.can_switch_stall === true,
          canCentralCheckout: row.app_user.can_central_checkout === true,
          defaultWarehouseId: row.app_user.default_warehouse_id ?? null,
          approvalPermissions: row.app_user.user_approval_permissions ?? [],
          warehouses: (row.app_user.user_warehouses ?? []).map((warehouse) => ({
            id: warehouse.warehouse_id,
            name: warehouse.name,
            code: warehouse.code,
            branchId: warehouse.branch_id,
          })),
        }
      : null,
  };
}

export type UserEmployeeItem = ReturnType<typeof mapEmployeeUserRow>;
