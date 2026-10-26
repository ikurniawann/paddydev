import { requireIamPage } from "@/lib/auth/require-user";
import { userHasIamAction } from "@/lib/iam/has-menu";
import { IAM } from "@/lib/iam/prefixes";
import { OvertimePage } from "@/features/hris/overtime";

/** Aksi update di menu daun Lembur = hak koreksi AdminHRIS (EPIC-063 R6), sama dengan guard Go. */
const OVERTIME_LEAF = ["hris.kepegawaian.overtime"] as const;

// HRIS → Kepegawaian → Lembur: approval pengajuan + penugasan perusahaan + koreksi.
export default async function HrisOvertimePage() {
  const user = await requireIamPage(IAM.hrisKepegawaian);
  const canCorrect = await userHasIamAction(user.id, user.role, OVERTIME_LEAF, "update");
  return <OvertimePage canCorrect={canCorrect} />;
}
