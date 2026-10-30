import { requireUser } from "@/lib/auth/require-user";
import { EssAbsensiTimPage } from "@/features/hris/ess";

export default async function AbsensiTimPage() {
  await requireUser();
  return <EssAbsensiTimPage />;
}
