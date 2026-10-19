import { apiGet, apiPost, apiPut, apiPatch, apiDelete, buildListUrl } from "@/lib/api-client";
import type { ShiftScheduleRowLike } from "@/lib/hris/employee-profile-shifts";
import type { ScheduleType } from "@/lib/hris/schedule-type";
import type { CreateUserEmployeeInput, UpdateUserEmployeeInput } from "@/lib/users/schemas";
import type { UserEmployeeItem } from "@/lib/users/user-mapper";
import type { Employee } from "@/types/hris";
import type {
  AttendanceRow,
  EmployeeDocumentInput,
  EmployeeDocumentRow,
  EmploymentHistoryRow,
  LeaveBalanceRow,
  UserDirectoryStats,
  UserFormLookups,
  UserListParams,
  UserListResponse,
} from "./types";

const BASE = "/api/users";

export const fetchUserList = (params?: UserListParams) =>
  apiGet<UserListResponse>(buildListUrl(BASE, params));

export const fetchUserDetail = (id: string) => apiGet<{ data: UserEmployeeItem }>(`${BASE}/${id}`);

export const fetchUserDirectoryStats = async (): Promise<UserDirectoryStats> => {
  const [all, active, withAccess] = await Promise.all([
    fetchUserList({ limit: 1, page: 1 }),
    fetchUserList({ limit: 1, page: 1, is_active: "true" }),
    fetchUserList({ limit: 1, page: 1, is_access_app: "true" }),
  ]);
  return {
    total: all.total,
    active: active.total,
    withAccess: withAccess.total,
  };
};

export const fetchHRISEmployeeDetail = (id: string) =>
  apiGet<{ data: Employee }>(`/api/hris/employees/${id}`);

/** Login As (super_admin only) — session berpindah ke user target. */
export const impersonateUser = (userId: string) =>
  apiPost<{
    success: boolean;
    data: { id: string; email: string; role: string; full_name: string };
    message: string;
  }>("/api/auth/impersonate", { user_id: userId });

export const fetchEmployeeDocuments = (employeeId: string) =>
  apiGet<{ data: EmployeeDocumentRow[] }>(
    `/api/hris/employees/documents?employee_id=${employeeId}`
  );

export interface EmployeeLifecycleData {
  employee: {
    join_date: string | null;
    end_date: string | null;
    employment_status: string;
    is_active: boolean;
    created_at: string;
    has_account: boolean;
  };
  recruitment: {
    candidate_id: string;
    applied_at: string;
    source: string | null;
    position_title: string | null;
    offer_accepted_at: string | null;
    promoted_at: string | null;
  } | null;
  account: {
    email: string;
    created_at: string;
    last_sign_in_at: string | null;
  } | null;
  onboarding: {
    total: number;
    completed: number;
    last_completed_at: string | null;
  };
  history: {
    id: string;
    change_type: string;
    effective_date: string;
    reason: string | null;
    notes: string | null;
    prev_employment_status: string | null;
    new_employment_status: string | null;
    prev_department_name: string | null;
    new_department_name: string | null;
    prev_job_title: string | null;
    new_job_title: string | null;
  }[];
  offboarding: {
    id: string;
    status: string;
    resignation_type: string | null;
    resignation_date: string | null;
    last_working_day: string | null;
    clearance_hrd: boolean | null;
    clearance_it: boolean | null;
    clearance_finance: boolean | null;
    clearance_manager: boolean | null;
    completed_at: string | null;
  } | null;
}

export const fetchEmployeeLifecycle = (employeeId: string) =>
  apiGet<{ data: EmployeeLifecycleData }>(`/api/hris/employees/${employeeId}/lifecycle`);

// ── Dokumen asal rekrutmen (CV + Laporan Pipeline) ──────────────────────
export interface EmployeeRecruitmentDocs {
  candidate_id: string;
  cv_url: string | null;
  status: string;
  applied_at: string;
  position_title: string | null;
  report_available: boolean;
}

export const fetchEmployeeRecruitmentDocs = (employeeId: string) =>
  apiGet<{ data: EmployeeRecruitmentDocs | null }>(
    `/api/hris/employees/${employeeId}/recruitment-documents`
  );

export const fetchEmploymentHistory = (employeeId: string) =>
  apiGet<{ data: EmploymentHistoryRow[] }>(
    `/api/hris/employment-history?employee_id=${employeeId}`
  );

