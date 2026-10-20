import { buildListUrl, apiGet, apiPost, apiPut, apiDelete } from "@/lib/api-client";
import type {
  LeaveItem,
  LeaveListParams,
  LeaveEmployeeLite,
  CreateLeavePayload,
  UpdateLeavePayload,
  ApproveLeavePayload,
  LeaveQuota,
} from "./types";

const BASE = "/api/hris/leaves";

export const fetchLeaveList = (params?: LeaveListParams) =>
  apiGet<{ data: LeaveItem[] }>(buildListUrl(BASE, params)).then((res) => res.data);

export const createLeave = (body: CreateLeavePayload) =>
  apiPost<{ data: LeaveItem; message?: string }>(BASE, body);

export const updateLeave = (id: string, body: UpdateLeavePayload) =>
  apiPut<{ data: LeaveItem; message?: string }>(`${BASE}/${id}`, body);

export const deleteLeave = (id: string) => apiDelete(`${BASE}/${id}`);

export const approveLeave = (body: ApproveLeavePayload) =>
  apiPost<{ data: LeaveItem; wa_link?: string | null }>(`${BASE}/approve`, body);

export const fetchLeaveEmployees = () =>
  apiGet<{ data: LeaveEmployeeLite[] }>("/api/hris/employees?is_active=true&limit=500&sort_by=full_name&sort_order=asc").then(
    (res) => res.data
  );

/** Unggah foto lampiran (data URL); balikan path privat untuk attachment_url. */
export const uploadLeaveAttachment = (photo: string, employeeId: string) =>
  apiPost<{ data: { path: string } }>(`${BASE}/attachment`, {
    photo,
    employee_id: employeeId,
  }).then((res) => res.data.path);

export async function downloadLeavesCsv(params?: LeaveListParams) {
  const res = await fetch(buildListUrl(`${BASE}/export`, params));
  if (!res.ok) {
    const json = await res.json().catch(() => ({}));
    throw new Error((json as { error?: string }).error || "Export failed");
  }
  return res.blob();
}

export const fetchLeaveQuota = (month: string, employeeId?: string) => {
  const qs = new URLSearchParams({ month });
  if (employeeId) qs.set("employee_id", employeeId);
  return apiGet<{ data: LeaveQuota }>(`/api/hris/leave-quota?${qs}`)
    .then((res) => res.data)
    .catch(() => null);
};
