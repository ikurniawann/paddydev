"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import { positionsQueryKeys } from "./query-keys";
import { createPosition, updatePosition, deletePosition } from "./api";
import type { PositionPayload } from "./types";

function useInvalidatePositions() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: positionsQueryKeys.all });
}

export function useCreatePosition() {
  const invalidate = useInvalidatePositions();
  return useMutation({
    mutationFn: (payload: PositionPayload) => createPosition(payload),
    onSuccess: invalidate,
  });
}

export function useUpdatePosition() {
  const invalidate = useInvalidatePositions();
  return useMutation({
    mutationFn: ({ id, ...payload }: PositionPayload & { id: string }) =>
      updatePosition(id, payload),
    onSuccess: invalidate,
  });
}

export function useDeletePosition() {
  const invalidate = useInvalidatePositions();
  return useMutation({
    mutationFn: withDeleteReason("Hapus jabatan", (id: string, reason) => deletePosition(id, reason)),
    retry: false,
    onSuccess: invalidate,
  });
}
