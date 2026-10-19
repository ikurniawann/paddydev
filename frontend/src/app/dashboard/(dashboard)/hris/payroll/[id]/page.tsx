import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { PayrollDetailPage } from "@/features/hris/payroll";

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  await requireIamPage(IAM.hrisCompensation, "/dashboard/me");
  return <PayrollDetailPage params={params} />;
}
