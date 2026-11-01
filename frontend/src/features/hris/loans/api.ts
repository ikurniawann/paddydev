import { apiGet, apiPost, buildListUrl } from "@/lib/api-client";
import type { LoanDecision, LoanPayload, LoanRow } from "./types";

/** status "all" = tanpa filter. */
export const fetchLoans = (status: string) =>
  apiGet<{ data: LoanRow[] }>(
    buildListUrl("/api/hris/loans", { status: status === "all" ? undefined : status })
  ).then((res) => res.data ?? []);

export const createLoan = (payload: LoanPayload) =>
  apiPost<{ message?: string }>("/api/hris/loans", payload);

export const decideLoan = ({ id, approved, rejection_reason }: LoanDecision) =>
  apiPost<{ message?: string }>(`/api/hris/loans/${id}/approve`, { approved, rejection_reason });

/**
 * Unggah 1 lampiran pinjaman (PDF/JPG/PNG/WebP ≤ 5 MB) → path privat. HR
 * mengunggah atas nama karyawan lewat employeeId; selain HR server memakai
 * akun sendiri (EPIC-076 R8).
 */
export async function uploadLoanAttachment(file: File, employeeId?: string): Promise<string> {
  const body = new FormData();
  body.append("file", file);
  if (employeeId) body.append("employee_id", employeeId);
  const res = await fetch("/api/hris/loans/attachments", { method: "POST", body });
  const json = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(json?.error || "Gagal mengunggah lampiran");
  return json.data.path as string;
}
