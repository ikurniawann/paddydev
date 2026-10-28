import type { PlacementSearchFilter } from "./types";

export const placementQueryKeys = {
  all: ["hris", "placements"] as const,
  branches: () => ["hris", "placements", "branches"] as const,
  employee: (employeeId: string) => ["hris", "placements", "employee", employeeId] as const,
  search: (filter: PlacementSearchFilter) => ["hris", "placements", "search", filter] as const,
  clock: (employeeId?: string) => ["hris", "placements", "clock", employeeId ?? "me"] as const,
};
