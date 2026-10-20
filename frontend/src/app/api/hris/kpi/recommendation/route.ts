import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { UUID_RE } from "@/lib/hris/workforce-route";
import { IAM } from "@/lib/iam/prefixes";
import { loadKpiRecommendations, parseEmployeeIds } from "@/lib/kpi/recommendation";
import { searchParamsOf } from "@/lib/payroll/request-input";

/** GET ?employee_ids=a,b,c: rata-rata skor KPI 3 periode terakhir per karyawan. */
export const GET = apiHandler(async (request: NextRequest) => {
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  // Id bukan UUID diabaikan agar cast ::uuid[] tidak menggagalkan seluruh query
  const ids = parseEmployeeIds(searchParamsOf(request).employee_ids).filter((id) =>
    UUID_RE.test(id)
  );
  return NextResponse.json({ data: await loadKpiRecommendations(ids) });
}, "hris/kpi/recommendation.GET");
