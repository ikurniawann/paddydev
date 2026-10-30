import { ApiError, apiGet, apiPost } from "@/lib/api-client";
import { todayWib } from "@/lib/dates";
import { geofenceNoteRequired, type GeofenceNoteInfo } from "@/lib/hris/geofence";
import type {
  EssAnnouncementDetail,
  EssAnnouncementItem,
  EssBeranda,
  EssClockPayload,
  EssClockResponse,
  EssLeaveForm,
  EssLeaveRow,
  EssLoanPayload,
  EssLoanRow,
  EssMe,
  EssOvertimeDayTypeHint,
  EssOvertimeDecision,
  EssOvertimeForm,
  EssOvertimeRow,
  EssPayslip,
  EssTeamMember,
  EssTodayAttendance,
} from "./types";

/**
 * Bacaan ESS sengaja "lunak": gagal memuat = data kosong, sama seperti
 * perilaku halaman sebelumnya (karyawan tidak melihat pesan error mentah).
 */
function softGet<T>(url: string, fallback: T): Promise<T> {
  return apiGet<{ data?: T | null }>(url)
    .then((res) => res.data ?? fallback)
    .catch(() => fallback);
}

export const fetchEssMe = () => softGet<EssMe | null>("/api/hris/me", null);

export const fetchEssBeranda = () => softGet<EssBeranda | null>("/api/hris/me/beranda", null);

export const fetchTodayAttendance = () =>
  softGet<EssTodayAttendance[]>(
    `/api/hris/attendance?employee_id=me&date=${todayWib()}&limit=1`,
    []
  ).then((rows) => rows[0] ?? null);

/** Absen di luar area tanpa keterangan: popup lalu kirim ulang payload yang sama + notes. */
export class ClockGeofenceError extends Error {
  readonly info: GeofenceNoteInfo;
  constructor(info: GeofenceNoteInfo) {
    super(info.message);
    this.name = "ClockGeofenceError";
    this.info = info;
  }
}

export async function submitClock(payload: EssClockPayload): Promise<EssClockResponse> {
  try {
    return await apiPost<EssClockResponse>("/api/hris/attendance", payload);
  } catch (error) {
    if (error instanceof ApiError) {
      const info = geofenceNoteRequired(error.status, error.body);
      if (info) throw new ClockGeofenceError(info);
    }
    throw error;
  }
}

export const fetchMyLeaves = () => softGet<EssLeaveRow[]>("/api/hris/leaves?employee_id=me&limit=20", []);

/** Upload lampiran (bila ada) lalu kirim pengajuan dengan path-nya. */
export async function submitMyLeave(form: EssLeaveForm, attachmentDataUrl?: string) {
  let attachmentUrl: string | undefined;
  if (attachmentDataUrl) {
    const upload = await apiPost<{ data: { path: string } }>("/api/hris/leaves/attachment", {
      photo: attachmentDataUrl,
    });
    attachmentUrl = upload.data.path;
  }
  return apiPost<{ data: unknown }>("/api/hris/leaves", { ...form, attachment_url: attachmentUrl });
}

export const fetchMyOvertime = () => softGet<EssOvertimeRow[]>("/api/hris/overtime?employee_id=me", []);

export const submitOvertime = (form: EssOvertimeForm) =>
  apiPost<{ data: unknown }>("/api/hris/overtime", { ...form, break_minutes: Number(form.break_minutes || 0) });

/** Unggah 1 dokumentasi lembur (PDF/JPG/PNG/WebP ≤ 5 MB); kembalikan path privat. */
export async function uploadOvertimeAttachment(file: File): Promise<string> {
  const body = new FormData();
  body.append("file", file);
  const res = await fetch("/api/hris/overtime/attachments", { method: "POST", body });
  const json = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(json?.error || "Gagal mengunggah dokumentasi");
  return json.data.path as string;
}

export const fetchOvertimeDayType = (date: string) =>
  apiGet<{ data: EssOvertimeDayTypeHint }>(`/api/hris/overtime/day-type?date=${encodeURIComponent(date)}`).then(
    (res) => res.data
  );

export const decideOvertime = (
  id: string,
  action: EssOvertimeDecision,
  rejectionReason?: string
) =>
  apiPost<{ message?: string }>("/api/hris/overtime/decide", {
    overtime_id: id,
    action,
    rejection_reason: rejectionReason,
  });

export const fetchMyLoans = () => softGet<EssLoanRow[]>("/api/hris/loans?employee_id=me", []);

export const fetchTeamLoanApprovals = () =>
  softGet<EssLoanRow[]>("/api/hris/loans?scope=approvals", []);

export const decideTeamLoan = (id: string, approved: boolean, rejectionReason?: string) =>
  apiPost<{ message?: string }>(`/api/hris/loans/${id}/approve`, {
    approved,
    ...(rejectionReason ? { rejection_reason: rejectionReason } : {}),
  });

export const submitLoan = (payload: EssLoanPayload) =>
  apiPost<{ data: unknown }>("/api/hris/loans", payload);

export const fetchMyPayslips = () =>
  softGet<EssPayslip[]>("/api/hris/payslips?employee_id=me", []);

export const fetchAnnouncementFeed = () =>
  softGet<EssAnnouncementItem[]>("/api/hris/announcements/feed", []);

export const fetchAnnouncement = (id: string) =>
  apiGet<{ data: EssAnnouncementDetail }>(`/api/hris/announcements/${id}`).then((res) => res.data);

export const markAnnouncementRead = (id: string) =>
  apiPost<unknown>(`/api/hris/announcements/${id}/read`, {});

export const fetchMyTeam = () =>
  apiGet<{ data: { members: EssTeamMember[] } }>("/api/hris/me/team").then(
    (res) => res.data.members
  );
