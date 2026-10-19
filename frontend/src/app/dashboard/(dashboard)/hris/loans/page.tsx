import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { LoansPage } from "@/features/hris/loans";

// HRIS → Kepegawaian → Pinjaman: kasbon/pinjaman + approval + pelunasan.
// Menu Pinjaman cukup (akun Non-Payroll, EPIC-057); Penggajian tetap boleh.
export default async function HrisLoansPage() {
  await requireIamPage(IAM.hrisLoans, "/dashboard/me");
  return <LoansPage />;
}
