import type { GeofenceStatus } from "@/lib/hris/geofence";

export interface AttendanceExportParams {
  employee_id?: string;
  status?: string;
  start_date?: string;
  end_date?: string;
}

// ── Roster harian (monitoring HRD) ──────────────────────────────────────
export type RosterStatus =
  | "hadir"
  | "terlambat"
  | "belum_absen"
  | "absen"
  | "cuti"
  | "libur_nasional"
  | "libur"
  | "tanpa_jadwal";

export interface RosterShift {
  id: string;
  name: string;
  start_time: string; // "HH:MM:SS"
  end_time: string;
  late_tolerance_minutes: number;
  is_overnight: boolean;
}

export interface RosterAttendance {
  id: string;
  clock_in: string | null;
  clock_out: string | null;
  work_hours: number | null;
  is_late: boolean;
  late_minutes: number;
  clock_in_photo_url: string | null;
  clock_out_photo_url: string | null;
  shift_id: string | null;
  /** Geofence clock-in/out (EPIC-062); null = tidak dinilai */
  clock_in_geofence: GeofenceStatus | null;
  clock_in_distance_m: number | null;
  clock_out_geofence: GeofenceStatus | null;
  clock_out_distance_m: number | null;
  /** Keterangan karyawan (wajib bila di luar area) */
  notes: string | null;
  /** Kurang jam / pulang lebih cepat (EPIC-065 AB-05); opsional karena fallback Next tidak mengisinya */
  is_short_hours?: boolean;
  /** Jam minimum cabang saat dilabel (jam); Go roster mengirim angka */
  min_work_hours?: string | number | null;
}

export interface RosterEmployee {
  employee_id: string;
  full_name: string;
  nip: string | null;
  photo_url: string | null;
  department_name: string | null;
  job_title: string | null;
  shift: RosterShift | null;
  status: RosterStatus;
  is_overdue: boolean;
  attendance: RosterAttendance | null;
  leave: { id: string; leave_type: string | null } | null;
}

export interface DailyRosterData {
  date: string;
  day_of_week: number;
  is_today: boolean;
  summary: Record<RosterStatus | "scheduled", number>;
  shifts: {
    id: string;
    name: string;
    start_time: string;
    end_time: string;
    is_overnight: boolean;
    sort_order: number | null;
  }[];
  /** Hari libur aktif pada tanggal roster (EPIC-036) — kosong bila hari kerja. */
  holidays: { name: string; type: string }[];
  employees: RosterEmployee[];
}

// ── Rekap (list absensi) ────────────────────────────────────────────────
export interface AttendanceListParams {
  employee_id?: string;
  start_date?: string;
  end_date?: string;
  is_late?: boolean;
  /** Filter toko: id cabang, atau "none" untuk absensi tanpa toko */
  branch_id?: string;
  page?: number;
  limit?: number;
}

export interface AttendanceListRow {
  id: string;
  employee_id: string;
  date: string;
  clock_in: string | null;
  clock_out: string | null;
  work_hours: number | null;
  status: string;
  is_late: boolean;
  late_minutes: number;
  clock_in_photo_url: string | null;
  clock_out_photo_url: string | null;
  notes: string | null;
  /** Toko tempat bekerja hari itu (payroll multi-toko) */
  branch_id: string | null;
  shift_id?: string | null;
  is_overtime?: boolean;
  /** numeric(5,2) sebagai string */
  overtime_hours?: string | number | null;
  validated_at?: string | null;
  /** Baris ditambah manual oleh HRD */
  is_manual?: boolean;
  /** Penyesuaian manual terakhir (riwayat di /api/hris/attendance-adjustments) */
  adjusted_at?: string | null;
  adjusted_by_name?: string | null;
  adjustment_count?: number;
  clock_in_geofence?: GeofenceStatus | null;
  clock_in_distance_m?: number | null;
  clock_out_geofence?: GeofenceStatus | null;
  clock_out_distance_m?: number | null;
  /** Kurang jam / pulang lebih cepat (EPIC-065 AB-05) */
  is_short_hours?: boolean;
  /** numeric(4,2) sebagai string, jam minimum cabang saat dilabel */
  min_work_hours?: string | number | null;
  branch: { id: string; code: string; name: string } | null;
  shift: { id: string; name: string } | null;
  employee: {
    id: string;
    full_name: string;
    nip: string | null;
    photo_url: string | null;
  } | null;
}

export interface AttendanceListResponse {
  data: AttendanceListRow[];
  pagination: { page: number; limit: number; total: number; totalPages: number };
}

// ── Statistik bulan & kalender ──────────────────────────────────────────
export interface AttendanceMonthStats {
  month_records: number;
  month_late: number;
  month_off_schedule: number;
  avg_work_hours: number | null;
}

