import type {
  ProductStockItem,
  RawMaterialStockItem,
  StockListParams,
  StockListResult,
} from "./types";

export type * from "./types";

function buildParams(params: StockListParams) {
  const sp = new URLSearchParams();
  if (params.page) sp.set("page", String(params.page));
  if (params.limit) sp.set("limit", String(params.limit));
  if (params.status && params.status !== "all") sp.set("status", params.status);
  if (params.search) sp.set("search", params.search);
  if (params.warehouse_id && params.warehouse_id !== "all") {
    sp.set("warehouse_id", params.warehouse_id);
  }
  return sp;
}

export async function listRawMaterialStock(
  params: StockListParams = {},
): Promise<StockListResult<RawMaterialStockItem>> {
  const res = await fetch(
    `/api/inventory/raw-materials?${buildParams(params).toString()}`,
  );
  const data = await res.json();
  return {
    items: data.data?.data || data.data || [],
    total: data.pagination?.total || 0,
  };
}

export async function listProductStock(
  params: StockListParams = {},
): Promise<StockListResult<ProductStockItem>> {
  const res = await fetch(
    `/api/inventory/finished-goods?${buildParams(params).toString()}`,
  );
  const data = await res.json();
  return {
    items: data.data?.data || data.data || [],
    total: data.pagination?.total || 0,
  };
}

export async function listStockWarehouses(): Promise<
  { id: string; name: string; code: string; branch_name?: string; label?: string }[]
> {
  const res = await fetch("/api/purchasing/warehouses");
  const json = await res.json();
  if (!res.ok) {
    throw new Error(json.error || json.message || "Gagal memuat data outlet");
  }
  return Array.isArray(json.data) ? json.data : [];
}
