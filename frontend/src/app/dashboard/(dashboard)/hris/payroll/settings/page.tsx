import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { PayrollSettingsPage } from "@/features/hris/payroll";

export default async function Page() {
  await requireIamPage(IAM.hrisCompensation, "/dashboard/me");
  return <PayrollSettingsPage />;
}
