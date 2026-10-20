import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { feedbackResponseUpdateSchema } from "@/lib/hris/feedback-schemas";
import {
  decideAssignment,
  deleteResponse,
  getResponse,
  requireApproverEmployeeId,
  updateResponse,
} from "@/lib/hris/feedback-repo";
import { readJson } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * GET/PUT/DELETE satu jawaban feedback. POST/PATCH memutuskan penugasan
 * ber-id sama (approve/reject); jalur utama keputusan ada di
 * /api/hris/feedback-approvals.
 */

interface RouteParams {
  params: Promise<{ id: string }>;
}

// relasi employee pada penugasan, sama dengan versi lama endpoint ini
const ASSIGNMENT_EMPLOYEE = "employees";

export const GET = apiHandler(async (_request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  return NextResponse.json({ data: await getResponse((await params).id) });
}, "hris/feedback-responses/[id] GET");

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = await params;
  const body = await readJson(request, feedbackResponseUpdateSchema, "Data tidak valid");
  return NextResponse.json({ data: await updateResponse(id, body) });
}, "hris/feedback-responses/[id] PUT");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.kpiScorecard);
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = await params;
  await deleteResponse(id);
  await auditHrisDelete(request, user, "performance.feedback_responses", id, reason);
  return NextResponse.json({ success: true });
}, "hris/feedback-responses/[id] DELETE");

const approveSchema = z.object({ manager_comments: z.string().max(5000).nullable().optional() });

/** POST — setujui penugasan. */
export const POST = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = await params;
  const body = await readJson(request, approveSchema);
  const approverId = await requireApproverEmployeeId("User not found in employees table");
  const data = await decideAssignment(
    id,
    { action: "approve", comments: body.manager_comments || null },
    approverId,
    ASSIGNMENT_EMPLOYEE
  );
  return NextResponse.json({ success: true, data, message: "Feedback approved successfully" });
}, "hris/feedback-responses/[id] POST");

const rejectSchema = z.object({
  rejection_reason: z
    .string({ error: "Rejection reason is required" })
    .min(1, "Rejection reason is required")
    .max(5000),
});

/** PATCH — tolak penugasan. */
export const PATCH = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisPerformanceAdmin);
  const { id } = await params;
  const body = await readJson(request, rejectSchema);
  const approverId = await requireApproverEmployeeId("User not found in employees table");
  const data = await decideAssignment(
    id,
    { action: "reject", reason: body.rejection_reason },
    approverId,
    ASSIGNMENT_EMPLOYEE
  );
  return NextResponse.json({
    success: true,
    data,
    message: "Feedback rejected. Employee has been notified.",
  });
}, "hris/feedback-responses/[id] PATCH");
