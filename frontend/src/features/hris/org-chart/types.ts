export type { OrgEmployee } from "@/lib/hris/org-chart";

export interface OrgDepartment {
  id: string;
  name: string;
}

/* ── Bagan Draft 6 (EPIC-101) ── */

export interface ChartHolder {
  id: string;
  name: string;
  nip: string;
}

export interface ChartNode {
  code: string;
  parent_code: string | null;
  title: string;
  raw_title: string;
  is_position: boolean;
  job_level: string | null;
  level_confidence: string | null;
  employment_type: string | null;
  vendor: string | null;
  division_code: string;
  division_name: string;
  region: string | null;
  store_codes: string[];
  status: "active" | "planned" | "vacant" | "frozen";
  needs_confirm: boolean;
  flags: string[];
  holders: ChartHolder[];
}

export type ChartVersionStatus = "draft" | "approved" | "archived";

/** Versi bagan; field persetujuan berisi null selama belum disetujui (EPIC-102). */
export interface ChartVersion {
  id: string;
  label: string;
  status: ChartVersionStatus;
  imported_at?: string;
  valid_from: string | null;
  approved_at: string | null;
  approved_by_name: string | null;
  nodes?: number;
}

export interface ChartTreeResponse {
  version: ChartVersion;
  nodes: ChartNode[];
}

export interface ChartIssue {
  rule: string;
  code: string;
  message: string;
}

export interface ChartValidation {
  errors: ChartIssue[];
  warnings: ChartIssue[];
}

export interface ChartGapSummary {
  employees: number;
  matched: number;
  ambiguous: number;
  outside: number;
  manager_diff: number;
  alias_review: number;
  empty_nodes: number;
  stores_outside: number;
}

export interface ChartGapResponse {
  version: ChartVersion;
  summary: ChartGapSummary;
  titles_outside: { title: string; employees: number }[];
  stores_outside: string[];
  chart_stores_missing: string[];
}

export interface ChartTreeFilter {
  division: string;
  region: string;
}

/* ── Posisi Kosong (EPIC-102) ── */

export type VacancyState = "empty" | "maybe" | "planned" | "frozen";

export interface ChartVacancyCounts {
  positions: number;
  filled: number;
  empty: number;
  maybe: number;
  planned: number;
  frozen: number;
}

export interface ChartVacancyGroup extends ChartVacancyCounts {
  division: string;
  division_name: string;
  region: string;
}

export interface ChartVacancyRow {
  code: string;
  title: string;
  division_code: string;
  division_name: string;
  region: string;
  store_codes: string[];
  parent_code: string;
  parent_title: string;
  state: VacancyState;
  holders: number;
  maybe_holders: string[];
}

export interface ChartVacancyResponse {
  version: ChartVersion;
  summary: ChartVacancyCounts;
  groups: ChartVacancyGroup[];
  rows: ChartVacancyRow[];
}

export interface ChartVacancyFilter extends ChartTreeFilter {
  state: string;
}

/* ── Impor versi bagan (EPIC-103) ── */

export interface ChartUploadIssue {
  line: number;
  column: string;
  name: string;
  message: string;
}

export interface ChartDiff {
  added: string[];
  removed: string[];
  changed: { code: string; title: string; fields: string[] }[];
}

export interface ChartImportPreview {
  label: string;
  base: { label: string };
  summary: { nodes: number; positions: number; errors: number; warnings: number };
  errors: ChartUploadIssue[];
  warnings: ChartUploadIssue[];
  diff: ChartDiff;
  vacancy: ChartVacancyCounts;
  fingerprint: string;
}

export interface ChartImportInput {
  file: File;
  label: string;
  base: string;
}

/** Filter region untuk node tanpa region (HQ), EPIC-104. */
export const NO_REGION = "-";

/** Aksi bagan yang dimiliki akun ini (EPIC-104 R7). */
export interface ChartAccess {
  approve: boolean;
  import: boolean;
}