export const fetchEmployeeAttendance = (employeeId: string, month: number, year: number) => {
  const params = new URLSearchParams({
    employee_id: employeeId,
    month: String(month),
    year: String(year),
  });
  return apiGet<{ data: AttendanceRow[] }>(`/api/hris/attendance?${params}`);
};

export const fetchEmployeeLeaveBalances = (employeeId: string) =>
  apiGet<{ data: LeaveBalanceRow[] }>(`/api/hris/leave-balances/${employeeId}`);

// ── Kontrak karyawan (PKWTT / PKWT) ─────────────────────────────────────
export interface EmployeeContractRow {
  id: string;
  employee_id: string;
  contract_number: string;
  contract_type: "pkwtt" | "pkwt";
  status: "draft" | "active" | "ended" | "terminated" | "converted";
  start_date: string;
  end_date: string | null;
  probation_end_date: string | null;
  parent_contract_id: string | null;
  sequence: number;
  position_title: string | null;
  department_name: string | null;
  work_location: string | null;
  base_salary: string | null;
  signed_at: string | null;
  signed_document_url: string | null;
  kemnaker_registered_at: string | null;
  compensation_amount: string | null;
  compensation_paid_at: string | null;
  terminated_reason: string | null;
  notes: string | null;
  created_by_name: string | null;
  created_at: string;
}

export interface CreateContractInput {
  contract_type: "pkwtt" | "pkwt";
  start_date: string;
  end_date?: string | null;
  probation_end_date?: string | null;
  position_title?: string | null;
  work_location?: string | null;
  base_salary?: number | null;
  notes?: string | null;
}

export interface ContractActionInput {
  action: "activate" | "end" | "terminate" | "convert" | "renew" | "edit" | "update";
  end_date?: string | null;
  reason?: string;
  signed_at?: string | null;
  signed_document_url?: string;
  kemnaker_registered_at?: string | null;
  compensation_paid_at?: string | null;
  notes?: string | null;
  // aksi edit (draft saja)
  start_date?: string;
  probation_end_date?: string | null;
  position_title?: string | null;
  work_location?: string | null;
  base_salary?: number | null;
}

export interface ExpiringContractsData {
  days: number;
  contracts: {
    contract_id: string;
    employee_id: string;
    employee_name: string;
    contract_number: string;
    contract_type: "pkwt" | "pkwtt";
    position_title: string | null;
    end_date: string;
    days_left: number;
  }[];
  probations: {
    contract_id: string;
    employee_id: string;
    employee_name: string;
    contract_number: string;
    position_title: string | null;
    probation_end_date: string;
    days_left: number;
  }[];
  noContract: {
    employee_id: string;
    employee_name: string;
    employment_status: string;
    join_date: string | null;
    draft_contract_number: string | null;
    draft_start_date: string | null;
  }[];
}

export const fetchExpiringContracts = (days = 30) =>
  apiGet<{ data: ExpiringContractsData }>(`/api/hris/contracts/expiring?days=${days}`);

export const fetchEmployeeContracts = (employeeId: string) =>
  apiGet<{ data: EmployeeContractRow[] }>(`/api/hris/employees/${employeeId}/contracts`);

export const createEmployeeContract = (employeeId: string, payload: CreateContractInput) =>
  apiPost<{ data: EmployeeContractRow; message: string }>(
    `/api/hris/employees/${employeeId}/contracts`,
    payload
  );

export const patchEmployeeContract = (contractId: string, payload: ContractActionInput) =>
  apiPatch<{ message: string; compensation_amount?: number | null }>(
    `/api/hris/contracts/${contractId}`,
    payload
  );

export const deleteEmployeeContract = (contractId: string) =>
  apiDelete(`/api/hris/contracts/${contractId}`);

export async function uploadContractSignedDocument(contractId: string, file: File) {
  const formData = new FormData();
  formData.append("file", file);
  const res = await fetch(`/api/hris/contracts/${contractId}/signed-document`, {
    method: "POST",
    body: formData,
  });
  const json = (await res.json().catch(() => ({}))) as {
    message?: string;
    error?: string;
  };
  if (!res.ok) {
    throw new Error(json.error ?? `Upload gagal (${res.status})`);
  }
  return json as { message: string };
}

