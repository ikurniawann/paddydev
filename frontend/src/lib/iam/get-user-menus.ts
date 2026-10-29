import { cache } from "react";
import type { UserRole } from "@/types";
import { filterMenusByLoyaltyFeatures } from "@/lib/crm/loyalty-features";
import { getLoyaltyFeatures } from "@/lib/crm/loyalty-features-server";
import { isEssOnlyRole } from "./access";
import { iamDbQuery, isIamDbConfigured } from "./pg-client";
import type { MenuRow, NavIconName, NavItem } from "./types";
import { isNavVisibleMenuType } from "./types";

const DEFAULT_ICON: NavIconName = "clipboard";

const VALID_ICONS = new Set<string>([
  "home",
  "users",
  "clipboard",
  "star",
  "chart",
  "settings",
  "logout",
  "briefcase",
  "shopping",
  "cube",
  "pr",
  "po",
  "reports",
  "sitemap",
  "database",
  "building",
  "identification",
  "calendar",
  "dollar-sign",
  "money",
  "user-plus",
  "file-text",
  "chart-bar",
  "plus",
  "paper-airplane",
  "check-circle",
  "chart-pie",
  "arrow-down-on-square",
  "truck",
  "document-magnifying-glass",
  "circle-stack",
  "document-text",
  "clipboard-document-check",
  "megaphone",
  "banknotes",
  "ticket",
  "clock",
  "video",
  "map",
  "palette",
  "plug",
  "shopping-bag",
  "store",
  "package",
  "bar-chart-3",
]);

function toNavIcon(icon: string | null | undefined): NavIconName {
  if (icon && VALID_ICONS.has(icon)) {
    return icon as NavIconName;
  }
  return DEFAULT_ICON;
}

function buildMenuTree(menus: MenuRow[]): NavItem[] {
  const byParent = new Map<string | null, MenuRow[]>();

  for (const menu of menus) {
    if (!isNavVisibleMenuType(menu.menu_type)) {
      continue;
    }

    const key = menu.parent_id;
    const list = byParent.get(key) ?? [];
    list.push(menu);
    byParent.set(key, list);
  }

  const sortMenus = (items: MenuRow[]) =>
    [...items].sort((a, b) => a.order_number - b.order_number || a.menu_name.localeCompare(b.menu_name));

  const toNavItem = (menu: MenuRow): NavItem | null => {
    if (!isNavVisibleMenuType(menu.menu_type)) {
      return null;
    }

    const childRows = sortMenus(byParent.get(menu.id) ?? []);
    const children = childRows
      .map((child) => toNavItem(child))
      .filter((child): child is NavItem => child !== null);

    if (menu.menu_type === "group" && children.length === 0) {
      const href = menu.route_path?.trim();
      if (!href || href === "#") {
        return null;
      }
    }

    const href = menu.route_path ?? "#";

    return {
      href,
      label: menu.menu_name,
      icon: toNavIcon(menu.icon),
      ...(children.length > 0 ? { children } : {}),
    };
  };

  return sortMenus(byParent.get(null) ?? [])
    .map((menu) => toNavItem(menu))
    .filter((item): item is NavItem => item !== null);
}

/** IAM role ids: `iam.user_roles` rows, else `iam.roles` by `configuration.users.role` code. */
export async function resolveRoleIds(userId: string, role: UserRole): Promise<string[]> {
  const userRoles = await iamDbQuery<{ role_id: string }>(
    `SELECT role_id FROM iam.user_roles WHERE user_id = $1`,
    [userId]
  );

  if (userRoles.length > 0) {
    return userRoles.map((row) => row.role_id);
  }

  const roleRow = await iamDbQuery<{ id: string }>(
    `SELECT id FROM iam.roles WHERE code = $1 AND deleted_at IS NULL LIMIT 1`,
    [role]
  );

  return roleRow[0]?.id ? [roleRow[0].id] : [];
}

async function fetchPermittedMenuIds(roleIds: string[]): Promise<Set<string>> {
  if (roleIds.length === 0) {
    return new Set();
  }

  const rows = await iamDbQuery<{ menu_id: string }>(
    `SELECT menu_id
     FROM iam.role_menu_permissions
     WHERE role_id = ANY($1::uuid[]) AND is_active = true`,
    [roleIds]
  );

  return new Set(rows.map((row) => row.menu_id));
}

