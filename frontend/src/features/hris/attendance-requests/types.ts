import type { AttendanceGapKind, AttendanceRequestKind, AttendanceRequestStatus } from "@/lib/hris/attendance-requests";

/** Baris hris.attendance_requests (GET /api/hris/attendance-requests, /api/hris/me/attendance-requests). */
export interface AttendanceRequest {
  id: string;
  employee_id: string;
  date: string; // YYYY-MM-DD
  kind: AttendanceRequestKind;
  /** "HH:MM" WIB yang diajukan; null = tidak diajukan */
  clock_in: string | null;
  clock_out: string | null;
  reason: string;
  /** Path privat foto bukti (dibaca lewat attendanceRequestAttachmentUrl) */
  attachment_path: string | null;
  status: AttendanceRequestStatus;
  attendance_id: string | null;
  decided_by: string | null;
  decided_by_name: string | null;
  decided_at: string | null;
  rejection_reason: string | null;
  created_at: string;
  updated_at: string;
  employee: { id: string; full_name: string; nip: string | null } | null;
  /**
   * Absensi yang tercatat pada tanggal itu saat ini; null bila belum ada.
   * clock_in/clock_out di sini timestamp ISO (kolom hris.attendance), bukan "HH:MM".
   */
  recorded: { id: string; clock_in: string | null; clock_out: string | null; status: string } | null;
}

export interface AttendanceRequestPayload {
  date: string;
  clock_in: string | null;
  clock_out: string | null;
  reason: string;
  /** Data URL JPG/PNG/WebP, maks 5 MB (opsional) */
  photo?: string;
}

export type AttendanceRequestAction = "approve" | "reject";

export interface AttendanceRequestFilter {
  status?: AttendanceRequestStatus;
  employee_id?: string;
}

export interface AttendanceGapDay {
  date: string;
  kind: AttendanceGapKind;
  /** Jam masuk tercatat (WIB "HH:MM") untuk "Belum absen pulang". */
  clock_in: string | null;
}

export interface AttendanceGaps {
  /** null bila akun tidak tertaut ke karyawan */
  start: string | null;
  end: string | null;
  days: AttendanceGapDay[];
}
