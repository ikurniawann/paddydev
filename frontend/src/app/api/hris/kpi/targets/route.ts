import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { createTarget, createTargetSchema, deleteTarget, listTargets } from "@/lib/kpi/targets-repo";
import { createServerPgClient } from "@/lib/pg/create-client";
import { parseInput, readOptionalJson, searchParamsOf } from "@/lib/payroll/request-input";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/** GET: 500 target KPI terbaru. */
export const GET = apiHandler(async () => {
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  return NextResponse.json({ data: await listTargets(await createServerPgClient()) });
}, "hris/kpi/targets.GET");

/** POST: target baru dengan satu scope (role/department/karyawan) atau umum. */
export const POST = apiHandler(async (request: NextRequest) => {
  const user = await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const input = await readOptionalJson(request, createTargetSchema);
  const data = await createTarget(await createServerPgClient(), user.id, input);
  return NextResponse.json({ data, message: "Target tersimpan" }, { status: 201 });
}, "hris/kpi/targets.POST");

const deleteQuerySchema = z.object({ id: z.string({ error: "id wajib" }).min(1, "id wajib") });

/** DELETE ?id: hapus target. */
export const DELETE = apiHandler(async (request: NextRequest) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kpiScorecard);
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = parseInput(deleteQuerySchema, searchParamsOf(request));
  await deleteTarget(await createServerPgClient(), id);
  await auditHrisDelete(request, user, "performance.kpi_targets", id, reason);
  return NextResponse.json({ message: "Target dihapus" });
}, "hris/kpi/targets.DELETE");
