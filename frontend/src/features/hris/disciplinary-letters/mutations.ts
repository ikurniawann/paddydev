"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { issueLetter, revokeLetter } from "./api";
import { letterKeys } from "./query-keys";
import type { IssueLetterPayload } from "./types";

export function useIssueLetter() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (payload: IssueLetterPayload) => issueLetter(payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: letterKeys.all }),
  });
}

export function useRevokeLetter() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => revokeLetter(id, reason),
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: letterKeys.all }),
  });
}
