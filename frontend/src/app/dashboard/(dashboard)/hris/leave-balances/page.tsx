import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { LeaveBalancesPage } from "@/features/hris/leave-balances";

/** Saldo Cuti (EPIC-068): jatah, penyesuaian, terpakai, sisa bawaan per karyawan + pengaturan bawaan. */
export default async function HrisLeaveBalancesPage() {
  await requireIamPage(IAM.hrisLeaveBalances);
  return <LeaveBalancesPage />;
}
