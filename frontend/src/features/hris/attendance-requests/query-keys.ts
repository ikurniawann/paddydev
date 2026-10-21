import type { AttendanceRequestFilter } from "./types";

export const attendanceRequestQueryKeys = {
  all: ["hris", "attendance-requests"] as const,
  mine: () => ["hris", "attendance-requests", "mine"] as const,
  list: (filter: AttendanceRequestFilter) => ["hris", "attendance-requests", "list", filter] as const,
  gaps: () => ["hris", "attendance-requests", "gaps"] as const,
};
