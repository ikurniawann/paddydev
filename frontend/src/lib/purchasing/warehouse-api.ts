// Daftar stall/gudang (configuration.warehouses) sesuai scope user untuk
// /api/purchasing/warehouses (picker Items, POS katalog, pengaturan user).
import { z } from "zod";
import {
  resolveWarehouseBranchFilter,
  resolveWarehouseCompanyFilter,
  type UserScope,
} from "@/lib/api/scope";
import { withBranchLabels } from "@/lib/auth/stall-label";
import type { DbClient } from "@/lib/pg/types";

export const warehouseListQuerySchema = z.object({
  branch_id: z.string().uuid("Branch ID harus valid").optional(),
  is_active: z.coerce.boolean().optional().default(true),
});

export async function listWarehouses(
  db: DbClient,
  scope: UserScope | null,
  params: z.infer<typeof warehouseListQuerySchema>
): Promise<unknown[]> {
  const branchFilter = resolveWarehouseBranchFilter(scope, params.branch_id);
  const companyFilter = resolveWarehouseCompanyFilter(scope);

  let query = db.from("warehouses", "configuration").select("*").eq("is_active", params.is_active);

  if (branchFilter) {
    query = query.eq("branch_id", branchFilter);
  } else if (companyFilter) {
    // User bercope company: hanya gudang milik cabang company-nya.
    const { data: branches, error: branchError } = await db
      .from("branches", "configuration")
      .select("id")
      .eq("company_id", companyFilter);
    if (branchError) throw branchError;
    const branchIds = ((branches ?? []) as Array<{ id: string }>).map((branch) => branch.id);
    if (branchIds.length === 0) return [];
    query = query.in("branch_id", branchIds);
  }

  const { data, error } = await query.order("name", { ascending: true });
  if (error) throw error;
  const warehouses = (data ?? []) as Array<{
    name: string;
    branch_id: string;
    is_default?: boolean | null;
  }>;
  if (warehouses.length === 0) return [];

  // Tiap branch punya "Main Storage" sendiri: sertakan nama branch + label
  // ("<nama> — <branch>" bila daftar melintasi branch) agar baris bisa dibedakan.
  const { data: branchRows, error: namesError } = await db
    .from("branches", "configuration")
    .select("id, name")
    .in("id", [...new Set(warehouses.map((warehouse) => warehouse.branch_id))]);
  if (namesError) throw namesError;
  const branchNames = new Map(
    ((branchRows ?? []) as Array<{ id: string; name: string }>).map((branch) => [branch.id, branch.name])
  );
  return withBranchLabels(warehouses, branchNames);
}
