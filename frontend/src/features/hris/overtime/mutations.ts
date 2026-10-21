"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { assignOvertime, correctOvertime, decideOvertime } from "./api";
import { overtimeQueryKeys } from "./query-keys";

export function useAssignOvertime() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: assignOvertime,
    onSuccess: () => qc.invalidateQueries({ queryKey: overtimeQueryKeys.lists() }),
  });
}

export function useDecideOvertime() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: decideOvertime,
    onSuccess: () => qc.invalidateQueries({ queryKey: overtimeQueryKeys.lists() }),
  });
}

export function useCorrectOvertime() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: correctOvertime,
    retry: false,
    onSuccess: () => qc.invalidateQueries({ queryKey: overtimeQueryKeys.lists() }),
  });
}
