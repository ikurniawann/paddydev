"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import { payrollQueryKeys } from "./query-keys";
import {
  createPayrollRun,
  calculatePayroll,
  updatePayrollStatus,
  deletePayrollRun,
  savePayrollSettings,
  notifyPayslip,
  savePayrollInput,
  deletePayrollInput,
  importPayrollInputs,
  type SavePayrollSettingsPayload,
} from "./api";

export function useSavePayrollInput(runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: { employee_id: string; code: string; amount: number; note?: string | null }) =>
      savePayrollInput(runId, payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: payrollQueryKeys.inputs(runId) }),
  });
}

export function useDeletePayrollInput(runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: withDeleteReason(
      "Hapus komponen manual",
      ({ employeeId, code }: { employeeId: string; code: string }, reason) => deletePayrollInput(runId, employeeId, code, reason)
    ),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: payrollQueryKeys.inputs(runId) }),
  });
}

export function useImportPayrollInputs(runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (csv: string) => importPayrollInputs(runId, csv),
    onSuccess: () => qc.invalidateQueries({ queryKey: payrollQueryKeys.inputs(runId) }),
  });
}
import type { CreatePayrollPayload } from "./types";

export function useCreatePayrollRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: CreatePayrollPayload) => createPayrollRun(payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: payrollQueryKeys.all }),
  });
}

export function useCalculatePayroll() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ runId, includeThr }: { runId: string; includeThr: boolean }) =>
      calculatePayroll(runId, includeThr),
    onSuccess: () => qc.invalidateQueries({ queryKey: payrollQueryKeys.all }),
  });
}

export function useUpdatePayrollStatus() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ runId, status }: { runId: string; status: string }) =>
      updatePayrollStatus(runId, status),
    onSuccess: () => qc.invalidateQueries({ queryKey: payrollQueryKeys.all }),
  });
}

export function useDeletePayrollRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: withDeleteReason("Hapus payroll run", (runId: string, reason) => deletePayrollRun(runId, reason)),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: payrollQueryKeys.all }),
  });
}

export function useSavePayrollSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: SavePayrollSettingsPayload) =>
      savePayrollSettings(payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: payrollQueryKeys.all }),
  });
}

/** Kirim notifikasi WA slip terbit; segarkan run agar tanda terkirim muncul. */
export function useNotifyPayslip(runId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payrollDetailId: string) => notifyPayslip(payrollDetailId),
    onSuccess: () => qc.invalidateQueries({ queryKey: payrollQueryKeys.run(runId) }),
  });
}
