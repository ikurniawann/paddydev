import { requireUser } from "@/lib/auth/require-user";
import { EssPerjalananDinasPage } from "@/features/hris/business-trips";

export default async function PerjalananDinasPage() {
  await requireUser();
  return <EssPerjalananDinasPage />;
}
