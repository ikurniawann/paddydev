import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { SchedulesPage } from "@/features/hris/schedules";

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? "";

/**
 * Jadwal Kerja per cabang (EPIC-056, EPIC-070). Tautan Atur jadwal dari
 * Penempatan membawa `branch_id` dan `employees` (id dipisah koma, maks 50).
 */
export default async function HrisSchedulesPage({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>;
}) {
  await requireIamPage(IAM.hrisKepegawaian);
  const sp = await searchParams;
  const employees = first(sp.employees)
    .split(",")
    .map((id) => id.trim())
    .filter(Boolean)
    .slice(0, 50);
  return <SchedulesPage initialBranchId={first(sp.branch_id)} initialEmployeeIds={employees} />;
}
