import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { requireWorkforceActor } from "@/lib/hris/workforce-route";
import { IAM } from "@/lib/iam/prefixes";
import {
  listScorecards,
  scorecardActionSchema,
  scorecardQuerySchema,
  setScorecardStatus,
} from "@/lib/kpi/scorecards-repo";
import { parseInput, readOptionalJson, searchParamsOf } from "@/lib/payroll/request-input";
import { createServerPgClient } from "@/lib/pg/create-client";

/**
 * GET ?period_year&period_month&employee_id|team=1|history=N: HR semua,
 * atasan (team=1) bawahannya, lainnya hanya scorecard miliknya.
 */
export const GET = apiHandler(async (request: NextRequest) => {
  const actor = await requireWorkforceActor();
  const query = parseInput(scorecardQuerySchema, searchParamsOf(request));
  return NextResponse.json(await listScorecards(await createServerPgClient(), actor, query));
}, "hris/kpi/scorecards.GET");

/** PATCH { action: finalize|reopen, scorecard_id } */
export const PATCH = apiHandler(async (request: NextRequest) => {
  const user = await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const input = await readOptionalJson(request, scorecardActionSchema);
  return NextResponse.json(await setScorecardStatus(await createServerPgClient(), user.id, input));
}, "hris/kpi/scorecards.PATCH");
