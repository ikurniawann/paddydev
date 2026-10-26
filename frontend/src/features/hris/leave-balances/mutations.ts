"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { essQueryKeys } from "@/features/hris/ess/query-keys";
import { previewLeaveSettings, resolveCarryReview, saveLeaveSettings, updateLeaveBalance } from "./api";
import { leaveBalanceQueryKeys } from "./query-keys";
import type { LeaveBalancePayload, LeaveSettingsPatch, LeaveSettingsSavePatch } from "./types";

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
    mutationFn: (patch: LeaveSettingsSavePatch) => saveLeaveSettings(patch),
    onSuccess: (res) => {
      // Respons PUT menjadi isi cache lebih dulu: kartu yang menghapus isian
      // sementaranya tidak sempat menampilkan nilai lama sebelum refetch.
      qc.setQueryData(leaveBalanceQueryKeys.settings(), res.data);
      void qc.invalidateQueries({ queryKey: leaveBalanceQueryKeys.settings() });
      // Batas baru menghitung ulang sisa bawaan tahun berjalan: daftar saldo dan tinjauan ikut segar.
      void qc.invalidateQueries({ queryKey: leaveBalanceQueryKeys.all });
      void qc.invalidateQueries({ queryKey: essQueryKeys.all });
    },
  });
}

/** Pratinjau dampak patch pengaturan; tidak menyimpan dan tidak mengubah cache. */
export function usePreviewLeaveSettings() {
  return useMutation({ mutationFn: (patch: LeaveSettingsPatch) => previewLeaveSettings(patch) });
}

/** HR menandai satu tinjauan sisa bawaan selesai (catatan wajib); daftar saldo dan tinjauan ikut segar. */
export function useResolveCarryReview() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, note }: { id: string; note: string }) => resolveCarryReview(id, note),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: leaveBalanceQueryKeys.all });
    },
  });
}