export interface CalendarAttendance {
  id: string;
  date: string;
  clock_in: string | null;
  clock_out: string | null;
  work_hours: number | null;
  status: string;
  is_late: boolean;
  /** Kurang jam / pulang lebih cepat (EPIC-065 AB-05) */
  is_short_hours?: boolean;
  min_work_hours?: string | number | null;
}

/** Baris pola jadwal + detail shift dari /api/hris/attendance/schedule. */
export interface CalendarScheduleRow {
  day_of_week: number;
  shift_id: string | null;
  effective_from: string;
  effective_to: string | null;
  shift_name: string | null;
  start_time: string | null; // "HH:MM:SS"
  end_time: string | null;
  is_overnight: boolean | null;
}

// ── Periode absensi (closing) & penyesuaian manual ─────────────────────
export type AttendancePeriodStatus = "open" | "closed";

export interface AttendancePeriodStats {
  employees: number;
  rows: number;
  without_clock_out: number;
  unvalidated: number;
  adjusted: number;
  late: number;
  /** Pengajuan absensi karyawan yang belum diputus HR di rentang periode. */
  pending_requests?: number;
}

export interface AttendancePeriodEvent {
  id: string;
  period_id: string;
  action: "close" | "auto_close" | "reopen" | "closing_date";
  actor_name: string | null;
  reason: string | null;
  details: Record<string, unknown> | null;
  created_at: string;
}

export interface AttendancePeriodRun {
  id: string;
  run_name: string;
  status: string;
}

export interface AttendancePeriod {
  id: string;
  period_year: number;
  period_month: number;
  /** "Oktober 2026" (bulan payroll) */
  name: string;
  /** "21 Sep – 20 Okt 2026" */
  label: string;
  start_date: string;
  end_date: string;
  closing_date: string;
  closing_date_custom: boolean;
  status: AttendancePeriodStatus;
  is_current: boolean;
  has_ended: boolean;
  days_until_closing: number;
  closed_at: string | null;
  closed_by_name: string | null;
  close_mode: "manual" | "auto" | null;
  close_summary: AttendancePeriodStats | null;
  reopened_at: string | null;
  reopened_by_name: string | null;
  reopen_reason: string | null;
  reopen_count: number;
  stats?: AttendancePeriodStats;
  events?: AttendancePeriodEvent[];
  payroll_runs?: AttendancePeriodRun[];
  blocking_payroll_runs?: AttendancePeriodRun[];
}

export interface AttendancePeriodSettings {
  /** 0 = akhir bulan (bulan kalender), 1–28 = tanggal cut-off */
  cutoff_day: number;
  closing_offset_days: number;
  updated_at: string | null;
  updated_by_name: string | null;
}

export interface AttendancePeriodList {
  settings: AttendancePeriodSettings;
  current: { period_year: number; period_month: number; id: string };
  year: number;
  today: string;
  periods: AttendancePeriod[];
  permissions: { can_manage: boolean; can_reopen: boolean };
}

export interface AttendanceSettingsData {
  settings: AttendancePeriodSettings;
  current_period: AttendancePeriod;
  today: string;
  can_manage: boolean;
}

export type AttendanceRowStatus = "present" | "late" | "absent" | "half-day" | "remote";

/** Body tambah/ubah absensi manual; jam "HH:MM" WIB pada tanggal absensi. */
export interface AttendanceAdjustPayload {
  employee_id?: string;
  date?: string;
  clock_in?: string | null;
  clock_out?: string | null;
  status?: AttendanceRowStatus;
  branch_id?: string | null;
  /** null = hitung otomatis dari shift */
  late_minutes?: number | null;
  overtime_hours?: number | null;
  notes?: string | null;
  reason: string;
}

export interface AttendanceAdjustment {
  id: string;
  attendance_id: string;
  employee_id: string;
  date: string;
  action: "create" | "update" | "delete";
  reason: string;
  changed_fields: string[];
  before: Record<string, unknown> | null;
  after: Record<string, unknown> | null;
  actor_name: string | null;
  created_at: string;
}

/** Pratinjau periode untuk pengaturan yang belum disimpan (server-side, ikut periode beku). */
export interface AttendancePeriodPreviewItem {
  period_year: number;
  period_month: number;
  name: string;
  label: string;
  start_date: string;
  end_date: string;
  closing_date: string;
  status: AttendancePeriodStatus;
  /** true = periode pernah di-closing, rentangnya tetap */
  frozen: boolean;
}

export interface AttendancePeriodPreview {
  cutoff_day: number;
  closing_offset_days: number;
  current: AttendancePeriodPreviewItem;
  next: AttendancePeriodPreviewItem;
}
