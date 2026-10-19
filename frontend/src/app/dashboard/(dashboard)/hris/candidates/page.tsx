import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { CandidatesPage } from "@/features/hris/candidates";

export default async function Page() {
  await requireIamPage(IAM.hrisRecruitment, "/dashboard/me");
  return <CandidatesPage />;
}
