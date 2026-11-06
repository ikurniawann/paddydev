import { requireIamPage } from "@/lib/auth/require-user";
import { userHasIamAction } from "@/lib/iam/has-menu";
import { IAM } from "@/lib/iam/prefixes";
import { DisciplinaryLettersPage } from "@/features/hris/disciplinary-letters";

// HRIS → Kehadiran & Cuti → Surat Peringatan (EPIC-094): membaca cukup prefix menu;
// terbitkan butuh aksi create, batalkan butuh update (sama dengan rute Go).
export default async function HrisDisciplinaryLettersPage() {
  const user = await requireIamPage(IAM.hrisDisciplinaryLetters);
  const [canIssue, canRevoke] = await Promise.all([
    userHasIamAction(user.id, user.role, IAM.hrisDisciplinaryLetters, "create"),
    userHasIamAction(user.id, user.role, IAM.hrisDisciplinaryLetters, "update"),
  ]);
  return <DisciplinaryLettersPage canIssue={canIssue} canRevoke={canRevoke} />;
}
