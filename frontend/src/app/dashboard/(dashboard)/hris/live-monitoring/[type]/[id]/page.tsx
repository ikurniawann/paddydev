import { notFound } from "next/navigation";
import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { LiveMonitorDetailPage } from "@/features/hris/live-monitoring";

export const metadata = { title: "Live Monitoring" };

export default async function Page({
  params,
}: {
  params: Promise<{ type: string; id: string }>;
}) {
  await requireIamPage(IAM.hrisRecruitment, "/dashboard/me");
  const { type, id } = await params;
  if (type !== "psikotes" && type !== "interview") notFound();
  return <LiveMonitorDetailPage type={type} sessionId={id} />;
}
