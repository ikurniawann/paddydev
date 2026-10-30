import { apiGet, apiPost, buildListUrl } from "@/lib/api-client";
import { privateFileHref } from "@/lib/hris/attachments";
import type {
  AttendanceGaps,
  AttendanceRequest,
  AttendanceRequestAction,
  AttendanceRequestFilter,
  AttendanceRequestPayload,
} from "./types";

const BASE = "/api/hris/attendance-requests";

export const fetchMyAttendanceRequests = () =>
  apiGet<{ data: AttendanceRequest[] }>("/api/hris/me/attendance-requests").then((res) => res.data ?? []);

export const fetchAttendanceRequests = (filter: AttendanceRequestFilter = {}) =>
  apiGet<{ data: AttendanceRequest[] }>(buildListUrl(BASE, { status: filter.status, employee_id: filter.employee_id })).then(
    (res) => res.data ?? []
  );

export const fetchMyAttendanceGaps = () =>
  apiGet<{ data: AttendanceGaps }>("/api/hris/me/attendance-gaps").then((res) => res.data);

export const createAttendanceRequest = (payload: AttendanceRequestPayload) =>
  apiPost<{ data: AttendanceRequest; message?: string }>(BASE, payload);

export const decideAttendanceRequest = (id: string, action: AttendanceRequestAction, rejectionReason?: string) =>
  apiPost<{ data: AttendanceRequest; message?: string }>(`${BASE}/${id}/decide`, {
    action,
    rejection_reason: rejectionReason,
  });

export const cancelAttendanceRequest = (id: string) =>
  apiPost<{ data: AttendanceRequest; message?: string }>(`${BASE}/${id}/cancel`, {});

/** URL foto bukti (dibaca pemohon dan HR); path dari server. */
export const attendanceRequestAttachmentUrl = (path: string) => privateFileHref(`${BASE}/attachment`, path);
