import { apiGet, apiPost, apiPut, apiDelete } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type { DepartmentItem, DepartmentPayload } from "./types";

const BASE = "/api/master/departments";

export const fetchDepartmentList = () =>
  apiGet<{ data: DepartmentItem[] }>(BASE).then((res) => res.data);

export const createDepartment = (body: DepartmentPayload) =>
  apiPost<{ data: DepartmentItem; message?: string }>(BASE, body);

export const updateDepartment = (id: string, body: DepartmentPayload) =>
  apiPut<{ data: DepartmentItem; message?: string }>(`${BASE}/${id}`, body);

export const deleteDepartment = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`${BASE}/${id}`, reason));
