import { useQuery } from "@tanstack/react-query";
import { fetchScheduleGrid } from "./api";
import { rangeError } from "./schedule-range";

export const scheduleKeys = {
  grid: (p: { branch_id: string; from: string; to: string }) => ["hris", "schedules", p] as const,
};

export function useScheduleGrid(p: { branch_id: string; from: string; to: string }) {
  return useQuery({
    queryKey: scheduleKeys.grid(p),
    queryFn: () => fetchScheduleGrid(p).then((res) => res.data),
    enabled: !!p.branch_id && !rangeError(p.from, p.to),
  });
}
