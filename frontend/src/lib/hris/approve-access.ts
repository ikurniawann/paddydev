/**
 * Jalur HR keputusan cuti/lembur/absensi/Perjalanan Dinas (EPIC-067 R10/R11):
 * aksi "approve" pada menu daun, bukan role aplikasi. Fallback Next memakai
 * aturan yang sama dengan Go Actor.CanApprove (R15). Atasan langsung tetap
 * memutus bawahannya tanpa menu; aturan itu ada di overtime-rules.ts.
 */
import { userHasIamAction } from "@/lib/iam/has-menu";
import type { UserRole } from "@/types";
import type { WorkforceActor } from "./workforce-auth";

/** Daun keputusan HR (sama dengan Go iam.LeafAbsensi, iam.LeafCuti, iam.LeafLembur). */
export const HRIS_APPROVE_LEAF = {
  absensi: "hris.workforce.attendance",
  cuti: "hris.workforce.leaves",
  lembur: "hris.kepegawaian.overtime",
} as const;

export type HrisApproveLeaf = (typeof HRIS_APPROVE_LEAF)[keyof typeof HRIS_APPROVE_LEAF];

/** True bila role IAM akun memberi "approve" pada daun (atau turunannya). */
export function canApproveLeaf(actor: Pick<WorkforceActor, "userId" | "role">, leaf: HrisApproveLeaf): Promise<boolean> {
  return userHasIamAction(actor.userId, actor.role as UserRole, [leaf], "approve");
}
