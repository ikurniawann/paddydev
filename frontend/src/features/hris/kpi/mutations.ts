"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { withDeleteReason } from "@/lib/hris/delete-reason";
import { kpiQueryKeys } from "./query-keys";
import {
  createDeptTask,
  createPerfCycle,
  patchPerfReview,
  saveKpiConfig,
  createKpiTarget,
  deleteKpiTarget,
  runKpiSnapshotApi,
  saveKpiRubric,
  updateDeptOccurrence,
  updateScorecardStatus,
} from "./api";
import type {
  CreateDeptTaskPayload,
  CreateKpiTargetPayload,
  DeptOccurrenceAction,
  PerfReviewPatch,
  SaveKpiConfigPayload,
  SaveRubricPayload,
} from "./types";

function useInvalidateKpi() {
  const qc = useQueryClient();
  return () => qc.invalidateQueries({ queryKey: kpiQueryKeys.all });
}

export function useRunKpiSnapshot() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: (payload: { period_month: number; period_year: number }) =>
      runKpiSnapshotApi(payload),
    onSuccess: invalidate,
  });
}

export function useSaveKpiRubric() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: (payload: SaveRubricPayload) => saveKpiRubric(payload),
    onSuccess: invalidate,
  });
}

export function useUpdateScorecardStatus() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: (payload: { action: "finalize" | "reopen"; scorecard_id: string }) =>
      updateScorecardStatus(payload),
    onSuccess: invalidate,
  });
}

export function useCreateKpiTarget() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: (payload: CreateKpiTargetPayload) => createKpiTarget(payload),
    onSuccess: invalidate,
  });
}

export function useDeleteKpiTarget() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: withDeleteReason("Hapus target KPI", (id: string, reason) => deleteKpiTarget(id, reason)),
    retry: false,
    onSuccess: invalidate,
  });
}

export function useDeptOccurrenceAction() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: ({ occurrenceId, body }: { occurrenceId: string; body: DeptOccurrenceAction }) =>
      updateDeptOccurrence(occurrenceId, body),
    onSuccess: invalidate,
  });
}

export function useCreateDeptTask() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: (payload: CreateDeptTaskPayload) => createDeptTask(payload),
    onSuccess: invalidate,
    retry: false,
  });
}

export function useCreatePerfCycle() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: ({ year, quarter }: { year: number; quarter: number }) =>
      createPerfCycle(year, quarter),
    onSuccess: invalidate,
    retry: false,
  });
}

export function usePatchPerfReview() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: PerfReviewPatch }) =>
      patchPerfReview(id, payload),
    onSuccess: invalidate,
  });
}

export function useSaveKpiConfig() {
  const invalidate = useInvalidateKpi();
  return useMutation({
    mutationFn: (payload: SaveKpiConfigPayload) => saveKpiConfig(payload),
    onSuccess: invalidate,
  });
}
