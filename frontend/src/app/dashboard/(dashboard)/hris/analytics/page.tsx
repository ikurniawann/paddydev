import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { AnalyticsPage } from "@/features/hris/analytics";

export default async function Page() {
  await requireIamPage(IAM.hrisAnalytics, "/dashboard/me");
  return <AnalyticsPage />;
}
