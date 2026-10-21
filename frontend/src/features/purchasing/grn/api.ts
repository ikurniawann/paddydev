import {
  mapGrnDelivery,
  mapPoLine,
  type buildContinueGrnPayload,
  type buildCreateGrnPayload,
  type GrnDeliveryApiRow,
  type GrnDeliveryOption,
  type PoLine,
  type PoLineApiRow,
  type ReceivingModuleType,
  type ReceivingUserScope,
} from "@/lib/purchasing/grn-ui-lines";
import { apiErrorMessage } from "@/lib/purchasing/receiving-ui-http";
import type { ReceivingWorkspaceData } from "@/lib/purchasing/receiving-ui-workspace";
import type { GrnListParams, GrnListResult, VendorCreditRow } from "./types";

export type PurchasingModuleType = ReceivingModuleType;
export type CreateGrnPayload = ReturnType<typeof buildCreateGrnPayload>;
export type UpdateGrnPayload = NonNullable<ReturnType<typeof buildContinueGrnPayload>>;

type ApiBody = { success?: boolean; data?: unknown; pagination?: { total?: number } } | null;

/** fetch + JSON; galat (HTTP atau `success: false`) dilempar dengan pesan dari server. */
async function request(url: string, fallbackError: string, init?: RequestInit): Promise<ApiBody> {
  const res = await fetch(url, init);
  const json: ApiBody = await res.json().catch(() => null);
  if (!res.ok || json?.success === false) {
    throw new Error(apiErrorMessage(json, fallbackError));
  }
  return json;
}

const jsonInit = (method: string, body: unknown): RequestInit => ({
  method,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

const moduleQuery = (moduleType?: PurchasingModuleType) =>
  moduleType === "product" ? "?module_type=product" : "";

const asArray = <T>(value: unknown): T[] => (Array.isArray(value) ? (value as T[]) : []);

export async function listGrns(params: GrnListParams = {}): Promise<GrnListResult> {
  const sp = new URLSearchParams();
  if (params.page) sp.set("page", String(params.page));
  if (params.limit) sp.set("limit", String(params.limit));
  if (params.search) sp.set("search", params.search);

  const json = await request(`/api/purchasing/grn?${sp.toString()}`, "Gagal memuat data GRN");
  return { data: asArray(json?.data), total: json?.pagination?.total || 0 };
}

export async function getGrn<T>(id: string): Promise<T> {
  const json = await request(`/api/purchasing/grn/${id}`, "Data GRN tidak ditemukan");
  return (json?.data ?? json) as T;
}

/** QC GRN; null bila belum ada inspeksi (endpoint mengembalikan 404). */
export async function getGrnQC<T>(id: string): Promise<T | null> {
  const res = await fetch(`/api/purchasing/grn/${id}/qc`);
  const json: ApiBody = await res.json().catch(() => null);
  if (!res.ok || json?.success === false) return null;
  return (json?.data ?? null) as T | null;
}

export async function getGrnVendorCredits(grnId: string): Promise<VendorCreditRow[]> {
  const json = await request(`/api/purchasing/grn/${grnId}/vendor-credits`, "Gagal memuat vendor credit");
  return asArray(json?.data);
}

export async function deleteGrn(id: string): Promise<{ message?: string }> {
  const json = await request(`/api/purchasing/grn/${id}`, "Gagal menghapus GRN", { method: "DELETE" });
  return (json ?? {}) as { message?: string };
}

export async function updateGrn(id: string, payload: UpdateGrnPayload): Promise<unknown> {
  return request(`/api/purchasing/grn/${id}`, "Gagal mengupdate GRN", jsonInit("PATCH", payload));
}

export async function createGrn(payload: CreateGrnPayload): Promise<{ data?: { id?: string } }> {
  const json = await request("/api/purchasing/grn", "Gagal membuat penerimaan barang", jsonInit("POST", payload));
  return (json ?? {}) as { data?: { id?: string } };
}

async function getPoDetail<T>(poId: string): Promise<T | null> {
  const res = await fetch(`/api/purchasing/po/${poId}`);
  const json: ApiBody = await res.json().catch(() => null);
  return (json?.data ?? null) as T | null;
}

/** Item PO untuk form GRN; jatuh ke detail PO bila endpoint item kosong. */
export async function getGrnPoLines(
  poId: string,
  moduleType: PurchasingModuleType = "raw_material"
): Promise<PoLine[]> {
  const res = await fetch(`/api/purchasing/po/${poId}/items`);
  const json: ApiBody = await res.json().catch(() => null);
  let rows = asArray<PoLineApiRow>(json?.data);
  if (rows.length === 0) {
    rows = (await getPoDetail<{ items?: PoLineApiRow[] }>(poId))?.items ?? [];
  }
  return rows.map((row) => mapPoLine(row, moduleType));
}

export async function getGrnPoBranchId(poId: string): Promise<string | null> {
  const po = await getPoDetail<{ branch_id?: string | null }>(poId);
  return po?.branch_id ?? null;
}

export async function listGrnDeliveries(moduleType?: PurchasingModuleType): Promise<GrnDeliveryOption[]> {
  const json = await request(
    `/api/purchasing/delivery/for-grn${moduleQuery(moduleType)}`,
    "Gagal memuat data pengiriman",
    { cache: "no-store" }
  );
  return asArray<GrnDeliveryApiRow>(json?.data).map(mapGrnDelivery);
}

export type Warehouse = { id: string; name: string; code: string; branch_name?: string; label?: string };

export async function listWarehouses(branchId?: string | null): Promise<Warehouse[]> {
  const query = branchId ? `?branch_id=${encodeURIComponent(branchId)}` : "";
  const json = await request(`/api/purchasing/warehouses${query}`, "Gagal memuat data gudang");
  return asArray(json?.data);
}

export async function getReceivingUserScope(): Promise<ReceivingUserScope> {
  const res = await fetch("/api/auth/scope", { cache: "no-store" });
  const json = await res.json().catch(() => null);
  if (!res.ok || !json?.success) {
    throw new Error(apiErrorMessage(json, "Gagal memuat scope pengguna"));
  }
  return json.data as ReceivingUserScope;
}

export async function getReceivingWorkspace(
  moduleType?: PurchasingModuleType
): Promise<ReceivingWorkspaceData> {
  const json = await request(
    `/api/purchasing/receiving-workspace${moduleQuery(moduleType)}`,
    "Gagal memuat workspace penerimaan",
    { cache: "no-store" }
  );
  const data = (json?.data ?? {}) as Partial<Record<keyof ReceivingWorkspaceData, unknown>>;
  return {
    purchase_orders: asArray(data.purchase_orders),
    deliveries: asArray(data.deliveries),
    grns: asArray(data.grns),
  };
}

export type SubmitGrnQcPayload = {
  status?: "approved" | "rejected" | "partial";
  parameter_inspeksi?: Record<string, unknown>;
  hasil_inspeksi?: Record<string, string>;
  catatan?: string | null;
  rekomendasi?: string | null;
  items: {
    grn_item_id: string;
    raw_material_id?: string | null;
    product_id?: string | null;
    qty_inspected: number;
    qty_accepted: number;
    qty_rejected: number;
    catatan?: string | null;
  }[];
};

export async function createQCInspection(grnId: string, payload: SubmitGrnQcPayload): Promise<unknown> {
  const json = await request(
    `/api/purchasing/grn/${grnId}/qc`,
    "Gagal mengirim hasil QC",
    jsonInit("POST", payload)
  );
  return json?.data ?? json;
}
