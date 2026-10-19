import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { PipelinePage } from "@/features/hris/pipeline";

export default async function Page() {
  await requireIamPage(IAM.hrisRecruitment, "/dashboard/me");
  return <PipelinePage />;
}
