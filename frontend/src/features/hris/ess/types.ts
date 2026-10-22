import type { GeofenceStatus } from "@/lib/hris/geofence";
import type { OvertimeDayType } from "@/lib/hris/overtime-rules";
import type { PayslipAmounts, SlipAllocation } from "@/features/hris/payroll/types";

export interface EssTodayShift {
  name: string;
  start_time: string | null;
  end_time: string | null;
  late_tolerance_minutes: number;
}

export interface EssLeaveBalance {
  year: number;
  annual_leave_total: string;
  annual_leave_used: string;
  annual_leave_remaining: string;
}

/** GET /api/hris/me: karyawan tertaut akun yang login (null = tidak tertaut). */
export interface EssMe {
  employee: {
    id: string;
    full_name: string;
    nip: string | null;
    position_title: string | null;
    department_name: string | null;
  } | null;
  leave_balance: EssLeaveBalance | null;
  today_shift?: EssTodayShift | null;
  has_schedule?: boolean;
}

export interface EssTodayAttendance {
  id: string;
  clock_in: string | null;
  clock_out: string | null;
  is_late: boolean;
  late_minutes: number;
  /** Toko tempat clock-in (payroll multi-toko), null bila tidak tercatat */
  branch?: { id: string; code: string; name: string } | null;
  clock_in_geofence?: GeofenceStatus | null;
  clock_in_distance_m?: number | null;
  clock_out_geofence?: GeofenceStatus | null;
  clock_out_distance_m?: number | null;
}

export interface EssClockLocation {
  latitude: number;
  longitude: number;
  accuracy?: number;
}

export interface EssClockPayload {
  action: "clock-in" | "clock-out";
  photo: string;
  attendance_id?: string;
  /** Toko clock-in: salah satu penempatan aktif; kosong = server memakai penempatan utama */
  branch_id?: string;
  clock_in_location?: EssClockLocation;
  clock_out_location?: EssClockLocation;
  /** Keterangan; wajib (≥ 5 karakter) bila server menilai di luar area */
  notes?: string;
}

export interface EssClockResponse {
  message?: string;
  data?: {
    is_late?: boolean;
    late_minutes?: number;
    clock_in_geofence?: GeofenceStatus | null;
    clock_in_distance_m?: number | null;
    clock_out_geofence?: GeofenceStatus | null;
    clock_out_distance_m?: number | null;
  };
}

export interface EssLeaveRow {
  id: string;
  leave_type: string;
  start_date: string;
  end_date: string;
  total_days: number;
  status: string;
  reason: string;
  rejection_reason: string | null;
}

export interface EssLeaveForm {
  leave_type: string;
  start_date: string;
  end_date: string;
  reason: string;
}

export interface EssOvertimeRow {
  id: string;
  date: string;
  start_time: string;
  end_time: string;
  hours: string | number;
  source: "employee" | "company";
  status: string;
  reason: string | null;
  rejection_reason: string | null;
  day_type?: OvertimeDayType;
  break_minutes: number;
  attachments: string[];
  requester?: { full_name: string } | null;
}

export interface EssOvertimeForm {
  date: string;
  start_time: string;
  end_time: string;
  break_minutes: string;
  reason: string;
  day_type: OvertimeDayType;
  attachments: string[];
}

/** GET /api/hris/overtime/day-type (EPIC-063): saran jenis dari jadwal; null = jadwal tidak diketahui. */
export interface EssOvertimeDayTypeHint {
  date: string;
  day_type: OvertimeDayType | null;
  label: string | null;
}

export type EssOvertimeDecision = "approve" | "reject" | "cancel";

export interface EssLoanRow {
  id: string;
  employee?: { full_name: string } | null;
  loan_type: string;
  principal_amount: string | number;
  monthly_installment: string | number;
  remaining_balance: string | number;
  paid_amount: string | number;
  tenor_months: number;
  first_installment_month: number | null;
  first_installment_year: number | null;
  status: string;
  purpose: string | null;
  rejection_reason: string | null;
}

export interface EssLoanPayload {
  loan_type: string;
  principal_amount: number;
  tenor_months: number;
  purpose?: string;
}

export interface EssPayslip extends PayslipAmounts {
  overtime_hours?: number;
  /** Rincian biaya per toko (karyawan multi-toko) */
  allocations?: SlipAllocation[];
  payroll_run?: {
    id: string;
    period_month: number;
    period_year: number;
    status: string;
    paid_at: string | null;
    company_name?: string | null;
  } | null;
}

export interface EssAnnouncementItem {
  id: string;
  title: string;
  cover_image_url: string | null;
  video_provider: string | null;
  tags: string[];
  is_pinned: boolean;
  publish_at: string | null;
  created_at: string;
  is_read: boolean;
}

export interface EssAnnouncementDetail extends EssAnnouncementItem {
  body_html: string;
  video_id: string | null;
  created_by_name: string | null;
}

export interface EssTeamMember {
  id: string;
  full_name: string;
  nip: string | null;
  position_title: string | null;
  department_name: string | null;
  photo_url: string | null;
  schedule_summary: string | null;
  /** EPIC-056: Non-Shift memakai jadwal tetap (read-only di Shift Tim). */
  schedule_type?: "shift" | "non_shift";
  schedule_since: string | null;
}

// ── Beranda (/api/hris/me/beranda) ──
export interface EssWeekDay {
  date: string;
  day_of_week: number;
  is_today: boolean;
  status: "shift" | "libur" | "none";
  shift_name: string | null;
  start_time: string | null;
  end_time: string | null;
}

export interface EssRecentRequest {
  kind: "cuti" | "lembur" | "pinjaman";
  id: string;
  label: string;
  detail: string;
  status: string;
  created_at: string;
  href: string;
}

export interface EssBeranda {
  employee: {
    full_name: string;
    nip: string | null;
    join_date: string | null;
    employment_status: string;
    position_title: string | null;
    department_name: string | null;
  } | null;
  leave_balance: Omit<EssLeaveBalance, "year"> | null;
  today_shift: EssTodayShift | null;
  has_schedule: boolean;
  week_schedule: EssWeekDay[];
  attendance: {
    month: number;
    present: number;
    late: number;
    off_schedule: number;
    avg_work_hours: number | null;
    clocked_in_today: boolean;
  };
  latest_payslip: {
    net_salary: number;
    run_name: string | null;
    period_month: number;
    period_year: number;
    paid_at: string | null;
  } | null;
  active_loans: { count: number; total_remaining: number; monthly_installment: number };
  recent_requests: EssRecentRequest[];
  announcements: {
    items: Omit<EssAnnouncementItem, "video_provider">[];
    unread: number;
    total: number;
  };
  kpi: { count: number; avg_achievement: number | null; avg_score: number | null } | null;
}
