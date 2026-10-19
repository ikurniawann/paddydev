import { hasAnyIamMenuPrefix } from "@/lib/iam/match";
import { IAM } from "@/lib/iam/prefixes";

const ROLE_HOME: Record<string, string> = {
  purchasing_admin: "/dashboard/purchasing",
  purchasing_manager: "/dashboard/purchasing",
  purchasing_staff: "/dashboard/purchasing",
  pos: "/dashboard/pos",
  pos_supervisor: "/dashboard/pos",
  finance_staff: "/dashboard/hris/payroll",
};

const HR_ROLES = new Set(["hrd", "admin", "hiring_manager"]);

/**
 * Beranda per peran (EPIC-021). HR tanpa Rekrutmen/Analytics (akun
 * Non-Rekrutmen, keputusan 19-10-2026) ke daftar karyawan, bukan dashboard
 * rekrutmen yang tidak boleh dibukanya.
 */
export function homeFor(role: string, granted: readonly string[]): string {
  const home = ROLE_HOME[role] ?? "/dashboard/rekrutmen";
  if (
    home === "/dashboard/rekrutmen" &&
    HR_ROLES.has(role) &&
    !hasAnyIamMenuPrefix(granted, [...IAM.hrisRecruitment, ...IAM.hrisAnalytics])
  ) {
    return "/dashboard/employees";
  }
  return home;
}
