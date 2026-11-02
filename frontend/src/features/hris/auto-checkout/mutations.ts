"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createAutoCheckoutRule, deleteAutoCheckoutRule, setAutoCheckoutRuleActive } from "./api";
import { autoCheckoutKeys } from "./query-keys";
import type { AutoCheckoutRulePayload } from "./types";

function useInvalidateRules() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: autoCheckoutKeys.all });
}

export function useCreateAutoCheckoutRule() {
  const onSuccess = useInvalidateRules();
  return useMutation({ mutationFn: (payload: AutoCheckoutRulePayload) => createAutoCheckoutRule(payload), retry: false, onSuccess });
}

export function useToggleAutoCheckoutRule() {
  const onSuccess = useInvalidateRules();
  return useMutation({
    mutationFn: ({ id, isActive }: { id: string; isActive: boolean }) => setAutoCheckoutRuleActive(id, isActive),
    retry: false,
    onSuccess,
  });
}

export function useDeleteAutoCheckoutRule() {
  const onSuccess = useInvalidateRules();
  return useMutation({ mutationFn: (id: string) => deleteAutoCheckoutRule(id), retry: false, onSuccess });
}
