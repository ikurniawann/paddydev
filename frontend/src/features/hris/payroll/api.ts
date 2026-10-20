import { apiGet, apiPost, apiPut, apiDelete } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type {
  PayrollRun,
  CreatePayrollPayload,
  CalculatePayrollResult,
  PayrollRunDetail,
  PayslipDetail,
  StoreCostReport,
  PayrollEntity,
  PayrollInput,
  PayrollInputImportResult,
} from "./types";

export const fetchPayrollRuns = () =>
  apiGet<{ data: PayrollRun[] }>("/api/hris/payroll").then((res) => res.data || []);

export const fetchPayrollRun = (id: string) =>
  apiGet<{ data: PayrollRunDetail }>(`/api/hris/payroll/${id}`).then((res) => res.data);

export const createPayrollRun = (payload: CreatePayrollPayload) =>
  apiPost<{ data: PayrollRun }>("/api/hris/payroll", payload);

export const calculatePayroll = (runId: string, includeThr: boolean) =>
  apiPost<CalculatePayrollResult>(`/api/hris/payroll/${runId}/calculate`, {
    include_thr: includeThr,
  });

export const updatePayrollStatus = (runId: string, status: string) =>
  apiPut<{ data: PayrollRun }>(`/api/hris/payroll/${runId}`, { status });

export const deletePayrollRun = (runId: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/hris/payroll/${runId}`, reason));

export const notifyPayslip = (payrollDetailId: string) =>
  apiPost<{ message?: string; data?: { wa_link?: string | null } }>("/api/hris/payslips/notify", {
    payroll_detail_id: payrollDetailId,
  });

export const fetchPayslip = (payrollRunId: string, employeeId: string) =>
  apiGet<{ data: PayslipDetail[] }>(
    `/api/hris/payslips?payroll_run_id=${payrollRunId}&employee_id=${employeeId}`
  ).then((res) => res.data?.[0] ?? null);

// ---- Laporan Biaya Gaji per Toko ----

export const fetchStoreCosts = (runId: string) =>
  apiGet<{ data: StoreCostReport }>(`/api/hris/payroll/${runId}/store-costs`).then((res) => res.data);

export async function downloadStoreCosts(runId: string, format: "csv" | "xlsx"): Promise<Blob> {
  const res = await fetch(`/api/hris/payroll/${runId}/store-costs/export?format=${format}`);
  if (!res.ok) {
    const error = await res.json().catch(() => ({}));
    throw new Error(error.error || "Export gagal");
  }
  return res.blob();
}

// ---- Pengaturan payroll (payroll_settings + payroll_tax_config) ----

export type PayrollSettingsRow = Record<string, unknown> & { id?: string };

export interface PayrollSettingsData {
  settings: PayrollSettingsRow | null;
  tax_config: PayrollSettingsRow | null;
  tax_year: number;
}

export interface SavePayrollSettingsPayload {
  settings?: Record<string, unknown>;
  tax_config?: Record<string, unknown> & { tax_year: number };
}

export const fetchPayrollInputs = (runId: string) =>
  apiGet<{ data: PayrollInput[] }>(`/api/hris/payroll/${runId}/inputs`).then((res) => res.data || []);

/** Simpan satu komponen manual; amount 0 menghapusnya. */
export const savePayrollInput = (
  runId: string,
  payload: { employee_id: string; code: string; amount: number; note?: string | null }
) => apiPut<{ data: PayrollInput | null }>(`/api/hris/payroll/${runId}/inputs`, payload);

export const deletePayrollInput = (runId: string, employeeId: string, code: string, reason: string) =>
  apiDelete(
    withReasonQuery(
      `/api/hris/payroll/${runId}/inputs?employee_id=${encodeURIComponent(employeeId)}&code=${encodeURIComponent(code)}`,
      reason
    )
  );

export const importPayrollInputs = (runId: string, csv: string) =>
  apiPost<{ data: PayrollInputImportResult }>(`/api/hris/payroll/${runId}/inputs/import`, { csv }).then(
    (res) => res.data
  );

export const fetchPayrollSettings = (taxYear?: number) =>
  apiGet<{ data: PayrollSettingsData }>(
    `/api/hris/payroll-settings${taxYear ? `?tax_year=${taxYear}` : ""}`
  ).then((res) => res.data);

export const savePayrollSettings = (payload: SavePayrollSettingsPayload) =>
  apiPut<{ data: PayrollSettingsData }>("/api/hris/payroll-settings", payload);

export const fetchPayrollEntities = () =>
  apiGet<{ data: PayrollEntity[] }>("/api/hris/payroll/entities").then((res) => res.data || []);
