import { apiDelete, apiGet, apiPatch, apiPost } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type { ShiftRow, ShiftSavePayload } from "./types";

/** Daftar shift beserta jumlah karyawan pemakainya (`employees_using`). */
export const fetchShifts = () =>
  apiGet<{ data: ShiftRow[] }>("/api/hris/shifts?with_usage=true").then((res) => res.data ?? []);

export const saveShift = (payload: ShiftSavePayload, id?: string) =>
  id ? apiPatch(`/api/hris/shifts/${id}`, payload) : apiPost("/api/hris/shifts", payload);

export const deleteShift = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/shifts/${id}`, reason)).then((res) => res as { message?: string });