async function includeAncestorMenus(menuIds: Set<string>): Promise<Set<string>> {
  const expanded = new Set(menuIds);
  let pending = [...menuIds];

  while (pending.length > 0) {
    const rows = await iamDbQuery<{ id: string; parent_id: string | null }>(
      `SELECT id, parent_id
       FROM iam.menus
       WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL`,
      [pending]
    );

    pending = [];
    for (const row of rows) {
      if (row.parent_id && !expanded.has(row.parent_id)) {
        expanded.add(row.parent_id);
        pending.push(row.parent_id);
      }
    }
  }

  return expanded;
}

async function fetchMenusByIds(menuIds: string[]): Promise<MenuRow[]> {
  if (menuIds.length === 0) {
    return [];
  }

  return iamDbQuery<MenuRow>(
    `SELECT id, parent_id, code, menu_name, route_path, icon, order_number, level, menu_type, is_visible, is_active
     FROM iam.menus
     WHERE id = ANY($1::uuid[])
       AND deleted_at IS NULL
       AND is_active = true
       AND is_visible = true
     ORDER BY level ASC, order_number ASC`,
    [menuIds]
  );
}

function isNavigableHref(href: string | undefined): href is string {
  return Boolean(href && href !== "#");
}

/**
 * Semua href navigable di pohon nav — TERMASUK parent ber-href (beda dari
 * flattenModuleNavLinks yang hanya daun). Dipakai guard path layout
 * (fix H1 EPIC-032 A4): pathname harus tercakup salah satu href grant.
 */
export function collectNavHrefs(items: NavItem[]): string[] {
  const result: string[] = [];
  for (const item of items) {
    if (isNavigableHref(item.href)) result.push(item.href);
    if (item.children?.length) result.push(...collectNavHrefs(item.children));
  }
  return result;
}

/** Flatten module menu groups into leaf sidebar links for top-nav bars (POS, etc.). */
export function flattenModuleNavLinks(items: NavItem[]): NavItem[] {
  const result: NavItem[] = [];

  for (const item of items) {
    if (item.children?.length) {
      result.push(...flattenModuleNavLinks(item.children));
      continue;
    }

    if (isNavigableHref(item.href)) {
      result.push({ href: item.href, label: item.label, icon: item.icon });
    }
  }

  return result;
}

function isIamUnavailable(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  const message = error.message.toLowerCase();
  return (
    message.includes("database_url belum diset") ||
    message.includes('schema "iam" does not exist') ||
    message.includes("relation") && message.includes("does not exist")
  );
}

export const getUserMenus = cache(async (userId: string, role: UserRole): Promise<NavItem[]> => {
  if (!isIamDbConfigured()) {
    return [];
  }

  try {
    const roleIds = await resolveRoleIds(userId, role);
    const permittedIds = await fetchPermittedMenuIds(roleIds);

    if (permittedIds.size === 0) {
      return [];
    }

    const allMenuIds = await includeAncestorMenus(permittedIds);
    const [menus, loyaltyFeatures] = await Promise.all([
      fetchMenusByIds([...allMenuIds]),
      getLoyaltyFeatures(),
    ]);

    // Menu fitur loyalty yang dinonaktifkan (ARK Coin / XP) disaring di sini,
    // jadi hilang dari sidebar, desktop, dan navigasi POS sekaligus — dan guard
    // path layout (collectNavHrefs) ikut menutup akses langsung ke halamannya.
    const visibleMenus = filterMenusByLoyaltyFeatures(menus, loyaltyFeatures);

    if (visibleMenus.length === 0) {
      return [];
    }

    return buildMenuTree(visibleMenus);
  } catch (error) {
    if (isIamUnavailable(error)) {
      return [];
    }

    console.error("[iam] getUserMenus failed:", error);
    return [];
  }
});

/**
 * Kebijakan akses berbasis IAM: user dianggap full-access bila role IAM-nya
 * punya permission aktif pada menu di luar Area Karyawan (kode `ess`/`ess.*`).
 *
 * Fallback ke kebijakan kode (`isEssOnlyRole`) bila IAM tidak tersedia atau
 * user belum punya role/permission sama sekali — mencegah lockout saat data
 * IAM belum ter-seed. Akses modul ditentukan grant menu, bukan FULL_ACCESS_ROLES.
 */
