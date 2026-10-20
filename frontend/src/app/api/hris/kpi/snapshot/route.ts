import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { runKpiSnapshot } from "@/lib/kpi/snapshot";
import { readOptionalJson } from "@/lib/payroll/request-input";

const snapshotSchema = z.object({
  period_month: z.coerce.number().int().min(1).max(12).nullish(),
  period_year: z.coerce.number().int().min(2020).max(2100).nullish(),
});

/**
 * POST { period_month?, period_year? }: snapshot KPI bulanan (idempoten,
 * scorecard final tak ditimpa). Default bulan berjalan. EPIC-010 Fase B.
 */
export const POST = apiHandler(async (request: NextRequest) => {
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const body = await readOptionalJson(request, snapshotSchema, "Periode tidak valid");
  const now = new Date();
  const periodMonth = body.period_month ?? now.getMonth() + 1;
  const periodYear = body.period_year ?? now.getFullYear();

  const summary = await runKpiSnapshot({ periodYear, periodMonth });
  const skipped = summary.scorecards_skipped_final
    ? `, ${summary.scorecards_skipped_final} final dilewati`
    : "";
  return NextResponse.json({
    data: summary,
    message: `Snapshot KPI ${periodMonth}/${periodYear}: ${summary.scorecards_upserted} scorecard diperbarui${skipped}`,
  });
}, "hris/kpi/snapshot.POST");
