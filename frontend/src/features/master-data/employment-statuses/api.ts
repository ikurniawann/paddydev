import { apiGet, apiPost, apiPut, apiDelete } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type { EmploymentStatusItem, EmploymentStatusPayload } from "./types";

const BASE = "/api/master/employment-statuses";

export const fetchEmploymentStatusList = () =>
  apiGet<{ data: EmploymentStatusItem[] }>(BASE).then((res) => res.data);

export const createEmploymentStatus = (body: EmploymentStatusPayload) =>
  apiPost<{ data: EmploymentStatusItem; message?: string }>(BASE, body);

export const updateEmploymentStatus = (id: string, body: EmploymentStatusPayload) =>
  apiPut<{ data: EmploymentStatusItem; message?: string }>(`${BASE}/${id}`, body);

export const deleteEmploymentStatus = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`${BASE}/${id}`, reason));
