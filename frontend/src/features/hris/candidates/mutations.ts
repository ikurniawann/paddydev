"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import type { CandidateCreateInput } from "@/lib/recruitment/candidate-query";
import { candidatesQueryKeys } from "./query-keys";
import { createCandidate, deleteCandidate, updateCandidateStatus, addCandidateNote } from "./api";

export function useCreateCandidate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ payload, cvFile }: { payload: CandidateCreateInput; cvFile?: File | null }) =>
      createCandidate(payload, cvFile),
    onSuccess: () => qc.invalidateQueries({ queryKey: candidatesQueryKeys.all }),
  });
}

export function useDeleteCandidate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: withDeleteReason("Hapus kandidat", (id: string, reason) => deleteCandidate(id, reason)),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: candidatesQueryKeys.all }),
  });
}

export function useUpdateCandidateStatus() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) => updateCandidateStatus(id, status),
    onSuccess: (_data, variables) => qc.invalidateQueries({ queryKey: candidatesQueryKeys.detail(variables.id) }),
  });
}

export function useAddCandidateNote() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, content }: { id: string; content: string }) => addCandidateNote(id, content),
    onSuccess: (_data, variables) => qc.invalidateQueries({ queryKey: candidatesQueryKeys.detail(variables.id) }),
  });
}
