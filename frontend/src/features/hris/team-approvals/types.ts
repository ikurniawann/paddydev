import type { OvertimeDayType } from "@/lib/hris/overtime-rules";

/** GET /api/hris/me/team-approvals (EPIC-063): pengajuan pending bawahan langsung. */
export interface TeamMemberRef {
  id: string;
  full_name: string;
  nip: string | null;
}

export interface TeamLeaveRequest {
  id: string;
  employee_id: string;
  leave_type: string;
  start_date: string;
  end_date: string;
  total_days: string | number;
  reason: string | null;
  attachment_url: string | null;
  created_at: string;
  employee: TeamMemberRef;
}

export interface TeamOvertimeRequest {
  id: string;
  employee_id: string;
  date: string;
  start_time: string;
  end_time: string;
  hours: string | number;
  day_type: OvertimeDayType;
  /** EPIC-066: istirahat (menit) dan path dokumentasi privat. */
  break_minutes: number;
  attachments: string[];
  reason: string | null;
  created_at: string;
  employee: TeamMemberRef;
}

export interface TeamApprovals {
  leaves: TeamLeaveRequest[];
  overtime: TeamOvertimeRequest[];
}

export type TeamRequestKind = "leave" | "overtime";
export type TeamDecision = "approve" | "reject";

/** GET /api/hris/me/team-history (EPIC-090): semua status, 2 periode absensi. */
export interface TeamHistoryLeave extends TeamLeaveRequest {
  status: string;
  attachment_review: "awaiting" | "returned" | "verified" | null;
  attachment_return_reason: string | null;
  rejection_reason: string | null;
  decided_by_name: string | null;
  manager_approved_name: string | null;
  attachment_reviewed_name: string | null;
}

export interface TeamHistoryOvertime extends TeamOvertimeRequest {
  status: string;
  /** employee = diajukan karyawan, company = penugasan HR. */
  source: string;
  rejection_reason: string | null;
  decided_by_name: string | null;
}

/** EPIC-091: kasbon/pinjaman bawahan langsung (Manager tetap penyetuju). */
export interface TeamHistoryLoan {
  id: string;
  employee_id: string;
  loan_type: string;
  principal_amount: string | number;
  monthly_installment: string | number;
  tenor_months: number;
  status: string;
  purpose: string | null;
  rejection_reason: string | null;
  /** Manager penyetuju; null = tanpa Manager (atasan terdekat/HR). */
  manager_approver_name: string | null;
  /** Tahap SPV sebelum Manager (EPIC-091); null = tanpa tahap SPV. */
  supervisor_approver_name?: string | null;
  supervisor_approved?: boolean;
  attachments: string[];
  collateral_attachments: string[];
  created_at: string;
  employee: TeamMemberRef;
}

export interface TeamHistory {
  period: { from: string; to: string; label: string };
  leaves: TeamHistoryLeave[];
  overtime: TeamHistoryOvertime[];
  loans?: TeamHistoryLoan[];
}

