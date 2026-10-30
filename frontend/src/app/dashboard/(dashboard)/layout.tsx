import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { requireUser } from "@/lib/auth/require-user";
import {
  collectNavHrefs,
  dropNavHref,
  getUserMenus,
  filterNavByPrefixes,
  isEssOnlyUser,
} from "@/lib/iam/get-user-menus";
import { canOpenJadwalToko, JADWAL_TOKO_PATH } from "@/lib/iam/schedule-delegate";
import { ABSENSI_TIM_PATH, hasActiveTeam } from "@/lib/iam/team";
import {
  allowedModulePaths,
  canAccessPath,
  fallbackMenuPath,
  isEssPath,
  isPathAllowedByMenus,
  ESS_HOME_PATH,
} from "@/lib/iam/access";
import { loadGrantedMenuCodesForUser } from "@/lib/iam/has-menu";
import { IamAccessProvider } from "@/components/iam/iam-access-provider";
import { AppSidebar } from "@/components/shared";
import { LayoutLoadError } from "@/components/layout-load-error";
import { getSafeErrorMessage, isNextControlFlowError } from "@/lib/next-control-flow";
import { isPathEnabled } from "@/lib/crm/loyalty-features";
import { getLoyaltyFeatures } from "@/lib/crm/loyalty-features-server";

export default async function DashboardGroupLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  let user;
  try {
    user = await requireUser();
  } catch (error) {
    if (isNextControlFlowError(error)) throw error;
    console.error("[dashboard-layout]", error);
    return (
      <LayoutLoadError
        title="Gagal memuat dashboard"
        message={getSafeErrorMessage(error, "Tidak bisa memuat sesi pengguna.")}
      />
    );
  }
  // Kebijakan ESS-only ditentukan IAM (permission menu non-ESS), bukan daftar role di kode.
  const essOnly = await isEssOnlyUser(user.id, user.role);
  const [allNavItems, grantedCodes, scheduleDelegate, teamManager] = await Promise.all([
    getUserMenus(user.id, user.role),
    loadGrantedMenuCodesForUser(user.id, user.role),
    canOpenJadwalToko(user.id, user.role),
    hasActiveTeam(user.id),
  ]);

  const pathname = (await headers()).get("x-pathname") ?? "";
  const menuAllowed = isPathAllowedByMenus(pathname, collectNavHrefs(allNavItems));
  const fallbackAllowed =
    grantedCodes.length === 0 && canAccessPath(user.role, pathname);
  const allowed =
    !pathname ||
    isEssPath(pathname) ||
    menuAllowed ||
    fallbackAllowed;
  // Halaman fitur loyalty nonaktif (ARK Coin / XP). Guard menu di atas cocok
  // per prefix — menu induk seperti /dashboard/crm akan meloloskan
  // /dashboard/crm/rewards walau menu Rewards sudah disembunyikan.
  const loyaltyBlocked = Boolean(pathname) && !isPathEnabled(pathname, await getLoyaltyFeatures());
  if (!allowed || loyaltyBlocked) {
    if (essOnly) redirect(ESS_HOME_PATH);
    const firstMenu = fallbackMenuPath(collectNavHrefs(allNavItems), pathname);
    redirect(firstMenu ?? allowedModulePaths(user.role)[0] ?? ESS_HOME_PATH);
  }
  // ESS-only: menu Area Karyawan + modul tambahan role. Full-access: buang
  // "Beranda" ESS (/dashboard/me) agar tak ganda dengan Beranda utama.
  const essNav = essOnly
    ? filterNavByPrefixes(allNavItems, [
        ESS_HOME_PATH,
        ...allowedModulePaths(user.role),
      ])
    : allNavItems.filter((item) => item.href !== "/dashboard/me");
  // Menu Jadwal Toko di-grant ke semua role; hanya pengatur aktif dan HR Jadwal Kerja
  // yang melihatnya (EPIC-071 R8, EPIC-073 R1). Buka lewat URL tetap aman: halamannya
  // menjelaskan "bukan pengatur".
  const withStore = scheduleDelegate ? essNav : dropNavHref(essNav, JADWAL_TOKO_PATH);
  // Absensi Tim hanya untuk akun yang punya bawahan aktif (EPIC-074 R6).
  const navItems = teamManager ? withStore : dropNavHref(withStore, ABSENSI_TIM_PATH);

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
