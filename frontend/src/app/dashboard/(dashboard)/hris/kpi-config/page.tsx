import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { KpiConfigPage } from "@/features/hris/kpi";

export default async function Page() {
  await requireIamPage(IAM.hrisPerformance, "/dashboard/me");
  return <KpiConfigPage />;
}
