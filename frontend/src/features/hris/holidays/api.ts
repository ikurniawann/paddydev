import { apiDelete, apiGet, apiPatch, apiPost } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type { HolidayImportItem, HolidayImportRow, HolidayPayload, HolidayRow } from "./types";

export const fetchHolidays = (year: number) =>
  apiGet<{ data: HolidayRow[] }>(`/api/hris/holidays?year=${year}&include_draft=1`).then(
    (res) => res.data ?? []
  );

export const fetchHolidayImportPreview = (year: number) =>
  apiGet<{ data: HolidayImportRow[] }>(`/api/hris/holidays/import?year=${year}`).then(
    (res) => res.data ?? []
  );

export const saveHoliday = (payload: HolidayPayload, id?: string) =>
  id ? apiPatch(`/api/hris/holidays/${id}`, payload) : apiPost("/api/hris/holidays", payload);

export const deleteHoliday = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/holidays/${id}`, reason));

export const importHolidays = (items: HolidayImportItem[]) =>
  apiPost<{ message?: string }>("/api/hris/holidays/import", { items });
