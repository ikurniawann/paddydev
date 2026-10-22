"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import type { BranchLocationPayload } from "@/lib/hris/geofence";
import { bulkPlacement, createPlacement, deletePlacement, saveBranchLocation, saveBranchMinWorkHours, updatePlacement } from "./api";
import { placementQueryKeys } from "./query-keys";
import type { BulkPlacementPayload, PlacementPayload } from "./types";

function useInvalidatePlacements() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: placementQueryKeys.all });
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

export function useBulkPlacement() {
  const onSuccess = useInvalidatePlacements();
  return useMutation({ mutationFn: (payload: BulkPlacementPayload) => bulkPlacement(payload), onSuccess });
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
