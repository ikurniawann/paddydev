import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { LiveMonitoringPage } from "@/features/hris/live-monitoring";

export const metadata = { title: "Live Monitoring" };

export default async function Page() {
  await requireIamPage(IAM.hrisRecruitment, "/dashboard/me");
  return <LiveMonitoringPage />;
}
