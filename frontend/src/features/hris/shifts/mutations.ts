"use client";

import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import { essQueryKeys } from "@/features/hris/ess/query-keys";
import { usersQueryKeys } from "@/features/users/query-keys";
import { deleteShift, saveShift } from "./api";
import { shiftQueryKeys } from "./query-keys";
import type { ShiftSavePayload } from "./types";

/**
 * Perubahan master shift langsung terlihat di halaman lain: pilihan shift di
 * form karyawan, grid jadwal, dan Shift Tim ESS tidak boleh menunggu cache basi.
 */
function invalidateShiftDependents(qc: QueryClient) {
  return Promise.all([
    qc.invalidateQueries({ queryKey: shiftQueryKeys.all }),
    qc.invalidateQueries({ queryKey: usersQueryKeys.shiftOptions() }),
    qc.invalidateQueries({ queryKey: ["hris", "schedules"] }),
    qc.invalidateQueries({ queryKey: essQueryKeys.team() }),
  ]);
}

export function useSaveShift() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ payload, id }: { payload: ShiftSavePayload; id?: string }) => saveShift(payload, id),
    onSuccess: () => invalidateShiftDependents(qc),
  });
}

export function useDeleteShift() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: withDeleteReason("Hapus shift", (id: string, reason) => deleteShift(id, reason)),
    retry: false,
    onSuccess: () => invalidateShiftDependents(qc),
  });
}
