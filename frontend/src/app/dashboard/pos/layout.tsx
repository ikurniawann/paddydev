import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { requireUser, type AuthUser } from "@/lib/auth/require-user";
import { dropNavHref, getUserMenus, isEssOnlyUser } from "@/lib/iam/get-user-menus";
import { canOpenJadwalToko, JADWAL_TOKO_PATH } from "@/lib/iam/schedule-delegate";
import { ABSENSI_TIM_PATH, hasActiveTeam } from "@/lib/iam/team";
import { loadGrantedMenuCodesForUser } from "@/lib/iam/has-menu";
import { hasAnyIamMenuPrefix } from "@/lib/iam/match";
import { IAM } from "@/lib/iam/prefixes";
import { canAccessPath, isFullAccessRole } from "@/lib/iam/access";
import type { NavItem } from "@/lib/iam/types";
import { AppSidebar } from "@/components/shared";
import { IamAccessProvider } from "@/components/iam/iam-access-provider";
import { LayoutLoadError } from "@/components/layout-load-error";
import { getSafeErrorMessage, isNextControlFlowError } from "@/lib/next-control-flow";
import { isPathEnabled } from "@/lib/crm/loyalty-features";
import { getLoyaltyFeatures } from "@/lib/crm/loyalty-features-server";

export default async function PosDashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  let user: AuthUser;
  let navItems: NavItem[];
  let essOnly: boolean;
  let grantedCodes: string[];
  try {
    user = await requireUser();
    let scheduleDelegate: boolean;
    let teamManager: boolean;
    [navItems, essOnly, grantedCodes, scheduleDelegate, teamManager] = await Promise.all([
      getUserMenus(user.id, user.role),
      isEssOnlyUser(user.id, user.role),
      loadGrantedMenuCodesForUser(user.id, user.role),
      canOpenJadwalToko(user.id, user.role),
      hasActiveTeam(user.id),
    ]);
    // Menu Jadwal Toko hanya untuk pengatur aktif dan HR Jadwal Kerja (EPIC-073 R1), sama dengan layout dashboard.
    if (!scheduleDelegate) navItems = dropNavHref(navItems, JADWAL_TOKO_PATH);
    // Absensi Tim hanya untuk akun yang punya bawahan aktif (EPIC-074 R6).
    if (!teamManager) navItems = dropNavHref(navItems, ABSENSI_TIM_PATH);
  } catch (error) {
    if (isNextControlFlowError(error)) throw error;
    console.error("[pos-layout]", error);
    return (
      <LayoutLoadError
        title="Gagal memuat POS"
        message={getSafeErrorMessage(error, "Tidak bisa memuat sesi atau menu.")}
      />
    );
  }

  const pathname = (await headers()).get("x-pathname") ?? "";
  const hasPosMenu = hasAnyIamMenuPrefix(grantedCodes, IAM.pos);
  const fallbackAllowed =
    grantedCodes.length === 0 &&
    (isFullAccessRole(user.role) || canAccessPath(user.role, pathname || "/dashboard/pos"));
  if (!hasPosMenu && !fallbackAllowed) {
    redirect("/dashboard");
  }
  // Halaman fitur loyalty yang dinonaktifkan (ARK Coin / XP) di CRM → Pengaturan.
  // Layout ini tidak menjaga path lewat menu, jadi dicek eksplisit.
  if (pathname && !isPathEnabled(pathname, await getLoyaltyFeatures())) {
    redirect("/dashboard/pos");
  }

  return (
    <IamAccessProvider grantedCodes={grantedCodes}>
    <AppSidebar
      user={{
        full_name: user.full_name,
        role: user.role,
        email: user.email,
        company_name: user.company_name,
        branch_id: user.branch_id,
        branch_name: user.branch_name,
        warehouse_name: user.warehouse_name,
        active_stall_id: user.active_stall_id,
        can_switch_stall: user.can_switch_stall,
        can_central_checkout: user.can_central_checkout,
        has_central_cashier_menu: user.has_central_cashier_menu,
      }}
      navItems={navItems}
      essOnly={essOnly}
    >
      {children}
    </AppSidebar>
    </IamAccessProvider>
  );
}
