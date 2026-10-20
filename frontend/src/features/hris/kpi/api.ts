import { apiDelete, apiGet, apiPatch, apiPost, apiPut, buildListUrl } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type {
  CreateDeptTaskPayload,
  CreateKpiTargetPayload,
  DeptOccurrenceAction,
  DeptTasksData,
  KpiConfigData,
  KpiScorecardsResult,
  KpiTargetRowUI,
  PerfCycleRow,
  PerfRealtimeData,
  PerfReviewDetail,
  PerfReviewPatch,
  PerfReviewsData,
  SaveKpiConfigPayload,
  SaveRubricPayload,
  SnapshotSummaryResult,
} from "./types";

const BASE = "/api/hris/kpi";

export const fetchKpiScorecards = (params: {
  period_year: number;
  period_month: number;
  employee_id?: string;
}) =>
  apiGet<KpiScorecardsResult>(buildListUrl(`${BASE}/scorecards`, params));

export const fetchKpiHistory = (params: {
  employee_id: string;
  history: number;
}) =>
  apiGet<KpiScorecardsResult>(buildListUrl(`${BASE}/scorecards`, params));

export const runKpiSnapshotApi = (payload: {
  period_month: number;
  period_year: number;
}) =>
  apiPost<{ data: SnapshotSummaryResult; message?: string }>(`${BASE}/snapshot`, payload);

export const saveKpiRubric = (payload: SaveRubricPayload) =>
  apiPost<{ data: unknown }>(`${BASE}/rubric`, payload);

export const updateScorecardStatus = (payload: {
  action: "finalize" | "reopen";
  scorecard_id: string;
}) =>
  apiPatch<{ data: unknown; wa_link?: string | null; message?: string }>(
    `${BASE}/scorecards`,
    payload
  );

export const fetchKpiTeam = (params: {
  period_year: number;
  period_month: number;
}) =>
  apiGet<KpiScorecardsResult>(
    buildListUrl(`${BASE}/scorecards`, { ...params, team: "1" })
  );

export const fetchKpiTargets = () =>
  apiGet<{ data: KpiTargetRowUI[] }>(`${BASE}/targets`).then(
    (res) => res.data
  );

export const createKpiTarget = (payload: CreateKpiTargetPayload) =>
  apiPost<{ data: unknown }>(`${BASE}/targets`, payload);

export const deleteKpiTarget = (id: string, reason: string) =>
  apiDelete(withReasonQuery(buildListUrl(`${BASE}/targets`, { id }), reason));

// ── Task Departemen ──
const DEPT_TASKS = "/api/hris/dept-tasks";

export const fetchDeptTasks = (month: string, departmentId?: string) =>
  apiGet<{ data: DeptTasksData }>(
    buildListUrl(DEPT_TASKS, { month, department_id: departmentId })
  ).then((res) => res.data);

export const updateDeptOccurrence = (occurrenceId: string, body: DeptOccurrenceAction) =>
  apiPatch<{ message?: string }>(`${DEPT_TASKS}/occurrences/${occurrenceId}`, body);

export const createDeptTask = (payload: CreateDeptTaskPayload) =>
  apiPost<{ message?: string }>(DEPT_TASKS, payload);

// ── Performance Review ──
const PERF = "/api/hris/performance";

/** Tanpa year/quarter: server memakai kuartal berjalan. */
export const fetchPerfRealtime = (year?: number, quarter?: number) =>
  apiGet<{ data: PerfRealtimeData }>(buildListUrl(`${PERF}/realtime`, { year, quarter })).then(
    (res) => res.data
  );

export const fetchPerfCycles = () =>
  apiGet<{ data: { cycles: PerfCycleRow[]; is_hr: boolean } }>(`${PERF}/cycles`).then(
    (res) => res.data
  );

export const createPerfCycle = (periodYear: number, periodQuarter: number) =>
  apiPost<{ message?: string }>(`${PERF}/cycles`, {
    period_year: periodYear,
    period_quarter: periodQuarter,
  });

export const fetchPerfReviews = (cycleId: string) =>
  apiGet<{ data: PerfReviewsData }>(
    buildListUrl(`${PERF}/reviews`, { cycle_id: cycleId })
  ).then((res) => res.data);

export const fetchPerfReview = (id: string) =>
  apiGet<{ data: PerfReviewDetail }>(`${PERF}/reviews/${id}`).then((res) => res.data);

export const patchPerfReview = (id: string, payload: PerfReviewPatch) =>
  apiPatch<{ message?: string }>(`${PERF}/reviews/${id}`, payload);

// ── Konfigurasi KPI per departemen ──
export const fetchKpiConfig = () =>
  apiGet<{ data: KpiConfigData }>("/api/hris/kpi-config").then((res) => res.data);

export const saveKpiConfig = (payload: SaveKpiConfigPayload) =>
  apiPut<{ message?: string }>("/api/hris/kpi-config", payload);
