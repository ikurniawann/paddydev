import { NextResponse, type NextRequest } from "next/server";
import { ApiError, requireIamMenuPrefix, validateBody } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { isUuid } from "@/lib/recruitment/candidate-query";
import { deleteEmploymentStatus, employmentStatusSchema, updateEmploymentStatus } from "@/lib/hris/master-data";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisMaster);
  const { id } = await params;
  if (!isUuid(id)) throw ApiError.badRequest("ID tidak valid");
  const data = await updateEmploymentStatus(id, await validateBody(request, employmentStatusSchema));
  return NextResponse.json({ data, message: "Status kepegawaian berhasil diupdate" });
}, "master/employment-statuses/[id]");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.statusKaryawan);
  const { id } = await params;
  if (!isUuid(id)) throw ApiError.badRequest("ID tidak valid");
  await deleteEmploymentStatus(id);
  await auditHrisDelete(request, user, "hris.employment_statuses", id, reason);
  return NextResponse.json({ message: "Status kepegawaian berhasil dihapus" });
}, "master/employment-statuses/[id]");
