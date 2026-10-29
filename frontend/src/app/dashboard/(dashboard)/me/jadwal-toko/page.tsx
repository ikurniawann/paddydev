import { requireUser } from "@/lib/auth/require-user";
import { EssJadwalTokoPage } from "@/features/hris/ess";

export default async function JadwalTokoPage() {
  await requireUser();
  return <EssJadwalTokoPage />;
}
