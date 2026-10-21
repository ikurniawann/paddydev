import { query, queryOne } from "@/lib/db";
import { compareWarehouses, sortWarehouses } from "@/lib/configuration/sort-warehouses";
import { computeStallAllAccess } from "@/lib/users/stall-assignment";

export interface StallOption {
  id: string;
  name: string;
  code: string;
  is_default: boolean;
  branch_id: string;
  /** Nama branch pemilik outlet ("" bila tidak ada). */
  branch_name: string;
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

/** Urut per nama branch, lalu Main Storage dulu dan urutan stall biasa. */
export function sortStallsByBranch<T extends StallOption>(stalls: T[]): T[] {
  return [...stalls].sort(
    (a, b) =>
      collator.compare(a.branch_name, b.branch_name) ||
      (a.branch_id < b.branch_id ? -1 : a.branch_id > b.branch_id ? 1 : 0) ||
      compareWarehouses(a, b)
  );
}

const STALL_COLUMNS = `w.id, w.name, w.code, w.is_default, w.branch_id, COALESCE(b.name, '') AS branch_name`;

/** Satu outlet aktif lengkap dengan branch-nya (untuk label stall aktif). */
export async function findStallOption(id: string): Promise<StallOption | null> {
  return queryOne<StallOption>(
    `SELECT ${STALL_COLUMNS}
     FROM configuration.warehouses w
     LEFT JOIN configuration.branches b ON b.id = w.branch_id
     WHERE w.id = $1 AND w.is_active`,
    [id]
  );
}

export interface StallAccess {
  /** true = boleh switch ke stall mana pun (termasuk "Semua Stall"). */
  allAccess: boolean;
  /** Stall yang boleh dipilih di switcher (sudah difilter & terurut). */
  stalls: StallOption[];
}

/**
 * Aturan switcher stall:
 * - super_admin / can_switch_stall / Main Storage: semua stall di branch.
 * - Selain itu: hanya stall penempatan (default).
 */
export async function getStallAccess(
  userId: string,
  role: string | null,
  branchId: string | null
): Promise<StallAccess> {
  const [assignments, flags] = await Promise.all([
    query<StallOption>(
      `SELECT ${STALL_COLUMNS}
       FROM configuration.user_warehouses uw
       JOIN configuration.warehouses w ON w.id = uw.warehouse_id AND w.is_active
       LEFT JOIN configuration.branches b ON b.id = w.branch_id
       WHERE uw.user_id = $1 AND uw.is_active`,
      [userId]
    ),
    queryOne<{ can_switch_stall: boolean }>(
      `SELECT COALESCE(can_switch_stall, false) AS can_switch_stall
       FROM configuration.users
       WHERE id = $1`,
      [userId]
    ),
  ]);

  const allAccess = computeStallAllAccess({
    role,
    canSwitchStall: flags?.can_switch_stall === true,
    assignedMainStorage: assignments.some((assignment) => assignment.is_default),
  });

  if (!allAccess) {
    return { allAccess, stalls: sortWarehouses(assignments) };
  }

  const params: unknown[] = [];
  let where = "w.is_active";
  if (branchId) {
    params.push(branchId);
    where += ` AND w.branch_id = $${params.length}`;
  }

  const stalls = await query<StallOption>(
    `SELECT ${STALL_COLUMNS}
     FROM configuration.warehouses w
     LEFT JOIN configuration.branches b ON b.id = w.branch_id
     WHERE ${where}`,
    params
  );

  return { allAccess, stalls: sortWarehouses(stalls) };
}
