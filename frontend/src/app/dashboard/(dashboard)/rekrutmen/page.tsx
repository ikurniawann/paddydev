import { requireIamPage } from "@/lib/auth/require-user";
import { IAM } from "@/lib/iam/prefixes";
import { RecruitmentDashboardPage } from "@/features/hris/dashboard";

/**
 * Dashboard rekrutmen — sebelumnya menempati /dashboard (EPIC-021).
 * Dipindah ke sini karena /dashboard kini milik ringkasan eksekutif.
 * Pembacanya sama dengan API dashboard: menu Rekrutmen atau Analytics
 * (EPIC-057: akun Non-Rekrutmen ditolak).
 */
export default async function Page() {
  await requireIamPage([...IAM.hrisRecruitment, ...IAM.hrisAnalytics], "/dashboard/me");
  return <RecruitmentDashboardPage />;
}
