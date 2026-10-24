import { apiDelete, apiGet, apiPatch, apiPost, apiPut, buildListUrl } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type { BranchLocationPayload } from "@/lib/hris/geofence";
import type { PlaceResult } from "@/lib/maps/coords";
import type {
  BulkPlacementPayload,
  ClockPlacements,
  Placement,
  PlacementOverviewFilter,
  PlacementOverviewRow,
  PlacementPayload,
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

export const fetchPlacementOverview = (filter: PlacementOverviewFilter) =>
  apiGet<{ data: PlacementOverviewRow[] }>(
    buildListUrl("/api/hris/employee-placements", {
      branch_id: filter.branch_id,
      unplaced: filter.unplaced ? "true" : undefined,
      q: filter.q,
      include_ended: filter.include_ended ? "true" : undefined,
    })
  ).then((res) => res.data ?? []);

export const createPlacement = (employeeId: string, payload: PlacementPayload) =>
  apiPost<{ data: Placement; message?: string }>(`/api/hris/employees/${employeeId}/placements`, payload);

export const updatePlacement = (id: string, payload: PlacementPayload) =>
  apiPatch<{ data: Placement; message?: string }>(`/api/hris/employee-placements/${id}`, payload);

export const deletePlacement = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/employee-placements/${id}`, reason));

export const bulkPlacement = (payload: BulkPlacementPayload) =>
  apiPost<{ data: { created: number; skipped: number }; message?: string }>(
    "/api/hris/employee-placements/bulk",
    payload
  );

/** Toko yang bisa dipilih saat clock-in; employeeId kosong = karyawan yang login. */
export const fetchClockPlacements = (employeeId?: string) =>
  apiGet<{ data: ClockPlacements }>(
    buildListUrl("/api/hris/me/placements", { employee_id: employeeId })
  ).then((res) => res.data ?? { placements: [], default_branch_id: null });
