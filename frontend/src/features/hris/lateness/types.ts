/** Info keterlambatan per periode absensi (EPIC-077). */

export type LatenessCode = "none" | "teguran" | "sp1" | "sp2" | "sp3";

export interface LatenessLevel {
  code: LatenessCode;
  label: string;
  range: string;
  min: number;
  max: number | null;
}

export interface LatenessNext {
  code: LatenessCode;
  label: string;
  at: number;
  remaining: number;
}

export interface LateDay {
  date: string;
  shift_name: string | null;
  /** "HH:MM" WIB. */
  scheduled_start: string | null;
  clock_in: string | null;
  late_minutes: number;
}

export interface LatenessPeriodOption {
  back: number;
  name: string;
  label: string;
}

export interface LatenessPeriod {
  name: string;
  label: string;
  start_date: string;
  end_date: string;
  status: string;
  is_current: boolean;
}

export interface LatenessStats {
  count: number;
  total_minutes: number;
  level: LatenessLevel;
  next: LatenessNext | null;
  days: LateDay[];
}

interface LatenessEnvelope {
  period: LatenessPeriod;
  periods: LatenessPeriodOption[];
  rules: LatenessLevel[];
}

export type MyLateness = LatenessEnvelope & LatenessStats;

export interface TeamLatenessMember extends LatenessStats {
  employee_id: string;
  full_name: string;
  position_title: string | null;
  direct: boolean;
  reporting_to_name: string;
}

export interface TeamLateness extends LatenessEnvelope {
  summary: Record<LatenessCode, number>;
  employees: TeamLatenessMember[];
}