export const isEssOnlyUser = cache(
  async (userId: string, role: UserRole): Promise<boolean> => {
    if (!isIamDbConfigured()) return isEssOnlyRole(role);

    try {
      const roleIds = await resolveRoleIds(userId, role);
      if (roleIds.length === 0) return isEssOnlyRole(role);

      const rows = await iamDbQuery<{ has_access: boolean }>(
        `SELECT EXISTS (
           SELECT 1
           FROM iam.role_menu_permissions rmp
           JOIN iam.menus m ON m.id = rmp.menu_id
           WHERE rmp.role_id = ANY($1::uuid[])
             AND rmp.is_active = true
             AND m.deleted_at IS NULL
             AND m.is_active = true
             AND m.code <> 'ess'
             AND m.code NOT LIKE 'ess.%'
         ) AS has_access`,
        [roleIds]
      );

      return !rows[0]?.has_access;
    } catch (error) {
      if (!isIamUnavailable(error)) {
        console.error("[iam] isEssOnlyUser failed:", error);
      }
      return isEssOnlyRole(role);
    }
  }
);

/**
 * Saring pohon nav → hanya item Area Karyawan (ESS, di bawah /dashboard/me).
 * Group (mis. "Area Karyawan") dipertahankan bila punya anak ESS yang lolos.
 * Dipakai untuk role ESS-only agar sidebar bersih dari modul bisnis.
 */
export function filterEssNav(items: NavItem[]): NavItem[] {
  return filterNavByPrefixes(items, ["/dashboard/me"]);
}

/**
 * Generalisasi filterEssNav (EPIC-022): pertahankan item yang href-nya cocok
 * salah satu prefix, plus group yang punya anak lolos. Dipakai role dengan
 * modul tambahan (mis. `sales` → ESS + /dashboard/sales-funnel).
 */
export function filterNavByPrefixes(
  items: NavItem[],
  prefixes: readonly string[]
): NavItem[] {
  const matches = (href: string) =>
    prefixes.some((prefix) => href === prefix || href.startsWith(`${prefix}/`));
  const keep = (item: NavItem): NavItem | null => {
    const children =
      item.children?.map(keep).filter((c): c is NavItem => c !== null) ?? [];
    if (matches(item.href) || children.length > 0) {
      return { ...item, ...(children.length > 0 ? { children } : {}) };
    }
    return null;
  };
  return items.map(keep).filter((i): i is NavItem => i !== null);
}

/**
 * Pohon nav baru tanpa item ber-`href` ini (rekursif), mis. menu Jadwal Toko
 * untuk akun yang bukan pengatur (EPIC-071). Group yang jadi kosong karena
 * pembuangan ikut dibuang; pohon asal tidak diubah.
 */
export function dropNavHref(items: NavItem[], href: string): NavItem[] {
  const result: NavItem[] = [];
  for (const item of items) {
    if (item.href === href) continue;
    if (!item.children?.length) {
      result.push(item);
      continue;
    }
    const children = dropNavHref(item.children, href);
    if (children.length > 0) result.push({ ...item, children });
    else if (isNavigableHref(item.href)) result.push({ href: item.href, label: item.label, icon: item.icon });
  }
  return result;
}

/** Cari NavItem berdasarkan href (rekursif). */
export function findNavItem(items: NavItem[], href: string): NavItem | undefined {
  for (const item of items) {
    if (item.href === href) return item;
    if (item.children) {
      const found = findNavItem(item.children, href);
      if (found) return found;
    }
  }
  return undefined;
}

/** First sidebar route outside a module prefix (e.g. leave POS → purchasing/items). */
export function findFirstBackOfficeHref(
  items: NavItem[],
  excludePathPrefix = "/dashboard/pos"
): string | null {
  for (const item of items) {
    const href = item.href?.trim();
    if (href && !href.startsWith(excludePathPrefix)) {
      return href;
    }
    if (item.children?.length) {
      const childHref = findFirstBackOfficeHref(item.children, excludePathPrefix);
      if (childHref) return childHref;
    }
  }
  return null;
}

/**
 * Menu turunan dari sebuah modul top-level (mis. href "/dashboard/pos"),
 * dipakai oleh top-navbar modul (POS / Purchasing) agar tetap DB-driven.
 */
export const getModuleMenus = cache(
  async (userId: string, role: UserRole, moduleHref: string): Promise<NavItem[]> => {
    const tree = await getUserMenus(userId, role);
    const children = findNavItem(tree, moduleHref)?.children ?? [];
    return flattenModuleNavLinks(children);
  }
);
