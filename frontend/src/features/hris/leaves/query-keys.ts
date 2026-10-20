import type { LeaveListParams } from "./types";

export const leavesQueryKeys = {
  all: ["hris", "leaves"] as const,
  list: (params?: LeaveListParams) =>
    ["hris", "leaves", "list", params ?? {}] as const,
  quota: (month: string, employeeId?: string) =>
    ["hris", "leaves", "quota", month, employeeId ?? "me"] as const,
  employees: () => ["hris", "leaves", "employees-lite"] as const,
};
