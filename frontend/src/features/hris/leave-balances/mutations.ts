"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { essQueryKeys } from "@/features/hris/ess/query-keys";
import { saveLeaveSettings, updateLeaveBalance } from "./api";
import { leaveBalanceQueryKeys } from "./query-keys";
import type { LeaveBalancePayload, LeaveSettingsPatch } from "./types";

/** HR mengubah saldo satu karyawan (wajib alasan); daftar, tab karyawan, dan ESS ikut segar. */
export function useUpdateLeaveBalance() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ employeeId, payload }: { employeeId: string; payload: LeaveBalancePayload }) =>
      updateLeaveBalance(employeeId, payload),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: leaveBalanceQueryKeys.all });
      void qc.invalidateQueries({ queryKey: essQueryKeys.all });
    },
  });
}

export function useSaveLeaveSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (patch: LeaveSettingsPatch) => saveLeaveSettings(patch),
    onSuccess: () => void qc.invalidateQueries({ queryKey: leaveBalanceQueryKeys.settings() }),
  });
}
