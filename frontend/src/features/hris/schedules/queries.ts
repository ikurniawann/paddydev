import { useQuery } from "@tanstack/react-query";
import { retryUnlessForbidden } from "@/features/hris/schedule-access/access-guard";
import { fetchScheduleGrid } from "./api";
import { rangeError } from "./schedule-range";

type GridParams = { branch_id: string; from: string; to: string };

export const scheduleKeys = {
  grid: (p: GridParams) => ["hris", "schedules", p] as const,
};

/** `store` = mode pengatur jadwal toko: 403 (akses dicabut) tidak dicoba ulang. */
export function useScheduleGrid(p: GridParams, store = false) {
  return useQuery({
    ...(store ? { retry: retryUnlessForbidden } : {}),
    queryKey: scheduleKeys.grid(p),
    queryFn: () => fetchScheduleGrid(p).then((res) => res.data),
    enabled: !!p.branch_id && !rangeError(p.from, p.to),
    // Pindah minggu: grid lama tetap tampil (centang & pilihan tidak hilang).
    // Ganti cabang: jangan tampilkan data cabang lain.
    placeholderData: (prev, prevQuery) =>
      (prevQuery?.queryKey[2] as GridParams | undefined)?.branch_id === p.branch_id ? prev : undefined,
  });
}
