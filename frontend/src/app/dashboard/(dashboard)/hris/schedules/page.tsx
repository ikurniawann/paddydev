import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { SchedulesPage } from "@/features/hris/schedules";

/** Jadwal Kerja per cabang (EPIC-056): bulk upload jadwal shift 7–45 hari. */
export default async function HrisSchedulesPage() {
  await requireIamPage(IAM.hrisKepegawaian);
  return <SchedulesPage />;
}
