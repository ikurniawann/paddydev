import { apiGet, buildListUrl } from "@/lib/api-client";

export interface ContractListItem {
  id: string;
  employee_id: string;
  employee_name: string;
  contract_number: string;
  contract_type: "pkwt" | "pkwtt";
  status: "draft" | "active" | "ended" | "terminated" | "converted";
  start_date: string;
  end_date: string | null;
  probation_end_date: string | null;
  days_left: number | null;
  position_title: string | null;
  department_name: string | null;
  /** Tidak dikirim API untuk akun tanpa akses Penggajian (EPIC-057). */
  base_salary?: string | null;
  sequence: number;
}

export interface ContractListParams {
  status?: string;
  type?: string;
  search?: string;
  days?: number;
  sort_by?: string;
  sort_order?: "asc" | "desc";
  page?: number;
  limit?: number;
}

export interface ContractListResponse {
  data: ContractListItem[];
  total: number;
  page: number;
  limit: number;
}

export const fetchContractList = (params: ContractListParams) =>
  apiGet<ContractListResponse>(
    buildListUrl("/api/hris/contracts", params as Record<string, string | number | undefined>)
  );

/** Rata-rata skor KPI 3 bulan terakhir per employee_id (rekomendasi PKWT). */
export type KpiRecommendation = Record<
  string,
  { avg_score: number; periods: number; has_final: boolean }
>;

export const fetchKpiRecommendation = (employeeIds: string[]) =>
  apiGet<{ data: KpiRecommendation }>(
    buildListUrl("/api/hris/kpi/recommendation", { employee_ids: employeeIds.join(",") })
  ).then((res) => res.data);
