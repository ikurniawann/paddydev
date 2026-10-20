"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import { leavesQueryKeys } from "./query-keys";
import { createLeave, updateLeave, deleteLeave, approveLeave } from "./api";
import type {
  CreateLeavePayload,
  UpdateLeavePayload,
  ApproveLeavePayload,
} from "./types";

function useInvalidateLeaves() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: leavesQueryKeys.all });
}

export function useCreateLeave() {
  const invalidate = useInvalidateLeaves();
  return useMutation({
    mutationFn: (payload: CreateLeavePayload) => createLeave(payload),
    onSuccess: invalidate,
  });
}

export function useUpdateLeave() {
  const invalidate = useInvalidateLeaves();
  return useMutation({
    mutationFn: ({ id, ...payload }: UpdateLeavePayload & { id: string }) =>
      updateLeave(id, payload),
    onSuccess: invalidate,
  });
}

export function useDeleteLeave() {
  const invalidate = useInvalidateLeaves();
  return useMutation({
    mutationFn: withDeleteReason("Hapus pengajuan cuti/izin", (id: string, reason) => deleteLeave(id, reason)),
    retry: false,
    onSuccess: invalidate,
  });
}

export function useApproveLeave() {
  const invalidate = useInvalidateLeaves();
  return useMutation({
    mutationFn: (payload: ApproveLeavePayload) => approveLeave(payload),
    onSuccess: invalidate,
  });
}
