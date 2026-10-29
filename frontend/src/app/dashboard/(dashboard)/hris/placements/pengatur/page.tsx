import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { ScheduleDelegatesPage } from "@/features/hris/schedule-delegates";

// HRIS → Penempatan Karyawan → Pengatur jadwal toko (EPIC-071): HR menunjuk pengatur dan wilayah tokonya.
export default async function HrisScheduleDelegatesPage() {
  await requireIamPage([...IAM.hrisPlacements, ...IAM.hrisKepegawaian, ...IAM.settingsUsers]);
  return <ScheduleDelegatesPage />;
}
