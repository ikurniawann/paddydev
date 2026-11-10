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

export interface ChartVersion {
  label: string;
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
