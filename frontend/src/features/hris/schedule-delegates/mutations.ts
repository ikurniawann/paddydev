"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { revokeScheduleDelegate, saveScheduleDelegate } from "./api";
import { SCHEDULE_ACCESS_KEY, scheduleDelegateKeys } from "./query-keys";
import type { ScheduleDelegatePayload } from "./types";

function useInvalidateDelegates() {
  const qc = useQueryClient();
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: scheduleDelegateKeys.all }),
      qc.invalidateQueries({ queryKey: SCHEDULE_ACCESS_KEY }),
    ]);
}

export function useSaveDelegate() {
  const onSuccess = useInvalidateDelegates();
  return useMutation({
    mutationFn: ({ employeeId, payload }: { employeeId: string; payload: ScheduleDelegatePayload }) =>
      saveScheduleDelegate(employeeId, payload),
    onSuccess,
  });
}

export function useRevokeDelegate() {
  const onSuccess = useInvalidateDelegates();
  return useMutation({ mutationFn: (employeeId: string) => revokeScheduleDelegate(employeeId), retry: false, onSuccess });
}
