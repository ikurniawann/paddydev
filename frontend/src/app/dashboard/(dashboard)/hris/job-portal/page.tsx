import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { JobPortalPage } from "@/features/hris/job-portal";

export default async function Page() {
  await requireIamPage(IAM.hrisRecruitment, "/dashboard/me");
  return <JobPortalPage />;
}
