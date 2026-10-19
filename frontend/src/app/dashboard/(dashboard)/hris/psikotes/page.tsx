import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { PsikotesAdminPage } from "@/features/hris/psikotes";

export default async function Page() {
  await requireIamPage(IAM.hrisRecruitment, "/dashboard/me");
  return <PsikotesAdminPage />;
}
