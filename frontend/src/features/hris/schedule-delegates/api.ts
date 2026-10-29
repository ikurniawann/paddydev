import { apiDelete, apiGet, apiPut, buildListUrl } from "@/lib/api-client";
import type { DelegateSuggest, ScheduleDelegate, ScheduleDelegatePayload } from "./types";

const BASE = "/api/hris/schedule-delegates";

export const fetchScheduleDelegates = () =>
  apiGet<{ data: ScheduleDelegate[] }>(BASE).then((res) => res.data ?? []);

/** Saran toko wilayah: penempatan berjalan orang itu ditambah seluruh bawahannya. */
export const fetchDelegateSuggest = (employeeId: string) =>
  apiGet<{ data: DelegateSuggest }>(buildListUrl(`${BASE}/suggest`, { employee_id: employeeId })).then(
    (res) => res.data ?? { branch_ids: [] }
  );

export const saveScheduleDelegate = (employeeId: string, payload: ScheduleDelegatePayload) =>
  apiPut<{ data: ScheduleDelegate }>(`${BASE}/${employeeId}`, payload).then((res) => res.data);

export const revokeScheduleDelegate = (employeeId: string) =>
  apiDelete(`${BASE}/${employeeId}`);
