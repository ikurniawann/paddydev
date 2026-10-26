import { apiGet, apiPut, buildListUrl } from "@/lib/api-client";
import type {
  LeaveBalanceAccess,
  LeaveBalanceListParams,
  LeaveBalancePayload,
  LeaveBalanceView,
  LeaveSettings,
  LeaveSettingsPatch,
} from "./types";

/** Daftar karyawan aktif dalam cakupan; baris yang belum ada tampil dengan jatah default (`exists: false`). */
export const fetchLeaveBalances = (params: LeaveBalanceListParams) =>
  apiGet<{ data: LeaveBalanceView[] }>(
    buildListUrl("/api/hris/leave-balances", { year: params.year, branch_id: params.branch_id, q: params.q })
  ).then((res) => res.data ?? []);

export const fetchLeaveBalance = (employeeId: string, year: number) =>
  apiGet<{ data: LeaveBalanceView }>(`/api/hris/leave-balances/${employeeId}?year=${year}`).then((res) => res.data);

export const updateLeaveBalance = (employeeId: string, payload: LeaveBalancePayload) =>
  apiPut<{ data: LeaveBalanceView; message: string }>(`/api/hris/leave-balances/${employeeId}`, payload);

/** Objek datar `{read, update, import, export}` tanpa amplop `data`. */
export const fetchLeaveBalanceAccess = () => apiGet<LeaveBalanceAccess>("/api/hris/me/leave-balance-access");

export const fetchLeaveSettings = () =>
  apiGet<{ data: LeaveSettings }>("/api/hris/leave-settings").then((res) => res.data);

export const saveLeaveSettings = (patch: LeaveSettingsPatch) =>
  apiPut<{ data: LeaveSettings; message: string }>("/api/hris/leave-settings", patch);
