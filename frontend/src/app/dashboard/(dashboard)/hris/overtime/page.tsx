import { requireIamPage } from "@/lib/auth/require-user";
import { userHasIamAction } from "@/lib/iam/has-menu";
import { IAM } from "@/lib/iam/prefixes";
import { HRD_ROLES } from "@/lib/hris/workforce-auth";
import { OvertimePage } from "@/features/hris/overtime";

/** Aksi update di menu daun Lembur = hak koreksi AdminHRIS (EPIC-063 R6), sama dengan guard Go. */
const OVERTIME_LEAF = ["hris.kepegawaian.overtime"] as const;

// HRIS → Kepegawaian → Lembur: approval pengajuan + penugasan perusahaan + koreksi.
export default async function HrisOvertimePage() {
  const user = await requireIamPage(IAM.hrisKepegawaian);
  const canCorrect = await userHasIamAction(user.id, user.role, OVERTIME_LEAF, "update");
  // Jenis hari saat Setujui hanya untuk HRD (sama dengan Actor.IsHRD di Go, ruling C1/R11).
  const isHrd = (HRD_ROLES as readonly string[]).includes(user.role);
  return <OvertimePage canCorrect={canCorrect} isHrd={isHrd} />;
}
