"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { decideTeamRequest } from "./api";
import { teamApprovalsQueryKeys } from "./query-keys";
import type { TeamDecision, TeamRequestKind } from "./types";

/**
 * Setujui/tolak; segarkan antrean dan badge menu (nav-badges), juga saat server
 * menolak (mis. 409 "sudah diproses orang lain") supaya antrean basi hilang.
 */
export function useDecideTeamRequest() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { kind: TeamRequestKind; id: string; action: TeamDecision; rejectionReason?: string }) =>
      decideTeamRequest(v.kind, v.id, v.action, v.rejectionReason),
    retry: false,
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: teamApprovalsQueryKeys.all });
      void qc.invalidateQueries({ queryKey: ["hris", "nav-badges"] });
    },
  });
}
