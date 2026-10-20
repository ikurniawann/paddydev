import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { readJson } from "@/lib/hris/workforce-route";
import { IAM } from "@/lib/iam/prefixes";
import { createServerPgClient } from "@/lib/pg/create-client";
import { deleteRun, getRun, updateRun, updateRunSchema } from "@/lib/payroll/runs";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

type Ctx = { params: Promise<{ id: string }> };

/** GET /api/hris/payroll/[id]: run + detail per karyawan. */
export const GET = apiHandler(async (_request: NextRequest, { params }: Ctx) => {
  await requireIamMenuPrefix(IAM.hrisCompensation);
  const { id } = await params;
  return NextResponse.json({ data: await getRun(await createServerPgClient(), id) });
}, "hris/payroll/[id].GET");

/** PUT /api/hris/payroll/[id]: transisi status (draft → processing → completed → paid). */
export const PUT = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const user = await requireIamMenuPrefix(IAM.hrisCompensation);
  const { id } = await params;
  const input = await readJson(request, updateRunSchema);
  return NextResponse.json(await updateRun(await createServerPgClient(), user.id, id, input));
}, "hris/payroll/[id].PUT");

/** DELETE /api/hris/payroll/[id]: run paid tidak bisa dihapus. */
export const DELETE = apiHandler(async (request: NextRequest, { params }: Ctx) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.runPenggajian);
  const { id } = await params;
  await deleteRun(await createServerPgClient(), id);
  await auditHrisDelete(request, user, "hris.payroll_runs", id, reason);
  return NextResponse.json({ message: "Payroll run berhasil dihapus" });
}, "hris/payroll/[id].DELETE");
