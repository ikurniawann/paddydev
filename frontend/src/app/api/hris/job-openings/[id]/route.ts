import { NextRequest, NextResponse } from "next/server";
import { ApiError, requireIamGuard } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import {
  deleteJobOpening,
  jobOpeningBodySchema,
  normalizeJobOpening,
  updateJobOpening,
  validateJobOpening,
} from "@/lib/hris/job-openings";
import { readJson } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const PATCH = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const guard = await requireIamGuard(IAM.hrisRecruitment);
  if (guard.error) return guard.error;
  const { id } = await params;
  const payload = normalizeJobOpening(await readJson(request, jobOpeningBodySchema), "update");
  const invalid = validateJobOpening(payload, "update");
  if (invalid) throw ApiError.badRequest(invalid);

  const data = await updateJobOpening(id, payload);
  return NextResponse.json({ data, message: "Lowongan berhasil diperbarui" });
}, "hris/job-openings PATCH");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.jobPortal);
  const { id } = await params;
  await deleteJobOpening(id);
  await auditHrisDelete(request, user, "recruitment.job_openings", id, reason);
  return NextResponse.json({ message: "Lowongan berhasil dihapus" });
}, "hris/job-openings DELETE");
