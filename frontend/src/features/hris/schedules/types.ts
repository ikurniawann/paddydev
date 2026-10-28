export interface ScheduleCell { date: string; code: string; source: "day" | "weekly" | "none" }
export interface ScheduleShift { code: string; name: string; start_time: string; end_time: string; break_minutes?: number }
export interface ScheduleGrid {
  branch: { id: string; name: string; code: string };
  dates: { date: string; label: string; holiday: string | null }[];
  employees: { id: string; nip: string; name: string; position: string; cells: ScheduleCell[] }[];
  non_shift_count: number;
  shifts: ScheduleShift[];
}
export interface ScheduleIssue { line: number; column: string; name: string; message: string }
export interface SchedulePreview {
  summary: { employees: number; new: number; changed: number; same: number };
  errors?: ScheduleIssue[];
  warnings?: ScheduleIssue[];
  fingerprint?: string;
  changes?: { nip: string; name: string; date: string; before: string; after: string }[];
}

/** Satu kotak yang diubah: kode shift, "L" (libur), atau "" (hapus jadwal per tanggal). Jangan pernah "-". */
export interface CellEdit { employee_id: string; date: string; code: string }

export interface CellsPayload {
  branch_id: string;
  step: "preview" | "commit";
  fingerprint?: string;
  ack_warnings?: boolean;
  reason?: string;
  cells: CellEdit[];
}

export interface CellsPreview {
  summary: { employees: number; new: number; changed: number; same: number; cleared: number };
  errors: ScheduleIssue[];
  warnings: ScheduleIssue[];
  fingerprint: string;
  /** Maks 500; after "" = kembali ikut pola mingguan. */
  changes: { nip: string; name: string; date: string; before: string; after: string }[];
  replaced_days: number;
}
