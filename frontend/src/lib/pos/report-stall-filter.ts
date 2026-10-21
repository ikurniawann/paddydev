import { query } from "@/lib/db";
import type { UserScope } from "@/lib/api/scope";
import { loadUserWarehouses } from "@/lib/users/user-warehouses";
import { loadCentralCashierGate } from "@/lib/pos/pos-sell-stall-server";
import { firstDayOfMonthWib, todayWib } from "@/lib/pos/report-dates";
import { withBranchLabels } from "@/lib/auth/stall-label";

type RawStall = {
  id: string;
  code: string;
  name: string;
  branch_id: string;
  is_default: boolean;
};

/** Urut per branch + beri branch_name/label dengan aturan yang sama di semua daftar outlet. */
function toStallOptions(
  stalls: RawStall[],
  branchNames: ReadonlyMap<string, string>
): ReportStallOption[] {
  return withBranchLabels(stalls, branchNames).map(
    ({ id, code, name, branch_id, branch_name, label }) => ({
      id,
      code,
      name,
      branch_id,
      branch_name,
      label,
    })
  );
}

async function loadBranchNames(branchIds: string[]): Promise<Map<string, string>> {
  if (branchIds.length === 0) return new Map();
  const rows = await query<{ id: string; name: string }>(
    `SELECT id, name FROM configuration.branches WHERE id = ANY($1::uuid[])`,
    [[...new Set(branchIds)]]
  );
  return new Map(rows.map((row) => [row.id, row.name]));
}

/** Kasir pusat jual lintas stall — laporan tidak boleh terkunci ke 1 assignment. */
export function shouldExpandReportStallsToBranch(input: {
  isUnscoped: boolean;
  canCentralCheckout: boolean;
}) {
  return input.isUnscoped || input.canCentralCheckout;
}

export type ReportStallOption = {
  id: string;
  code: string;
  name: string;
  branch_id: string;
  branch_name: string;
  /** "<nama> — <branch>" bila picker melintasi branch, selain itu = nama. */
  label: string;
};

export type ReportStallFilter = {
  /** null = semua stall yang diizinkan (atau seluruh sistem jika unscoped tanpa filter) */
  warehouseIds: string[] | null;
  stallOptions: ReportStallOption[];
  stallLocked: boolean;
  selectedWarehouseId: string | null;
};

function isUuid(value: string) {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(
    value
  );
}

/**
 * Resolve stall filter for POS sales/transaction reports.
 * - Stall-assigned users: limited to their warehouses; optional one-stall selection
 * - Unscoped / super: can pick any active stall or all
 */
export async function resolveReportStallFilter(
  scope: UserScope | null,
  requestedWarehouseId: string | null | undefined
): Promise<ReportStallFilter> {
  const requested =
    requestedWarehouseId && isUuid(requestedWarehouseId) ? requestedWarehouseId : null;

  const gate = scope
    ? await loadCentralCashierGate({ userId: scope.userId, role: scope.role })
    : null;
  const expandToBranch = shouldExpandReportStallsToBranch({
    isUnscoped: !scope || scope.isUnscoped || scope.role === "super_admin",
    canCentralCheckout: gate?.canCentralCheckout === true,
  });

  // !scope sudah tercakup expandToBranch; ditulis eksplisit supaya scope ter-narrow di bawah.
  if (!scope || expandToBranch) {
    const rawStalls = await query<RawStall>(
      `SELECT id, code, name, branch_id, is_default
       FROM configuration.warehouses
       WHERE is_active = true
         AND ($1::uuid IS NULL OR branch_id = $1)
       ORDER BY name ASC`,
      [scope?.branchId ?? null]
    );
    const stalls = toStallOptions(
      rawStalls,
      await loadBranchNames(rawStalls.map((row) => row.branch_id))
    );

    if (requested) {
      const allowed = stalls.some((row) => row.id === requested);
      if (!allowed) {
        throw new Error("Outlet tidak valid atau di luar scope");
      }
      return {
        warehouseIds: [requested],
        stallOptions: stalls,
        stallLocked: false,
        selectedWarehouseId: requested,
      };
    }

    return {
      warehouseIds: null,
      stallOptions: stalls,
      stallLocked: false,
      selectedWarehouseId: null,
    };
  }

  const assigned = await loadUserWarehouses(scope.userId);
  const stallOptions = toStallOptions(
    assigned.map((row) => ({
      id: row.warehouse_id,
      code: row.code,
      name: row.name,
      branch_id: row.branch_id,
      is_default: row.is_default,
    })),
    await loadBranchNames(assigned.map((row) => row.branch_id))
  );

  if (stallOptions.length === 0) {
    return {
      warehouseIds: [],
      stallOptions: [],
      stallLocked: true,
      selectedWarehouseId: null,
    };
  }

  if (requested) {
    const allowed = stallOptions.some((row) => row.id === requested);
    if (!allowed) {
      throw new Error("Outlet tidak sesuai assignment login");
    }
    return {
      warehouseIds: [requested],
      stallOptions,
      stallLocked: stallOptions.length === 1,
      selectedWarehouseId: requested,
    };
  }

  if (stallOptions.length === 1) {
    return {
      warehouseIds: [stallOptions[0].id],
      stallOptions,
      stallLocked: true,
      selectedWarehouseId: stallOptions[0].id,
    };
  }

  return {
    warehouseIds: stallOptions.map((row) => row.id),
    stallOptions,
    stallLocked: false,
    selectedWarehouseId: null,
  };
}

export function parseReportDateRange(dateFrom?: string | null, dateTo?: string | null) {
  const defaultFrom = firstDayOfMonthWib();
  const defaultTo = todayWib();
  const from = dateFrom && /^\d{4}-\d{2}-\d{2}$/.test(dateFrom) ? dateFrom : defaultFrom;
  const to = dateTo && /^\d{4}-\d{2}-\d{2}$/.test(dateTo) ? dateTo : defaultTo;
  if (from > to) {
    throw new Error("Tanggal dari tidak boleh melebihi tanggal sampai");
  }
  return {
    dateFrom: from,
    dateTo: to,
    // Hari operasional = WIB, bukan UTC: tanpa offset +07:00, order setelah
    // 17:00 WIB tercatat di tanggal laporan BERIKUTNYA dan order dini hari
    // hilang dari tanggalnya sendiri.
    startIso: `${from}T00:00:00.000+07:00`,
    endIso: `${to}T23:59:59.999+07:00`,
  };
}
