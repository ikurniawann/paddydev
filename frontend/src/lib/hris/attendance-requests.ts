import { canManageTrips } from "./business-trips";

/**
 * Pengajuan absen manual / koreksi absen (EPIC-065, MOM AB-04/AB-06, K-9):
 * status, label, dan validasi form. Pesan sama dengan server
 * (domain.CheckAttendanceRequest). Murni, aman diimpor komponen klien.
 */

export type AttendanceRequestStatus = "pending" | "approved" | "rejected" | "cancelled";
export type AttendanceRequestKind = "manual" | "correction";
export type AttendanceGapKind = "missing" | "no_clock_out";

export const ATTENDANCE_REQUEST_STATUS_META: Record<AttendanceRequestStatus, { label: string; className: string }> = {
  pending: { label: "Menunggu HR", className: "bg-amber-100 text-amber-700" },
  approved: { label: "Disetujui", className: "bg-green-100 text-green-700" },
  rejected: { label: "Ditolak", className: "bg-red-100 text-red-700" },
  cancelled: { label: "Dibatalkan", className: "bg-gray-100 text-gray-600" },
};

export const ATTENDANCE_REQUEST_KIND_LABEL: Record<AttendanceRequestKind, string> = {
  manual: "Absen manual",
  correction: "Koreksi absen",
};

export const ATTENDANCE_GAP_LABEL: Record<AttendanceGapKind, string> = {
  missing: "Tidak ada absen",
  no_clock_out: "Belum absen pulang",
};

/** Pemutus = HRD (Actor.IsHRD: super_admin/admin/hrd), sama dengan Perjalanan Dinas (R2). */
export const canDecideAttendanceRequests = canManageTrips;

export interface AttendanceRequestForm {
  date: string;
  /** "HH:MM" atau "" (tidak diajukan) */
  clock_in: string;
  clock_out: string;
  reason: string;
}

export const EMPTY_ATTENDANCE_REQUEST_FORM: AttendanceRequestForm = { date: "", clock_in: "", clock_out: "", reason: "" };

/** Panjang per code point (rune Go), bukan unit UTF-16 `String.length`. */
function runeCount(s: string): number {
  return [...s].length;
}

/** "" bila valid; urutan dan pesan sama dengan server. */
export function validateAttendanceRequestForm(form: AttendanceRequestForm, today: string): string {
  if (!form.date) return "Tanggal wajib diisi";
  if (form.date > today) return "Tanggal pengajuan tidak boleh di masa depan";
  if (runeCount(form.reason.trim()) < 5) return "Keterangan wajib diisi (minimal 5 karakter)";
  if (!form.clock_in && !form.clock_out) return "Isi jam masuk dan/atau jam pulang yang diajukan";
  return "";
}

/** Body POST /api/hris/attendance-requests: jam kosong = null (tidak diajukan). */
export function toAttendanceRequestPayload(form: AttendanceRequestForm, photo?: string) {
  return {
    date: form.date,
    clock_in: form.clock_in || null,
    clock_out: form.clock_out || null,
    reason: form.reason.trim(),
    ...(photo ? { photo } : {}),
  };
}

/** "Masuk 08:00 · Pulang 17:00" (hanya jam yang diajukan). */
export function attendanceRequestClocks(r: { clock_in: string | null; clock_out: string | null }): string {
  return [r.clock_in && `Masuk ${r.clock_in}`, r.clock_out && `Pulang ${r.clock_out}`].filter(Boolean).join(" · ");
}

/** Petunjuk form saat dibuka dari kartu "Absen terlewat". */
export function gapHint(kind: AttendanceGapKind, clockIn: string | null): string {
  return kind === "no_clock_out"
    ? `Tercatat masuk ${clockIn ?? "—"} tanpa absen pulang — isi jam pulang yang benar.`
    : "Tidak ada absen tercatat — isi jam masuk dan jam pulang yang benar.";
}
