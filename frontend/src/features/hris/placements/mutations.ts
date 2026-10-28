"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import type { BranchLocationPayload } from "@/lib/hris/geofence";
import { bulkEndPlacement, bulkPlacement, createPlacement, deletePlacement, saveBranchLocation, saveBranchMinWorkHours, updatePlacement } from "./api";
import { placementQueryKeys } from "./query-keys";
import type { BulkEndPlacementPayload, BulkPlacementPayload, PlacementPayload } from "./types";

/** Jadwal Kerja ikut berubah (karyawan tanpa penempatan tidak bisa dijadwalkan). */
const SCHEDULES_KEY = ["hris", "schedules"] as const;

function useInvalidatePlacements() {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: placementQueryKeys.all }),
      qc.invalidateQueries({ queryKey: SCHEDULES_KEY }),
    ]);
}

export function useCreatePlacement(employeeId: string) {
  const onSuccess = useInvalidatePlacements();
  return useMutation({
    mutationFn: (payload: PlacementPayload) => createPlacement(employeeId, payload),
    onSuccess,
  });
}

export function useUpdatePlacement() {
  const onSuccess = useInvalidatePlacements();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: PlacementPayload }) => updatePlacement(id, payload),
    onSuccess,
  });
}

export function useDeletePlacement() {
  const onSuccess = useInvalidatePlacements();
  return useMutation({ mutationFn: withDeleteReason("Hapus penempatan", (id: string, reason) => deletePlacement(id, reason)), retry: false, onSuccess });
}

/** Invalidasi hanya setelah tersimpan; pratinjau tidak mengubah data. */
function useInvalidateAfterCommit() {
  const qc = useQueryClient();
  return (step: string | undefined) => {
    if (step === "preview") return;
    void qc.invalidateQueries({ queryKey: placementQueryKeys.all });
    void qc.invalidateQueries({ queryKey: SCHEDULES_KEY });
  };
}

export function useBulkPlacement() {
  const invalidate = useInvalidateAfterCommit();
  return useMutation({
    mutationFn: (payload: BulkPlacementPayload) => bulkPlacement(payload),
    // Dengan mode tanpa step, server menganggapnya pratinjau.
    onSuccess: (_res, payload) => invalidate(payload.mode && !payload.step ? "preview" : payload.step),
  });
}

export function useBulkEndPlacement() {
  const invalidate = useInvalidateAfterCommit();
  return useMutation({
    mutationFn: (payload: BulkEndPlacementPayload) => bulkEndPlacement(payload),
    onSuccess: (_res, payload) => invalidate(payload.step),
  });
}

export function useSaveBranchLocation() {
  const onSuccess = useInvalidatePlacements();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: BranchLocationPayload }) => saveBranchLocation(id, payload),
    onSuccess,
  });
}

export function useSaveBranchMinWorkHours() {
  const qc = useQueryClient();
  const invalidatePlacements = useInvalidatePlacements();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: { min_work_hours: number | null } }) =>
      saveBranchMinWorkHours(id, payload),
    onSuccess: () => {
      void invalidatePlacements();
      // Label kurang jam di absensi periode terbuka ikut berubah.
      void qc.invalidateQueries({ queryKey: attendanceQueryKeys.all });
    },
  });
}
