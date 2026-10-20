import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { feedbackCycleUpdateSchema } from "@/lib/hris/feedback-schemas";
import { deleteCycle, getCycle, updateCycle } from "@/lib/hris/feedback-repo";
import { readJson } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

// Modul 360 feedback belum punya UI; semua handler khusus pengelola kinerja.

interface RouteContext {
  params: Promise<{ id: string }>;
}

export const GET = apiHandler(async (_request: NextRequest, { params }: RouteContext) => {
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  return NextResponse.json({ data: await getCycle((await params).id) });
}, "hris/feedback-cycles/[id] GET");

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteContext) => {
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = await params;
  const body = await readJson(request, feedbackCycleUpdateSchema, "Data tidak valid");
  return NextResponse.json({ data: await updateCycle(id, body) });
}, "hris/feedback-cycles/[id] PUT");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteContext) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kpiScorecard);
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = await params;
  await deleteCycle(id);
  await auditHrisDelete(request, user, "performance.feedback_cycles", id, reason);
  return NextResponse.json({ message: "Cycle deleted successfully" });
}, "hris/feedback-cycles/[id] DELETE");
