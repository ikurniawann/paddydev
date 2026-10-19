import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { SalaryPage } from "@/features/hris/salary";

export default async function Page() {
  await requireIamPage(IAM.hrisCompensation, "/dashboard/me");
  return <SalaryPage />;
}
