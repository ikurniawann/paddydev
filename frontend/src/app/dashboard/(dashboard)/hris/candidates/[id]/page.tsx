import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { CandidateDetailPage } from "@/features/hris/candidates";

export default async function Page() {
  await requireIamPage(IAM.hrisRecruitment, "/dashboard/me");
  return <CandidateDetailPage />;
}
