import { redirect } from "next/navigation";
import { UserRole } from "@/types";
import { cache } from "react";
import { createServerPgClient } from "@/lib/pg/create-client";
import { queryOne } from "@/lib/db";
import { loadUserWarehouses } from "@/lib/users/user-warehouses";
import { resolveActiveStallFromCookies } from "@/lib/auth/active-stall";
import { getStallAccess } from "@/lib/auth/stall-access";
import { stallDisplayName } from "@/lib/auth/stall-label";
import { resolveRoleIds } from "@/lib/iam/get-user-menus";
import {
  hasAnyIamMenuPrefix,
  hasIamMenuCode,
  loadGrantedMenuCodes,
  loadGrantedMenuCodesForUser,
} from "@/lib/iam/has-menu";
import { CENTRAL_CASHIER_MENU } from "@/lib/pos/central-cashier";

const PROFILE_SELECT =
  "full_name, role, brand_id, business_scope, can_switch_stall, can_central_checkout, default_warehouse_id";
const PROFILE_SELECT_LEGACY =
  "full_name, role, brand_id, business_scope, can_switch_stall, default_warehouse_id";

function isMissingColumnError(error: { code?: string; message?: string } | null): boolean {
  if (!error) return false;
  return (
    error.code === "42703" ||
    error.code === "PGRST204" ||
    /column .* does not exist/i.test(error.message ?? "")
  );
}

async function loadHasCentralCashierMenu(userId: string, role: UserRole): Promise<boolean> {
  try {
    const roleIds = await resolveRoleIds(userId, role);
    return hasIamMenuCode(await loadGrantedMenuCodes(roleIds), CENTRAL_CASHIER_MENU);
  } catch {
    return false;
  }
}

export interface AuthUser {
  id: string;
  full_name: string;
  role: UserRole;
  email: string;
  brand_id: string | null;
  company_name: string | null;
  branch_id: string | null;
  branch_name: string | null;
  warehouse_name: string | null;
  /**
   * Stall aktif pilihan user (switcher sidebar).
   * null = Semua Stall; bila cookie belum pernah diset, fallback ke penempatan user.
   */
  active_stall_id: string | null;
  /** true bila user boleh membuka StallSwitcher (multi-stall / admin / allAccess). */
  can_switch_stall: boolean;
  can_central_checkout: boolean;
  has_central_cashier_menu: boolean;
}

