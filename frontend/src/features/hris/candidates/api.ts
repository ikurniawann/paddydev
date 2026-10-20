import { apiGet, apiPost, apiDelete, buildListUrl } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type { Brand } from "@/types";
import type { CandidateCreateInput } from "@/lib/recruitment/candidate-query";
import type { CvOcrFields } from "@/lib/recruitment/cv-ocr";
import type {
  CandidateListParams,
  CandidateListResult,
  CandidateRow,
  CandidateView,
  CandidateActivity,
  CandidateNote,
  CandidateDetailResult,
} from "./types";

type CandidateFilter = Partial<Omit<CandidateListParams, "page" | "perPage">> & {
  sort?: "created_at" | "updated_at";
};

interface CandidatesResponse {
  data: CandidateRow[];
  meta: { total: number };
}

export const fetchCandidateList = ({ page, perPage, ...filter }: CandidateListParams): Promise<CandidateListResult> =>
  apiGet<CandidatesResponse>(buildListUrl("/api/candidates", { ...filter, page, limit: perPage })).then(
    (res) => ({ data: res.data ?? [], count: res.meta?.total ?? 0 })
  );

/** Semua kandidat sesuai filter tanpa paging (papan pipeline, talent pool, ekspor). */
export const fetchAllCandidates = (filter: CandidateFilter = {}): Promise<CandidateRow[]> =>
  apiGet<CandidatesResponse>(buildListUrl("/api/candidates", { ...filter, all: true })).then(
    (res) => res.data ?? []
  );

/** Brand aktif, urut nama: dipakai filter & form di seluruh modul rekrutmen. */
export const fetchActiveBrands = (): Promise<Brand[]> =>
  apiGet<{ data: Brand[] }>("/api/brands?active=true").then((res) => res.data ?? []);

export async function createCandidate(payload: CandidateCreateInput, cvFile?: File | null) {
  const { data: candidate } = await apiPost<{ data: CandidateRow }>("/api/candidates", payload);

  if (cvFile) {
    const formData = new FormData();
    formData.append("file", cvFile);
    const res = await fetch(`/api/candidates/${candidate.id}/cv-upload`, { method: "POST", body: formData });
    if (!res.ok) {
      const result = await res.json().catch(() => ({}));
      console.error("CV upload failed:", result.error);
    }
  }

  return candidate;
}

export const deleteCandidate = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/candidates/${id}`, reason));

export async function fetchCandidateDetail(id: string): Promise<CandidateDetailResult> {
  const [candidate, activities, notes] = await Promise.all([
    apiGet<{ data: CandidateView }>(`/api/candidates/${id}`),
    apiGet<{ data: CandidateActivity[] }>(`/api/candidates/${id}/activities`),
    apiGet<{ data: CandidateNote[] }>(`/api/candidates/${id}/notes`),
  ]);
  return { candidate: candidate.data, activities: activities.data ?? [], notes: notes.data ?? [] };
}

/** Lewat endpoint stage supaya jejak (siapa + dari-ke) tercatat otomatis. */
export const updateCandidateStatus = (id: string, status: string) =>
  apiPost(`/api/candidates/${id}/stage`, { status });

export const addCandidateNote = (id: string, content: string) =>
  apiPost<{ data: CandidateNote }>(`/api/candidates/${id}/notes`, { content }).then((res) => res.data);

/** OCR CV untuk mengisi otomatis form tambah kandidat. */
export async function extractCvFields(file: File): Promise<CvOcrFields> {
  const formData = new FormData();
  formData.append("file", file);
  const res = await fetch("/api/candidates/cv-extract", { method: "POST", body: formData });
  const json = await res.json().catch(() => null);
  if (!res.ok) throw new Error(json?.error || "OCR CV gagal");
  return json?.data ?? {};
}
