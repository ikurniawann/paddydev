import { apiGet, apiPost, apiPut } from "@/lib/api-client";
import type { PosReceiptSettings } from "@/lib/pos/receipt-settings";
import type {
  BusinessTree,
  CreateBusinessPayload,
  UpdateBusinessPayload,
  BusinessEntityType,
} from "./types";

const BASE = "/api/settings/business";

export async function fetchBusinessTree(): Promise<BusinessTree> {
  const res = await apiGet<{ data: BusinessTree }>(BASE);
  return res.data;
}

export async function createBusinessEntity(payload: CreateBusinessPayload) {
  return apiPost<{
    data: { id: string; type: BusinessEntityType };
    tree: BusinessTree;
  }>(BASE, payload);
}

export async function updateBusinessEntity(
  type: BusinessEntityType,
  id: string,
  payload: UpdateBusinessPayload,
) {
  const response = await fetch(`${BASE}/${type}/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  const json = await response.json();
  if (!response.ok) {
    throw new Error(json.error || "Gagal memperbarui data");
  }
  return json as { data: { id: string }; tree: BusinessTree };
}

export async function deleteBusinessEntity(
  type: BusinessEntityType,
  id: string,
) {
  const response = await fetch(`${BASE}/${type}/${id}`, { method: "DELETE" });
  const json = await response.json();
  if (!response.ok) {
    throw new Error(json.error || "Gagal menghapus data");
  }
  return json as { data: { id: string }; tree: BusinessTree };
}

export type ReceiptStallOption = {
  id: string;
  name: string;
  branch_id: string | null;
  branch_name?: string;
  /** "<nama> — <branch>" bila daftar melintasi branch (tampilkan lewat stallLabel). */
  label?: string;
};
export type ReceiptSettingsData = {
  data: PosReceiptSettings[];
  stalls: ReceiptStallOption[];
};

export const fetchReceiptSettings = () =>
  apiGet<ReceiptSettingsData>("/api/settings/receipt");

export const saveReceiptSettings = (body: {
  warehouse_id: string | null;
  branch_id: string | null;
  header_lines: string[];
  footer_lines: string[];
  show_stall_name: boolean;
}) => apiPut<{ data: PosReceiptSettings[] }>("/api/settings/receipt", body);

export interface CompanyProfile {
  legal_name: string | null;
  address: string | null;
  city: string | null;
  signer_name: string | null;
  signer_title: string | null;
}

export const fetchCompanyProfile = () =>
  apiGet<{ data: CompanyProfile }>("/api/settings/company-profile").then(
    (r) => r.data,
  );

export const saveCompanyProfile = (
  body: Partial<Record<keyof CompanyProfile, string>>,
) => apiPut("/api/settings/company-profile", body);

/** Public profile of a branch (configuration.branches public columns). */
export interface BranchProfile {
  id: string;
  name: string;
  slug: string;
  is_public: boolean;
  address: string | null;
  city: string | null;
  postcode: string | null;
  lat: number | null;
  lng: number | null;
  phone: string | null;
  email: string | null;
  instagram: string | null;
  directions: string | null;
  hero_image_url: string | null;
  benefits: { title: string; text: string }[];
  accordions: { facilities: string[]; parking: string[]; team: string[]; community: string[] };
  extras: { name: string; blurb: string }[];
  testimonials: { name: string; quote: string; role: string }[];
}

export type BranchProfileBody = Omit<BranchProfile, "id" | "name">;

export const fetchBranchProfile = (branchId: string) =>
  apiGet<{ data: BranchProfile }>(`${BASE}/branch/${branchId}/profile`).then((r) => r.data);

export const saveBranchProfile = (branchId: string, body: BranchProfileBody) =>
  apiPut<{ data: BranchProfile }>(`${BASE}/branch/${branchId}/profile`, body).then((r) => r.data);
