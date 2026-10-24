import { apiGet, apiPost } from "@/lib/api-client";
import type { OvertimeTimes } from "@/lib/hris/overtime-rules";
import type { TeamApprovals, TeamDecision, TeamRequestKind } from "./types";

const EMPTY: TeamApprovals = { leaves: [], overtime: [] };

export const fetchTeamApprovals = () =>
  apiGet<{ data: TeamApprovals }>("/api/hris/me/team-approvals").then((res) => res.data ?? EMPTY);

/** Keputusan memakai endpoint yang sama dengan HRIS; server menentukan atasan/HR. */
export function decideTeamRequest(
  kind: TeamRequestKind,
  id: string,
  action: TeamDecision,
  rejectionReason?: string,
  times?: Partial<OvertimeTimes>
) {
  if (kind === "leave") {
    return apiPost<{ message?: string }>("/api/hris/leaves/approve", {
      leave_id: id,
      action,
      rejection_reason: rejectionReason,
    });
  }
  return apiPost<{ message?: string }>("/api/hris/overtime/decide", {
    overtime_id: id,
    action,
    rejection_reason: rejectionReason,
    // EPIC-066: koreksi atasan; hanya kolom yang berbeda dari nilai tersimpan.
    ...times,
  });
}
