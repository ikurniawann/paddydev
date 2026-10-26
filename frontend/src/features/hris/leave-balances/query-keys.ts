import type { LeaveBalanceListParams } from "./types";

export const leaveBalanceQueryKeys = {
  all: ["hris", "leave-balances"] as const,
  list: (params: LeaveBalanceListParams) => ["hris", "leave-balances", "list", params] as const,
  one: (employeeId: string, year: number) => ["hris", "leave-balances", "one", employeeId, year] as const,
  access: () => ["hris", "me", "leave-balance-access"] as const,
  settings: () => ["hris", "leave-settings"] as const,
};
