import { apiGet, apiPost, buildListUrl } from "@/lib/api-client";
import type { DisciplinaryLetter, IssueLetterPayload, LetterLateness, LetterListParams } from "./types";

const BASE = "/api/hris/disciplinary-letters";

/** Tautan lampiran surat (privat; HR, karyawan, dan penyetujunya). */
export const letterAttachmentUrl = (path: string) => `${BASE}/attachment/${path}`;

/** dd-mm-yyyy, sama dengan teks lonceng. */
export const formatLetterDate = (iso: string) => {
  const [y, m, d] = iso.split("-");
  return d && m && y ? `${d}-${m}-${y}` : iso;
};

export const fetchLetters = ({ employeeId, includeRevoked }: LetterListParams) =>
  apiGet<{ data: DisciplinaryLetter[] }>(
    buildListUrl(BASE, { employee_id: employeeId, include_revoked: includeRevoked ? "1" : undefined })
  ).then((res) => res.data ?? []);

export const fetchMyLetters = () =>
  apiGet<{ data: DisciplinaryLetter[] }>("/api/hris/me/disciplinary-letters").then((res) => res.data ?? []);

export const fetchLetterLateness = (employeeId: string) =>
  apiGet<{ data: LetterLateness }>(buildListUrl(`${BASE}/lateness`, { employee_id: employeeId })).then((res) => res.data);

export const issueLetter = (payload: IssueLetterPayload) =>
  apiPost<{ data: DisciplinaryLetter }>(BASE, payload).then((res) => res.data);

export const revokeLetter = (id: string, reason: string) =>
  apiPost<{ data: DisciplinaryLetter }>(`${BASE}/${id}/revoke`, { reason }).then((res) => res.data);

/** Satu PDF/JPG/PNG/WebP maks. 5 MB untuk surat karyawan itu; mengembalikan path privat. */
export async function uploadLetterAttachment(file: File, employeeId: string): Promise<string> {
  const body = new FormData();
  body.append("file", file);
  body.append("employee_id", employeeId);
  const res = await fetch(`${BASE}/attachments`, { method: "POST", body });
  const json = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(json?.error || "Gagal mengunggah lampiran");
  return json.data.path as string;
}
