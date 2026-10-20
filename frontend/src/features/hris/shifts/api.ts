import { apiDelete, apiGet, apiPatch, apiPost } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type { ShiftPayload, ShiftRow } from "./types";

export const fetchShifts = () =>
  apiGet<{ data: ShiftRow[] }>("/api/hris/shifts").then((res) => res.data ?? []);

export const saveShift = (payload: ShiftPayload, id?: string) =>
  id ? apiPatch(`/api/hris/shifts/${id}`, payload) : apiPost("/api/hris/shifts", payload);

export const deleteShift = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/shifts/${id}`, reason));
