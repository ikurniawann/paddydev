import { redirect } from "next/navigation";
import { requireUser } from "@/lib/auth/require-user";
import { homeFor } from "@/lib/auth/home-for";
import { loadGrantedMenuCodesForUser } from "@/lib/iam/has-menu";
import { ExecutiveDashboardPage } from "@/features/dashboard/executive";

/**
 * Landing /dashboard (EPIC-021, keputusan owner 2026-07-21):
 * - super_admin + direksi → Ringkasan Eksekutif lintas modul.
 * - employee → Area Karyawan (aturan lama dipertahankan).
 * - Role lain diarahkan ke dashboard modul yang paling relevan dengan
 *   pekerjaannya (Fase B, lihat homeFor): purchasing → dashboard purchasing;
 *   pos → dashboard POS; finance → payroll; sisanya (hrd, admin, dst) →
 *   dashboard rekrutmen, kecuali HR tanpa menu Rekrutmen/Analytics → daftar
 *   karyawan (EPIC-057).
 */
export default async function Page() {
  const user = await requireUser();
  if (user.role === "employee") redirect("/dashboard/me");
  if (user.role === "super_admin" || user.role === "direksi") {
    return <ExecutiveDashboardPage />;
  }
  const granted = await loadGrantedMenuCodesForUser(user.id, user.role);
  redirect(homeFor(user.role, granted));
}
