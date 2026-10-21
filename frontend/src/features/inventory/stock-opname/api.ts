import type {
  CreateStockOpnameInput,
  StockOpnameDetail,
  StockOpnameListParams,
  StockOpnamePreviewLine,
  UpdateStockOpnameInput,
} from "./types";

export interface StockOpnameListResult {
  data: StockOpnameDetail[];
  pagination: {
    page: number;
    limit: number;
    total: number;
    total_pages: number;
  };
}

function buildParams(params: StockOpnameListParams) {
  const sp = new URLSearchParams();
  if (params.page) sp.set("page", String(params.page));
  if (params.limit) sp.set("limit", String(params.limit));
  if (params.status && params.status !== "all") sp.set("status", params.status);
  if (params.warehouse_id) sp.set("warehouse_id", params.warehouse_id);
  if (params.search) sp.set("search", params.search);
  if (params.reason) sp.set("reason", params.reason);
  return sp;
}

async function parseJson<T>(res: Response): Promise<T> {
  const json = await res.json();
  if (!res.ok) {
    throw new Error(json.message || json.error || "Permintaan gagal");
  }
  return json;
}

export async function listStockOpnames(
  params: StockOpnameListParams = {},
): Promise<StockOpnameListResult> {
  const res = await fetch(
    `/api/inventory/stock-opnames?${buildParams(params).toString()}`,
  );
  const json = await parseJson<{
    success: boolean;
    data: StockOpnameDetail[];
    pagination: StockOpnameListResult["pagination"];
  }>(res);
  return {
    data: json.data || [],
    pagination: json.pagination,
  };
}

export async function getStockOpname(id: string): Promise<StockOpnameDetail> {
  const res = await fetch(`/api/inventory/stock-opnames/${id}`);
  const json = await parseJson<{ success: boolean; data: StockOpnameDetail }>(
    res,
  );
  return json.data;
}

export async function createStockOpname(
  input: CreateStockOpnameInput,
): Promise<StockOpnameDetail> {
  const res = await fetch("/api/inventory/stock-opnames", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  const json = await parseJson<{
    success: boolean;
    data: StockOpnameDetail;
    message?: string;
  }>(res);
  return json.data;
}

export async function updateStockOpname(
  id: string,
  input: UpdateStockOpnameInput,
): Promise<StockOpnameDetail> {
  const res = await fetch(`/api/inventory/stock-opnames/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  const json = await parseJson<{
    success: boolean;
    data: StockOpnameDetail;
    message?: string;
  }>(res);
  return json.data;
}

export async function completeStockOpname(
  id: string,
): Promise<StockOpnameDetail> {
  const res = await fetch(`/api/inventory/stock-opnames/${id}/complete`, {
    method: "POST",
  });
  const json = await parseJson<{
    success: boolean;
    data: StockOpnameDetail;
    message?: string;
  }>(res);
  return json.data;
}

export async function listStockOpnamePreview(
  warehouseId: string,
): Promise<StockOpnamePreviewLine[]> {
  const res = await fetch(
    `/api/inventory/stock-opnames/preview?warehouse_id=${encodeURIComponent(warehouseId)}`,
  );
  const json = await parseJson<{
    success: boolean;
    data: StockOpnamePreviewLine[];
  }>(res);
  return json.data || [];
}

export async function listStockOpnameWarehouses(): Promise<
  { id: string; name: string; code: string; branch_name?: string; label?: string }[]
> {
  const res = await fetch("/api/purchasing/warehouses");
  const json = await res.json();
  if (!res.ok) {
    throw new Error(json.message || "Failed to load warehouse data");
  }
  return Array.isArray(json.data) ? json.data : [];
}
