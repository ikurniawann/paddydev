"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import { payrollQueryKeys } from "@/features/hris/payroll/query-keys";
import { cancelAttendanceRequest, createAttendanceRequest, decideAttendanceRequest } from "./api";
import { attendanceRequestQueryKeys } from "./query-keys";
import type { AttendanceRequestAction, AttendanceRequestPayload } from "./types";

/** Pengajuan, absensi (persetujuan menulis absensi), dan badge menu. */
function useInvalidateAttendanceRequests() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: attendanceRequestQueryKeys.all });
    void qc.invalidateQueries({ queryKey: attendanceQueryKeys.all });
    void qc.invalidateQueries({ queryKey: ["hris", "nav-badges"] });
  };
}

export function useCreateAttendanceRequest() {
  const onSuccess = useInvalidateAttendanceRequests();
  return useMutation({
    mutationFn: (payload: AttendanceRequestPayload) => createAttendanceRequest(payload),
    retry: false,
    onSuccess,
  });
}

/** Disegarkan juga saat gagal (mis. 400 "sudah dibatalkan") agar daftar basi hilang. */
export function useCancelAttendanceRequest() {
  const onSettled = useInvalidateAttendanceRequests();
  return useMutation({ mutationFn: (id: string) => cancelAttendanceRequest(id), retry: false, onSettled });
}

/**
 * Disegarkan juga saat gagal (409 "sudah diproses orang lain") agar antrean basi hilang.
 * Persetujuan menulis absensi, yang mengalir ke penutupan periode dan payroll, jadi
 * periode absensi dan payroll ikut disegarkan (pembuatan/pembatalan tidak perlu).
 */
export function useDecideAttendanceRequest() {
  const qc = useQueryClient();
  const refreshRequests = useInvalidateAttendanceRequests();
  const onSettled = () => {
    refreshRequests();
    void qc.invalidateQueries({ queryKey: attendanceQueryKeys.periodsAll() });
    void qc.invalidateQueries({ queryKey: payrollQueryKeys.all });
  };
  return useMutation({
    mutationFn: (vars: { id: string; action: AttendanceRequestAction; rejectionReason?: string }) =>
      decideAttendanceRequest(vars.id, vars.action, vars.rejectionReason),
    retry: false,
    onSettled,
  });
}
