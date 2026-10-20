"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import { employmentStatusesQueryKeys } from "./query-keys";
import {
  createEmploymentStatus,
  updateEmploymentStatus,
  deleteEmploymentStatus,
} from "./api";
import type { EmploymentStatusPayload } from "./types";

function useInvalidateEmploymentStatuses() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: employmentStatusesQueryKeys.all });
}

export function useCreateEmploymentStatus() {
  const invalidate = useInvalidateEmploymentStatuses();
  return useMutation({
    mutationFn: (payload: EmploymentStatusPayload) => createEmploymentStatus(payload),
    onSuccess: invalidate,
  });
}

export function useUpdateEmploymentStatus() {
  const invalidate = useInvalidateEmploymentStatuses();
  return useMutation({
    mutationFn: ({ id, ...payload }: EmploymentStatusPayload & { id: string }) =>
      updateEmploymentStatus(id, payload),
    onSuccess: invalidate,
  });
}

export function useDeleteEmploymentStatus() {
  const invalidate = useInvalidateEmploymentStatuses();
  return useMutation({
    mutationFn: withDeleteReason("Hapus status karyawan", (id: string, reason) => deleteEmploymentStatus(id, reason)),
    retry: false,
    onSuccess: invalidate,
  });
}
