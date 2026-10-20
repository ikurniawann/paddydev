/** GET /api/hris/leave-quota (EPIC-058): kuota bulanan, hari saja (tanpa angka gaji). */
export interface LeaveQuota {
  employee_id: string;
  month: string;
  position_level: string;
  leader_eligible: boolean;
  types: { leave_type: string; label: string; quota: number; used: number; remaining: number; available: boolean }[];
}

export interface LeaveEmployeeLite {
  id: string;
  full_name: string;
  nip: string;
  department?: { name: string } | null;
}

export interface LeaveItem {
  id: string;
  employee_id: string;
  leave_type: string;
  start_date: string;
  end_date: string;
  total_days: number;
  reason: string;
  status: string;
  created_at: string;
  employee?: {
    full_name: string;
    nip: string;
    department?: { name: string };
  };
  approver?: {
    full_name: string;
  };
}

export type LeaveListParams = {
  status?: string;
  leave_type?: string;
  limit?: number;
};

export interface CreateLeavePayload {
  employee_id: string;
  leave_type: string;
  start_date: string;
  end_date: string;
  reason: string;
  attachment_url?: string;
}

export type UpdateLeavePayload = Partial<{
  status: string;
  reason: string;
  attachment_url: string;
}>;

export interface ApproveLeavePayload {
  leave_id: string;
  action: "approve" | "reject";
  rejection_reason?: string;
}
