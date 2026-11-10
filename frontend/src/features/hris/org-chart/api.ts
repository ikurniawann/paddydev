import { apiGet, buildListUrl } from "@/lib/api-client";
import { fetchAllEmployees } from "@/lib/hris/fetch-all-employees";
import type { OrgEmployee } from "@/lib/hris/org-chart";
import type { ChartGapResponse, ChartTreeFilter, ChartTreeResponse, ChartValidation, OrgDepartment } from "./types";

export const fetchOrgEmployees = () => fetchAllEmployees<OrgEmployee>("is_active=true");

export const fetchOrgDepartments = () =>
  apiGet<{ data: (OrgDepartment & { is_active: boolean })[] }>("/api/master/departments").then((res) =>
    (res.data ?? []).filter((d) => d.is_active)
  );

/* ── Bagan Draft 6 (EPIC-101) ── */

/** Pohon bagan versi terbaru; filter menyaring node (kosong = semua). */
export const fetchOrgChartTree = (filter: ChartTreeFilter) =>
  apiGet<{ data: ChartTreeResponse }>(
    buildListUrl("/api/hris/org-chart/tree", { division: filter.division || undefined, region: filter.region || undefined })
  ).then((res) => res.data);

export const fetchOrgChartValidation = () =>
  apiGet<{ data: ChartValidation }>("/api/hris/org-chart/validation").then((res) => res.data);

export const fetchOrgChartGap = () =>
  apiGet<{ data: ChartGapResponse }>("/api/hris/org-chart/gap").then((res) => res.data);

/** Laporan selisih bagan vs data karyawan untuk HR. */
export const orgChartGapXlsxUrl = "/api/hris/org-chart/gap?format=xlsx";
