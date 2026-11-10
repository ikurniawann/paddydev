import { apiDelete, apiGet, apiPatch, apiPost, apiPut, buildListUrl } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type { BranchLocationPayload } from "@/lib/hris/geofence";
import type { PlaceResult } from "@/lib/maps/coords";
import { downloadBlob } from "@/lib/purchasing/report-ui-shared";
import type {
  BranchCodeImportPreview,
  BulkEndPlacementPayload,
  BulkPlacementPayload,
  BulkPlacementResponse,
  ClockPlacements,
  Placement,
  PlacementIdsResult,
  PlacementPayload,
  PlacementSearchFilter,
  PlacementSearchResult,
  SavedBranchLocation,
  SavedBranchMinWorkHours,
  StoreBranch,
} from "./types";

export const fetchStoreBranches = () =>
  apiGet<{ data: StoreBranch[] }>("/api/hris/branches").then((res) => res.data ?? []);

/** Cari nama tempat/alamat untuk peta Lokasi Toko (OSM Nominatim lewat server). */
export const searchPlaces = (q: string) =>
  apiGet<{ data: PlaceResult[] }>(`/api/hris/places?q=${encodeURIComponent(q)}`).then((res) => res.data ?? []);

/** PUT penuh: kirim selalu ketiga kunci (kunci yang hilang disimpan sebagai null). */
export const saveBranchLocation = (id: string, payload: BranchLocationPayload) =>
  apiPut<{ data: SavedBranchLocation }>(`/api/hris/branches/${id}/location`, payload);

/** PUT penuh: null = tanpa aturan jam minimum (EPIC-065 AB-05). */
export const saveBranchMinWorkHours = (id: string, payload: { min_work_hours: number | null }) =>
  apiPut<{ data: SavedBranchMinWorkHours; message?: string }>(`/api/hris/branches/${id}/min-work-hours`, payload);

export const fetchEmployeePlacements = (employeeId: string) =>
  apiGet<{ data: Placement[] }>(`/api/hris/employees/${employeeId}/placements`).then((res) => res.data ?? []);

export const createPlacement = (employeeId: string, payload: PlacementPayload) =>
  apiPost<{ data: Placement; message?: string }>(`/api/hris/employees/${employeeId}/placements`, payload);

export const updatePlacement = (id: string, payload: PlacementPayload) =>
  apiPatch<{ data: Placement; message?: string }>(`/api/hris/employee-placements/${id}`, payload);

export const deletePlacement = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/employee-placements/${id}`, reason));

/** Parameter filter bersama search & ids (kosong dibuang buildListUrl). */
const searchParams = (f: PlacementSearchFilter) => ({
  q: f.q?.trim(),
  branch_id: f.branch_id,
  department_id: f.department_id,
  position_id: f.position_id,
  status: f.status,
  include_ended: f.include_ended ? "true" : undefined,
});

/** Halaman Penempatan Karyawan: hasil berhalaman + jumlah per chip (EPIC-070). */
export const searchPlacements = (filter: PlacementSearchFilter) =>
  apiGet<{ data: PlacementSearchResult }>(
    buildListUrl("/api/hris/employee-placements/search", {
      ...searchParams(filter),
      page: filter.page,
      page_size: filter.page_size,
    })
  ).then((res) => res.data);

/** "Pilih semua hasil filter": semua id karyawan yang cocok (maks 2000). */
export const fetchPlacementIds = (filter: PlacementSearchFilter) =>
  apiGet<{ data: PlacementIdsResult }>(buildListUrl("/api/hris/employee-placements/ids", searchParams(filter))).then(
    (res) => res.data
  );

/** Tambah/pindah massal; kirim step "commit" untuk menyimpan (tanpa itu hanya pratinjau). */
export const bulkPlacement = (payload: BulkPlacementPayload) =>
  apiPost<BulkPlacementResponse>("/api/hris/employee-placements/bulk", payload);

/** Akhiri penempatan berjalan secara massal (pratinjau/simpan). */
export const bulkEndPlacement = (payload: BulkEndPlacementPayload) =>
  apiPost<BulkPlacementResponse>("/api/hris/employee-placements/bulk-end", payload);

/** Toko yang bisa dipilih saat clock-in; employeeId kosong = karyawan yang login. */
export const fetchClockPlacements = (employeeId?: string) =>
  apiGet<{ data: ClockPlacements }>(
    buildListUrl("/api/hris/me/placements", { employee_id: employeeId })
  ).then((res) => res.data ?? { placements: [], default_branch_id: null });

/* ── Kode & Status Toko (EPIC-100) ── */

/** Template berisi semua toko dengan kode, nama, dan status saat ini. */
export const branchCodeTemplateUrl = "/api/hris/branches/template";

/** Galat impor dengan status HTTP-nya (409 = data toko berubah sejak pratinjau). */
export class BranchCodeImportError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "BranchCodeImportError";
    this.status = status;
  }
}

type BranchCodeImportInput = {
  file: File;
  step: "preview" | "commit" | "error_file";
  fingerprint?: string;
  ack_warnings?: boolean;
};

function postBranchCodes(input: BranchCodeImportInput) {
  const form = new FormData();
  form.append("file", input.file);
  form.append("step", input.step);
  if (input.fingerprint) form.append("fingerprint", input.fingerprint);
  if (input.ack_warnings) form.append("ack_warnings", "true");
  return fetch("/api/hris/branches/import", { method: "POST", body: form });
}

/** step=preview mengembalikan pratinjau; step=commit mengembalikan ringkasan + pesan server. */
export async function importBranchCodeFile(
  input: Omit<BranchCodeImportInput, "step"> & { step: "preview" | "commit" }
): Promise<{ data: BranchCodeImportPreview; message?: string }> {
  const res = await postBranchCodes(input);
  const json = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new BranchCodeImportError((json as { error?: string }).error ?? `Upload gagal (${res.status})`, res.status);
  }
  return json as { data: BranchCodeImportPreview; message?: string };
}

/** File yang sama dengan kolom "Keterangan Error". */
export async function downloadBranchCodeErrors(file: File): Promise<void> {
  const res = await postBranchCodes({ file, step: "error_file" });
  if (!res.ok) throw new Error(`Gagal membuat file error (${res.status})`);
  downloadBlob(await res.blob(), `Error - ${file.name}`);
}
