import type { UserListParams } from "./types";

export const usersQueryKeys = {
  all: ["users"] as const,
  list: (params?: UserListParams) => ["users", "list", params ?? {}] as const,
  detail: (id: string) => ["users", "detail", id] as const,
  directoryStats: () => ["users", "directory-stats"] as const,
  withoutManager: () => ["users", "without-manager"] as const,
  formLookups: () => ["users", "form-lookups"] as const,
  hrisEmployee: (id: string) => ["users", "hris-employee", id] as const,
  documents: (employeeId: string) => ["users", "documents", employeeId] as const,
  recruitmentDocs: (employeeId: string) => ["users", "recruitment-docs", employeeId] as const,
  employmentHistory: (employeeId: string) => ["users", "employment-history", employeeId] as const,
  attendance: (employeeId: string, month: number, year: number) =>
    ["users", "attendance", employeeId, month, year] as const,
  leaveBalances: (employeeId: string) => ["users", "leave-balances", employeeId] as const,
  lifecycle: (employeeId: string) => ["users", "lifecycle", employeeId] as const,
  contracts: (employeeId: string) => ["users", "contracts", employeeId] as const,
  expiringContracts: (days: number) => ["users", "expiring-contracts", days] as const,
  shiftOptions: () => ["users", "shift-options"] as const,
  shiftSchedule: (employeeId: string) => ["users", "shift-schedule", employeeId] as const,
};
