import { requireIamPage } from "@/lib/auth/require-user";
import { userHasIamAction } from "@/lib/iam/has-menu";
import { IAM } from "@/lib/iam/prefixes";
import { ScheduleDelegatesPage } from "@/features/hris/schedule-delegates";

// HRIS → Penempatan Karyawan → Pengatur jadwal toko (EPIC-071): HR menunjuk pengatur dan wilayah tokonya.
// Membaca cukup prefix menu; menunjuk/mengubah/mencabut butuh aksi update (sama dengan PUT/DELETE Go).
const PLACEMENT_MANAGERS = [...IAM.hrisPlacements, ...IAM.hrisKepegawaian, ...IAM.settingsUsers];

export default async function HrisScheduleDelegatesPage() {
  const user = await requireIamPage(PLACEMENT_MANAGERS);
  const canManage = await userHasIamAction(user.id, user.role, PLACEMENT_MANAGERS, "update");
  return <ScheduleDelegatesPage canManage={canManage} />;
}
