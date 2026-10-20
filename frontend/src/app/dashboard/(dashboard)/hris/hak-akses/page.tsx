import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { HrisAccessPage } from "@/features/hris/access";

export default async function Page() {
  await requireIamPage(IAM.hrisAccess, "/dashboard/me");
  return <HrisAccessPage />;
}
