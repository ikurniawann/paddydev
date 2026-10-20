"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import { departmentsQueryKeys } from "./query-keys";
import { createDepartment, updateDepartment, deleteDepartment } from "./api";
import type { DepartmentPayload } from "./types";

function useInvalidateDepartments() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: departmentsQueryKeys.all });
}

export function useCreateDepartment() {
  const invalidate = useInvalidateDepartments();
  return useMutation({
    mutationFn: (payload: DepartmentPayload) => createDepartment(payload),
    onSuccess: invalidate,
  });
}

export function useUpdateDepartment() {
  const invalidate = useInvalidateDepartments();
  return useMutation({
    mutationFn: ({ id, ...payload }: DepartmentPayload & { id: string }) =>
      updateDepartment(id, payload),
    onSuccess: invalidate,
  });
}

export function useDeleteDepartment() {
  const invalidate = useInvalidateDepartments();
  return useMutation({
    mutationFn: withDeleteReason("Hapus departemen", (id: string, reason) => deleteDepartment(id, reason)),
    retry: false,
    onSuccess: invalidate,
  });
}
