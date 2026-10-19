import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { EditSalaryPage } from "@/features/hris/salary";

export default async function Page({ params }: { params: Promise<{ id: string }> }) {
  await requireIamPage(IAM.hrisCompensation, "/dashboard/me");
  return <EditSalaryPage params={params} />;
}
