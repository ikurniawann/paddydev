import { apiGet, buildListUrl } from "@/lib/api-client";
import { downloadBlob } from "@/lib/purchasing/report-ui-shared";
import type { ScheduleGrid, SchedulePreview } from "./types";

type Range = { branch_id: string; from: string; to: string };

export const fetchScheduleGrid = (p: Range) => apiGet<{ data: ScheduleGrid }>(buildListUrl("/api/hris/schedules", p));

export const scheduleTemplateUrl = (p: Range) => buildListUrl("/api/hris/schedules/template", p);

async function postImport(form: FormData): Promise<Response> {
  return fetch("/api/hris/schedules/import", { method: "POST", body: form });
}

function formOf(input: Range & { file: File; step: string; fingerprint?: string; ack_warnings?: boolean }): FormData {
  const form = new FormData();
  form.append("file", input.file);
  form.append("branch_id", input.branch_id);
  form.append("from", input.from);
  form.append("to", input.to);
  form.append("step", input.step);
  if (input.fingerprint) form.append("fingerprint", input.fingerprint);
  if (input.ack_warnings) form.append("ack_warnings", "true");
  return form;
}

export async function importScheduleFile(
  input: Range & { file: File; step: "preview" | "commit"; fingerprint?: string; ack_warnings?: boolean }
): Promise<{ data: SchedulePreview; message?: string }> {
  const res = await postImport(formOf(input));
  const json = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((json as { error?: string }).error ?? `Upload gagal (${res.status})`);
  return json as { data: SchedulePreview; message?: string };
}

export async function downloadScheduleErrors(input: Range & { file: File }): Promise<void> {
  const res = await postImport(formOf({ ...input, step: "error_file" }));
  if (!res.ok) throw new Error(`Gagal membuat file error (${res.status})`);
  downloadBlob(await res.blob(), `Error - ${input.file.name}`);
}
