import { apiGet, apiPut, buildListUrl } from "@/lib/api-client";
import { downloadBlob } from "@/lib/purchasing/report-ui-shared";
import type {
  LeaveBalanceAccess,
  LeaveBalanceImportPreview,
  LeaveBalanceImportScope,
  LeaveBalanceListParams,
  LeaveBalancePayload,
  LeaveBalanceView,
  LeaveSettings,
  LeaveSettingsPatch,
} from "./types";

/** Daftar karyawan aktif dalam cakupan; baris yang belum ada tampil dengan jatah default (`exists: false`). */
export const fetchLeaveBalances = (params: LeaveBalanceListParams) =>
  apiGet<{ data: LeaveBalanceView[] }>(
    buildListUrl("/api/hris/leave-balances", { year: params.year, branch_id: params.branch_id, q: params.q })
  ).then((res) => res.data ?? []);

export const fetchLeaveBalance = (employeeId: string, year: number) =>
  apiGet<{ data: LeaveBalanceView }>(`/api/hris/leave-balances/${employeeId}?year=${year}`).then((res) => res.data);

export const updateLeaveBalance = (employeeId: string, payload: LeaveBalancePayload) =>
  apiPut<{ data: LeaveBalanceView; message: string }>(`/api/hris/leave-balances/${employeeId}`, payload);

/** Objek datar `{read, update, import, export}` tanpa amplop `data`. */
export const fetchLeaveBalanceAccess = () => apiGet<LeaveBalanceAccess>("/api/hris/me/leave-balance-access");

export const fetchLeaveSettings = () =>
  apiGet<{ data: LeaveSettings }>("/api/hris/leave-settings").then((res) => res.data);

export const saveLeaveSettings = (patch: LeaveSettingsPatch) =>
  apiPut<{ data: LeaveSettings; message: string }>("/api/hris/leave-settings", patch);

/** Unduh template Excel (butuh `export`); branch_id kosong = semua cabang. */
export const leaveBalanceTemplateUrl = (p: LeaveBalanceImportScope) =>
  buildListUrl("/api/hris/leave-balances/template", { year: p.year, branch_id: p.branch_id });

/** Galat import dengan status HTTP-nya (409 = saldo berubah sejak pratinjau, perlu periksa ulang). */
export class LeaveBalanceImportError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "LeaveBalanceImportError";
    this.status = status;
  }
}

type ImportInput = LeaveBalanceImportScope & {
  file: File;
  step: "preview" | "commit" | "error_file";
  fingerprint?: string;
  ack_warnings?: boolean;
};

function importForm(input: ImportInput): FormData {
  const form = new FormData();
  form.append("file", input.file);
  form.append("year", String(input.year));
  form.append("branch_id", input.branch_id);
  form.append("step", input.step);
  if (input.fingerprint) form.append("fingerprint", input.fingerprint);
  if (input.ack_warnings) form.append("ack_warnings", "true");
  return form;
}

const postImport = (input: ImportInput) =>
  fetch("/api/hris/leave-balances/import", { method: "POST", body: importForm(input) });

/** step=preview mengembalikan pratinjau lengkap; step=commit mengembalikan ringkasan + pesan server. */
export async function importLeaveBalanceFile(
  input: Omit<ImportInput, "step"> & { step: "preview" | "commit" }
): Promise<{ data: LeaveBalanceImportPreview; message?: string }> {
  const res = await postImport(input);
  const json = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new LeaveBalanceImportError((json as { error?: string }).error ?? `Upload gagal (${res.status})`, res.status);
  }
  return json as { data: LeaveBalanceImportPreview; message?: string };
}

/** File yang sama dengan keterangan error per baris (kolom "Keterangan Error"). */
export async function downloadLeaveBalanceErrors(input: LeaveBalanceImportScope & { file: File }): Promise<void> {
  const res = await postImport({ ...input, step: "error_file" });
  if (!res.ok) throw new Error(`Gagal membuat file error (${res.status})`);
  downloadBlob(await res.blob(), `Error - ${input.file.name}`);
}
