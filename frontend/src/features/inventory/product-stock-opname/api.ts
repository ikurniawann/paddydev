import type {
  CreateProductStockOpnameInput,
  ProductStockOpnameDetail,
  ProductStockOpnameListParams,
  ProductStockOpnamePreviewLine,
  UpdateProductStockOpnameInput,
} from "./types";

export interface ProductStockOpnameListResult {
  data: ProductStockOpnameDetail[];
  pagination: {
    page: number;
    limit: number;
    total: number;
    total_pages: number;
  };
}

function buildParams(params: ProductStockOpnameListParams) {
  const sp = new URLSearchParams();
  if (params.page) sp.set("page", String(params.page));
  if (params.limit) sp.set("limit", String(params.limit));
  if (params.status && params.status !== "all") sp.set("status", params.status);
  if (params.search) sp.set("search", params.search);
  if (params.reason) sp.set("reason", params.reason);
  if (params.warehouse_id) sp.set("warehouse_id", params.warehouse_id);
  return sp;
}

async function parseJson<T>(res: Response): Promise<T> {
  const json = await res.json();
  if (!res.ok) {
    throw new Error(json.message || json.error || "Permintaan gagal");
  }
  return json;
}

export async function listProductStockOpnames(
  params: ProductStockOpnameListParams = {},
): Promise<ProductStockOpnameListResult> {
  const res = await fetch(
    `/api/inventory/product-stock-opnames?${buildParams(params).toString()}`,
  );
  const json = await parseJson<{
    success: boolean;
    data: ProductStockOpnameDetail[];
    pagination: ProductStockOpnameListResult["pagination"];
  }>(res);
  return {
    data: json.data || [],
    pagination: json.pagination,
  };
}

export async function getProductStockOpname(
  id: string,
): Promise<ProductStockOpnameDetail> {
  const res = await fetch(`/api/inventory/product-stock-opnames/${id}`);
  const json = await parseJson<{
    success: boolean;
    data: ProductStockOpnameDetail;
  }>(res);
  return json.data;
}

export async function createProductStockOpname(
  input: CreateProductStockOpnameInput,
): Promise<ProductStockOpnameDetail> {
  const res = await fetch("/api/inventory/product-stock-opnames", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  const json = await parseJson<{
    success: boolean;
    data: ProductStockOpnameDetail;
    message?: string;
  }>(res);
  return json.data;
}

export async function updateProductStockOpname(
  id: string,
  input: UpdateProductStockOpnameInput,
): Promise<ProductStockOpnameDetail> {
  const res = await fetch(`/api/inventory/product-stock-opnames/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  const json = await parseJson<{
    success: boolean;
    data: ProductStockOpnameDetail;
    message?: string;
  }>(res);
  return json.data;
}

export async function completeProductStockOpname(
  id: string,
): Promise<ProductStockOpnameDetail> {
  const res = await fetch(
    `/api/inventory/product-stock-opnames/${id}/complete`,
    {
      method: "POST",
    },
  );
  const json = await parseJson<{
    success: boolean;
    data: ProductStockOpnameDetail;
    message?: string;
  }>(res);
  return json.data;
}

export async function listProductStockOpnamePreview(
  warehouseId: string,
): Promise<ProductStockOpnamePreviewLine[]> {
  const sp = new URLSearchParams({ warehouse_id: warehouseId });
  const res = await fetch(
    `/api/inventory/product-stock-opnames/preview?${sp.toString()}`,
  );
  const json = await parseJson<{
    success: boolean;
    data: ProductStockOpnamePreviewLine[];
  }>(res);
  return json.data || [];
}

export async function listProductStockOpnameWarehouses(): Promise<
  { id: string; name: string; code: string; branch_name?: string; label?: string }[]
> {
  const res = await fetch("/api/purchasing/warehouses");
  const json = await res.json();
  if (!res.ok) {
    throw new Error(json.message || "Failed to load outlets");
  }
  return Array.isArray(json.data) ? json.data : [];
}