export const getUser = cache(async (): Promise<{
  user: AuthUser | null;
  db: Awaited<ReturnType<typeof createServerPgClient>>;
}> => {
  const db = await createServerPgClient();
  const {
    data: { user },
  } = await db.auth.getUser();

  if (!user) return { user: null, db };

  let { data: profile, error: profileError } = await db
    .from("users")
    .select(PROFILE_SELECT)
    .eq("id", user.id)
    .single();

  if (isMissingColumnError(profileError)) {
    const fallback = await db
      .from("users")
      .select(PROFILE_SELECT_LEGACY)
      .eq("id", user.id)
      .single();
    profile = fallback.data
      ? { ...fallback.data, can_central_checkout: false }
      : fallback.data;
    profileError = fallback.error;
  }

  // Query gagal (kolom belum ada, dsb.) bukan "tidak login" — jangan hapus sesi.
  // PGRST116 = tidak ada baris profil; itu yang boleh dianggap unauthenticated.
  if (profileError && profileError.code !== "PGRST116") {
    throw new Error(`Gagal memuat profil user: ${profileError.message}`);
  }

  if (!profile) return { user: null, db };

  const [scope, warehouses, resolvedStall, hasCentralCashierMenu] = await Promise.all([
    queryOne<{
      company_name: string | null;
      branch_id: string | null;
      branch_name: string | null;
    }>(
      `SELECT c.name AS company_name, u.branch_id, b.name AS branch_name
       FROM configuration.users u
       LEFT JOIN configuration.companies c ON c.id = u.company_id
       LEFT JOIN configuration.branches b ON b.id = u.branch_id
       WHERE u.id = $1`,
      [user.id]
    ),
    loadUserWarehouses(user.id),
    resolveActiveStallFromCookies(),
    loadHasCentralCashierMenu(user.id, profile.role as UserRole),
  ]);

  const access = await getStallAccess(
    user.id,
    profile.role,
    scope?.branch_id ?? null
  );
  const canSwitchStall =
    profile.role === "super_admin" ||
    profile.role === "admin" ||
    profile.can_switch_stall === true ||
    access.allAccess ||
    access.stalls.length > 1;

  const homeStall =
    warehouses.find((row) => row.warehouse_id === profile.default_warehouse_id) ??
    warehouses[0] ??
    null;

  let warehouse_name: string | null;
  let active_stall_id: string | null;

  if (canSwitchStall) {
    const allowedIds = new Set(access.stalls.map((stall) => stall.id));
    const fallbackName =
      (homeStall ? stallDisplayName(access.stalls, { id: homeStall.warehouse_id, name: homeStall.name }) : null) ??
      (access.allAccess ? "Semua Outlet" : null);
    const fallbackId = homeStall?.warehouse_id ?? null;

    if (resolvedStall.mode === "stall" && allowedIds.has(resolvedStall.stall.id)) {
      // Cookie valid & masih dalam penempatan / akses user.
      warehouse_name = stallDisplayName(access.stalls, resolvedStall.stall);
      active_stall_id = resolvedStall.stall.id;
    } else if (resolvedStall.mode === "all" && access.allAccess) {
      // "Semua Stall" hanya bila user memang punya akses bebas.
      warehouse_name = "Semua Outlet";
      active_stall_id = null;
    } else {
      // Cookie kosong / tidak valid / di luar penempatan → ikut tempat user.
      warehouse_name = fallbackName;
      active_stall_id = fallbackId;
    }
  } else {
    warehouse_name = homeStall
      ? homeStall.name
      : warehouses.length > 0
        ? warehouses.map((warehouse) => warehouse.name).join(", ")
        : null;
    active_stall_id = homeStall?.warehouse_id ?? warehouses[0]?.warehouse_id ?? null;
  }

  const isUnscoped =
    profile.role === "super_admin" || !profile.business_scope;

  const companyFromDb = scope?.company_name?.trim() || null;
  const branchFromDb = scope?.branch_name?.trim() || null;

  let company_name: string | null;
  let branch_name: string | null;

  if (isUnscoped) {
    company_name = companyFromDb ?? "Semua";
    branch_name = branchFromDb;
  } else {
    company_name = companyFromDb;
    branch_name = branchFromDb;
  }

  return {
    user: {
      id: user.id,
      full_name: profile.full_name,
      role: profile.role as UserRole,
      email: user.email ?? "",
      brand_id: profile.brand_id,
      company_name,
      branch_id: scope?.branch_id ?? null,
      branch_name,
      warehouse_name,
      active_stall_id,
      can_switch_stall: canSwitchStall,
      can_central_checkout: profile.can_central_checkout === true,
      has_central_cashier_menu: hasCentralCashierMenu,
    },
    db,
  };
});

export const requireUser = cache(async (): Promise<AuthUser> => {
  const { user } = await getUser();
  // Lewat /api/auth/logout (GET) agar cookie session yang tidak valid ikut
  // terhapus — redirect langsung ke /login membuat middleware memantulkan
  // balik ke /dashboard selama cookie masih ada (redirect loop).
  if (!user) redirect("/api/auth/logout");
  return user;
});

/**
 * @deprecated Pakai requireIamPage. Super_admin selalu lolos — bypass IAM.
 */
export async function requireRole(roles: UserRole[]): Promise<AuthUser> {
  const user = await requireUser();
  if (user.role !== "super_admin" && !roles.includes(user.role)) {
    redirect("/dashboard");
  }
  return user;
}

/** Gate halaman pakai grant IAM. Redirect bila prefix tidak di-grant. */
export async function requireIamPage(
  prefixes: readonly string[],
  fallbackHref = "/dashboard"
): Promise<AuthUser> {
  const user = await requireUser();
  const granted = await loadGrantedMenuCodesForUser(user.id, user.role);
  if (!hasAnyIamMenuPrefix(granted, prefixes)) {
    redirect(fallbackHref);
  }
  return user;
}
