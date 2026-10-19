export interface ScheduleCell { date: string; code: string; source: "day" | "weekly" | "none" }
export interface ScheduleGrid {
  branch: { id: string; name: string; code: string };
  dates: { date: string; label: string; holiday: string | null }[];
  employees: { id: string; nip: string; name: string; position: string; cells: ScheduleCell[] }[];
  non_shift_count: number;
  shifts: { code: string; name: string; start_time: string; end_time: string }[];
}
export interface ScheduleIssue { line: number; column: string; name: string; message: string }
export interface SchedulePreview {
  summary: { employees: number; new: number; changed: number; same: number };
  errors?: ScheduleIssue[];
  warnings?: ScheduleIssue[];
  fingerprint?: string;
  changes?: { nip: string; name: string; date: string; before: string; after: string }[];
}
