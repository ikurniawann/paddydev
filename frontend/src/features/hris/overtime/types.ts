import type { OvertimeDayType, OvertimeDecisionAction } from "@/lib/hris/overtime-rules";

export type { OvertimeDayType };

export interface OvertimeRow {
  id: string;
  employee_id: string;
  date: string;
  start_time: string;
  end_time: string;
  hours: string | number;
  source: "employee" | "company";
  status: string;
  reason: string | null;
  rejection_reason: string | null;
  /** rest_day = hari istirahat mingguan, dibayar tarif libur (EPIC-055). */
  day_type?: OvertimeDayType;
  /** EPIC-066: istirahat (menit) dan path dokumentasi privat. */
  break_minutes?: number;
  attachments?: string[];
  /** EPIC-063: kapasitas pemutus (atasan langsung / HR cadangan) dan koreksi AdminHRIS terakhir. */
  decided_as?: "manager" | "hr" | null;
  decided_by_name?: string | null;
  corrected_at?: string | null;
  corrected_by_name?: string | null;
  employee?: { id: string; full_name: string; nip: string | null; reporting_to?: string | null } | null;
  requester?: { full_name: string } | null;
  decider?: { full_name: string } | null;
}

export interface OvertimeFilters {
  status: string;
  /** "YYYY-MM" atau "" */
  month: string;
}

export interface OvertimeDecision {
  overtime_id: string;
  action: OvertimeDecisionAction;
  rejection_reason?: string;
  day_type?: OvertimeDayType;
}

/** PATCH /api/hris/overtime/{id} (AP-04). */
export interface OvertimeCorrection {
  id: string;
  date: string;
  start_time: string;
  end_time: string;
  day_type: OvertimeDayType;
  reason: string;
}
