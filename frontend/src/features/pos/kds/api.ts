import type { KDSOrder, KdsListParams, KdsStallOption } from "@/lib/pos/kds-types";

export type * from "@/lib/pos/kds-types";

export async function listKdsOrders(params: KdsListParams = {}): Promise<KDSOrder[]> {
  const sp = new URLSearchParams();
  sp.set("status", (params.status ?? ["pending", "confirmed", "preparing", "ready"]).join(","));
  if (params.station) sp.set("station", params.station);
  if (params.branchId) sp.set("branch_id", params.branchId);
  if (params.warehouseId) sp.set("warehouse_id", params.warehouseId);
  if (params.dateFrom) sp.set("date_from", params.dateFrom);
  if (params.dateTo) sp.set("date_to", params.dateTo);
  sp.set("limit", String(params.limit ?? 50));

  const res = await fetch(`/api/pos/kds?${sp.toString()}`, { cache: "no-store" });
  const data = await res.json();
  if (!data.success) {
    throw new Error(data.error || "Gagal fetch KDS");
  }
  return (data.data ?? []) as KDSOrder[];
}

export async function listKdsStalls(): Promise<KdsStallOption[]> {
  const res = await fetch("/api/auth/stall-options", { cache: "no-store" });
  const data = await res.json().catch(() => ({}));
  if (!res.ok || !data.success) return [];
  return ((data.data?.stalls ?? []) as KdsStallOption[]).map(({ id, name, code, label }) => ({
    id,
    name,
    code,
    label,
  }));
}

export async function updateKdsOrderStatus(
  orderId: string,
  status: string,
  reason?: string,
  station?: string,
  itemIds?: string[]
): Promise<{ success: boolean; error?: string }> {
  const res = await fetch(`/api/pos/orders/${orderId}/status`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      status,
      reason,
      station,
      ...(itemIds && itemIds.length > 0 ? { item_ids: itemIds } : {}),
    }),
  });
  return res.json();
}
