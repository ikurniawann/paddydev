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
