import { apiGet, apiPut } from "@/lib/api-client";
import type { HrisAccessChange, HrisAccessView, PermissionAuditRow } from "./types";

export const fetchHrisAccess = () => apiGet<{ data: HrisAccessView }>("/api/hris/access").then((res) => res.data);

export const saveHrisRoleAccess = (role: string, changes: HrisAccessChange[]) =>
  apiPut<{ data: HrisAccessView }>(`/api/hris/access/roles/${role}`, { changes }).then((res) => res.data);

export const saveHrisAccount = (userId: string, role: string) =>
  apiPut<{ data: HrisAccessView }>(`/api/hris/access/accounts/${userId}`, { role }).then((res) => res.data);

export const fetchPermissionAudit = () =>
  apiGet<{ data: PermissionAuditRow[] }>("/api/hris/access/audit").then((res) => res.data);