export const deleteContractSignedDocument = (contractId: string) =>
  apiDelete(`/api/hris/contracts/${contractId}/signed-document`);

export const createEmployeeDocument = (payload: EmployeeDocumentInput) =>
  apiPost<{ data: unknown }>("/api/hris/employees/documents", payload);

export const deleteEmployeeDocument = (docId: string) =>
  apiDelete(`/api/hris/employees/documents/${docId}`);

export const fetchUserFormLookups = async (): Promise<UserFormLookups> => {
  const [deptsRes, posRes, statusRes, managersRes, sectionsRes] = await Promise.all([
    apiGet<{ data: UserFormLookups["departments"] }>("/api/master/departments"),
    apiGet<{ data: UserFormLookups["positions"] }>("/api/master/positions"),
    apiGet<{ data: UserFormLookups["employmentStatuses"] }>("/api/master/employment-statuses"),
    apiGet<{ data: UserFormLookups["managers"] }>(
      "/api/hris/employees?is_active=true&limit=200&sort_by=full_name&sort_order=asc"
    ),
    apiGet<{ data: UserFormLookups["sections"] }>("/api/sections"),
  ]);

  return {
    departments: deptsRes.data ?? [],
    positions: posRes.data ?? [],
    employmentStatuses: (statusRes.data ?? []).filter((s) => s.is_active !== false),
    managers: managersRes.data ?? [],
    sections: (sectionsRes.data ?? []).map((s) => ({ id: s.id, name: s.name })),
  };
};

export const createUser = (body: CreateUserEmployeeInput) =>
  apiPost<{ data: UserEmployeeItem; message: string }>(BASE, body);

export const updateUser = (id: string, body: UpdateUserEmployeeInput) =>
  apiPut<{ data: UserEmployeeItem; message: string }>(`${BASE}/${id}`, body);

export const resetUserPassword = (id: string) =>
  apiPost<{ message: string; tempPassword: string }>(`${BASE}/${id}/reset-password`, {});

// ── Jadwal shift mingguan karyawan ───────────────────────────────────────
export interface ShiftOption {
  id: string;
  name: string;
  start_time: string;
  end_time: string;
  is_active: boolean;
}

export interface EmployeeShiftScheduleRow extends ShiftScheduleRowLike {
  id: string;
  start_time: string | null;
  end_time: string | null;
}

export const fetchShiftOptions = () => apiGet<{ data: ShiftOption[] }>("/api/hris/shifts");

export const fetchEmployeeShiftSchedule = (employeeId: string) =>
  apiGet<{ data: EmployeeShiftScheduleRow[] }>(`/api/hris/employees/${employeeId}/shifts`);

export const saveEmployeeShiftPattern = (
  employeeId: string,
  payload: {
    effective_from: string;
    days: { day_of_week: number; shift_id: string | null }[];
  }
) => apiPut<{ message: string }>(`/api/hris/employees/${employeeId}/shifts`, payload);

/* ── Tipe jadwal Shift / Non-Shift (EPIC-056) ───────────────────────── */

export interface ScheduleTypeResult {
  changed: number;
  cleared_days: number;
  warnings: string[];
}

export const fetchScheduleType = (id: string) =>
  apiGet<{ data: { schedule_type: ScheduleType; future_days: number } }>(`/api/hris/employees/${id}/schedule-type`);

export const saveScheduleType = (id: string, body: { schedule_type: ScheduleType; effective_from: string }) =>
  apiPut<{ data: ScheduleTypeResult; message?: string }>(`/api/hris/employees/${id}/schedule-type`, body);

export const bulkScheduleType = (body: {
  employee_ids: string[];
  schedule_type: ScheduleType;
  effective_from: string;
  dry_run?: boolean;
}) => apiPost<{ data: ScheduleTypeResult; message?: string }>("/api/hris/employees/schedule-type/bulk", body);

export const applyFixedSchedule = (body: { branch_id?: string; employee_ids?: string[]; effective_from: string }) =>
  apiPost<{ data: ScheduleTypeResult; message?: string }>("/api/hris/schedules/fixed", body);

export const resignEmployee = (
  id: string,
  body: { effective_date: string; resign_type: string; reason: string | null }
) => apiPost<{ data: unknown; message?: string }>(`/api/hris/employees/${id}/resign`, body);
