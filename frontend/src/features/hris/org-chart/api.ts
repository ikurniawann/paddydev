import { apiGet, apiPost, buildListUrl } from "@/lib/api-client";
import { fetchAllEmployees } from "@/lib/hris/fetch-all-employees";
import type { OrgEmployee } from "@/lib/hris/org-chart";
import type {
  ChartGapResponse,
  ChartTreeFilter,
  ChartTreeResponse,
  ChartVacancyFilter,
  ChartVacancyResponse,
  ChartValidation,
  ChartVersion,
  OrgDepartment,
} from "./types";

export const fetchOrgEmployees = () => fetchAllEmployees<OrgEmployee>("is_active=true");

export const fetchOrgDepartments = () =>
  apiGet<{ data: (OrgDepartment & { is_active: boolean })[] }>("/api/master/departments").then((res) =>
    (res.data ?? []).filter((d) => d.is_active)
  );

/* ── Bagan organisasi (EPIC-101/102) ── */
/* version "" = versi bawaan server: yang disetujui, bila belum ada impor terbaru. */

const v = (version: string) => version || undefined;

/** Semua versi, impor terbaru lebih dulu. */
export const fetchOrgChartVersions = () =>
  apiGet<{ data: ChartVersion[] }>("/api/hris/org-chart/versions").then((res) => res.data ?? []);

/** Pohon bagan; filter menyaring node (kosong = semua). */
export const fetchOrgChartTree = (filter: ChartTreeFilter, version: string) =>
  apiGet<{ data: ChartTreeResponse }>(
    buildListUrl("/api/hris/org-chart/tree", {
      version: v(version),
      division: filter.division || undefined,
      region: filter.region || undefined,
    })
  ).then((res) => res.data);

export const fetchOrgChartValidation = (version: string) =>
  apiGet<{ data: ChartValidation }>(buildListUrl("/api/hris/org-chart/validation", { version: v(version) })).then(
    (res) => res.data
  );

export const fetchOrgChartGap = (version: string) =>
  apiGet<{ data: ChartGapResponse }>(buildListUrl("/api/hris/org-chart/gap", { version: v(version) })).then(
    (res) => res.data
  );

/** Laporan selisih bagan vs data karyawan untuk HR. */
export const orgChartGapXlsxUrl = (version: string) =>
  buildListUrl("/api/hris/org-chart/gap", { version: v(version), format: "xlsx" });

const vacancyParams = (version: string, filter: ChartVacancyFilter) => ({
  version: v(version),
  division: filter.division || undefined,
  region: filter.region || undefined,
  state: filter.state || undefined,
});

/** Posisi Kosong: keadaan setiap slot, ringkasan per divisi/region. */
export const fetchOrgChartVacancy = (version: string, filter: ChartVacancyFilter) =>
  apiGet<{ data: ChartVacancyResponse }>(buildListUrl("/api/hris/org-chart/vacancy", vacancyParams(version, filter))).then(
    (res) => res.data
  );

export const orgChartVacancyXlsxUrl = (version: string, filter: ChartVacancyFilter) =>
  buildListUrl("/api/hris/org-chart/vacancy", { ...vacancyParams(version, filter), format: "xlsx" });

/** Setujui satu versi draft; versi yang disetujui sebelumnya diarsipkan server. */
export const approveOrgChartVersion = (id: string, body: { valid_from: string; reason?: string }) =>
  apiPost<{ data: ChartVersion }>(`/api/hris/org-chart/versions/${encodeURIComponent(id)}/approve`, body).then(
    (res) => res.data
  );
