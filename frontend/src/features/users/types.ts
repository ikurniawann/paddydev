export type { UserEmployeeItem } from "@/lib/users/user-mapper";
export type { CreateUserEmployeeInput, UpdateUserEmployeeInput } from "@/lib/users/schemas";

import type { UserEmployeeItem } from "@/lib/users/user-mapper";

export type {
  AccountStatus,
  ApprovalPermission,
  UserEmployeeFormValues,
} from "@/lib/hris/users-form";

export type UserListParams = {
  search?: string;
  department_id?: string;
  employment_status?: string;
  is_active?: string;
  is_access_app?: string;
  schedule_type?: "shift" | "non_shift";
  role?: string;
  page?: number;
  limit?: number;
  sort_by?: string;
  sort_order?: "asc" | "desc";
};

export interface UserListResponse {
  data: UserEmployeeItem[];
  total: number;
  page: number;
  perPage: number;
}

export interface UserDirectoryStats {
  total: number;
  active: number;
  withAccess: number;
}

export interface UserFormLookups {
  departments: Array<{ id: string; name: string }>;
  sections: Array<{ id: string; name: string }>;
  positions: Array<{ id: string; title: string; department: string }>;
  managers: Array<{ id: string; full_name: string; nip: string }>;
  employmentStatuses: Array<{
    code: string;
    name: string;
    is_active?: boolean;
  }>;
}

export interface EmployeeDocumentInput {
  employee_id: string;
  document_type: string;
  document_name: string;
  file_url: string;
  issue_date?: string;
  expiry_date?: string;
  notes?: string;
}

export interface EmployeeDocumentRow {
  id: string;
  document_type: string;
  document_name: string;
  file_url: string;
  issue_date?: string | null;
  expiry_date?: string | null;
  notes?: string | null;
  created_at?: string;
  is_verified?: boolean;
}

export interface EmploymentHistoryRow {
  id: string;
  change_type: string;
  effective_date: string;
  prev_department?: { name: string } | null;
  new_department?: { name: string } | null;
  prev_job_title?: { title: string } | null;
  new_job_title?: { title: string } | null;
  prev_employment_status?: string | null;
  new_employment_status?: string | null;
  reason?: string | null;
  notes?: string | null;
}

export interface AttendanceRow {
  id: string;
  date: string;
  check_in?: string | null;
  check_out?: string | null;
  clock_in?: string | null;
  clock_out?: string | null;
  status?: string | null;
  work_hours?: number | null;
  is_late?: boolean;
}

export interface LeaveBalanceRow {
  id: string;
  leave_type: string;
  leave_type_name?: string;
  total_days?: number;
  used_days?: number;
  remaining_days?: number;
  balance?: number;
  quota?: number;
  used?: number;
  year?: number;
}
