import { apiDelete, apiGet, apiPost, buildListUrl } from "@/lib/api-client";
import { downloadBlob } from "@/lib/purchasing/report-ui-shared";
import { fetchAllEmployees } from "@/lib/hris/fetch-all-employees";
import type { OrgEmployee } from "@/lib/hris/org-chart";
import type {
  ChartGapResponse,
  ChartImportInput,
  ChartImportPreview,
  ChartAccess,
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

/* ── Impor versi bagan (EPIC-103) ── */

/** Template Excel (sheet Bagan) dari versi yang sedang dilihat. */
export const orgChartTemplateUrl = (version: string) => buildListUrl("/api/hris/org-chart/template", { version: v(version) });

const IMPORT_403 = "Anda tidak punya hak mengimpor versi bagan. Minta admin menambahkan aksi Import pada menu Struktur Org.";

/** Error dengan status HTTP, supaya 409 (file/label berubah) bisa dibedakan. */
export class OrgChartImportError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

type ImportStep = { step: "preview" | "commit" | "error_file"; fingerprint?: string; ack_warnings?: boolean };

function postOrgChartImport(input: ChartImportInput & ImportStep) {
  const form = new FormData();
  form.append("file", input.file);
  form.append("label", input.label);
  if (input.base) form.append("base", input.base);
  form.append("step", input.step);
  if (input.fingerprint) form.append("fingerprint", input.fingerprint);
  if (input.ack_warnings) form.append("ack_warnings", "true");
  return fetch("/api/hris/org-chart/import", { method: "POST", body: form });
}

async function importFailure(res: Response): Promise<never> {
  const json = await res.json().catch(() => ({}));
  const raw = (json as { error?: string }).error;
  const msg = res.status === 403 || raw === "Insufficient permissions" ? IMPORT_403 : raw || `Impor gagal (${res.status})`;
  throw new OrgChartImportError(msg, res.status);
}

/** step=preview → pratinjau; step=commit → versi baru + pesan server. */
export async function importOrgChartFile(
  input: ChartImportInput & { step: "preview" | "commit"; fingerprint?: string; ack_warnings?: boolean }
): Promise<{ data: ChartImportPreview | { version: ChartVersion }; message?: string }> {
  const res = await postOrgChartImport(input);
  if (!res.ok) return importFailure(res);
  return res.json();
}

/** File yang sama dengan kolom "Keterangan Error". */
export async function downloadOrgChartImportErrors(input: ChartImportInput): Promise<void> {
  const res = await postOrgChartImport({ ...input, step: "error_file" });
  if (!res.ok) return importFailure(res);
  downloadBlob(await res.blob(), `Error - ${input.file.name}`);
}

/** Hapus versi draft (versi disetujui/arsip ditolak server). */
export const deleteOrgChartVersion = (id: string) =>
  apiDelete(`/api/hris/org-chart/versions/${encodeURIComponent(id)}`).catch((e: unknown) => {
    const msg = e instanceof Error ? e.message : "";
    throw new Error(msg === "Insufficient permissions" ? IMPORT_403 : msg || "Gagal menghapus versi");
  });

/** Tombol Setujui/Impor/Hapus hanya untuk akun yang punya aksinya (EPIC-104 R7). */
export const fetchOrgChartAccess = () =>
  apiGet<{ data: ChartAccess }>("/api/hris/org-chart/access").then((res) => res.data);
