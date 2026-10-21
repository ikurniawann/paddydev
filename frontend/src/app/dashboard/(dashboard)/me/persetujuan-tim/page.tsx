import { requireUser } from "@/lib/auth/require-user";
import { EssPersetujuanTimPage } from "@/features/hris/team-approvals";

// ESS → Persetujuan Tim (EPIC-063): antrean atasan langsung; datanya per akun dari API.
export default async function PersetujuanTimPage() {
  await requireUser();
  return <EssPersetujuanTimPage />;
}
