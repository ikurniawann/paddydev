"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import { jobPortalQueryKeys } from "./query-keys";
import { saveJobOpening, deleteJobOpening } from "./api";
import type { JobOpeningPayload } from "@/lib/recruitment/job-portal-form";

export function useSaveJobOpening() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ payload, id }: { payload: JobOpeningPayload; id?: string }) =>
      saveJobOpening(payload, id),
    onSuccess: () => qc.invalidateQueries({ queryKey: jobPortalQueryKeys.jobs() }),
  });
}

export function useDeleteJobOpening() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: withDeleteReason("Hapus lowongan", (id: string, reason) => deleteJobOpening(id, reason)),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: jobPortalQueryKeys.jobs() }),
  });
}
