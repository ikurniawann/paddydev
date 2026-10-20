import { NextResponse, type NextRequest } from "next/server";
import { ApiError, requireIamMenuPrefix, validateBody } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { isUuid } from "@/lib/recruitment/candidate-query";
import { deleteDepartment, departmentSchema, updateDepartment } from "@/lib/hris/master-data";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisMaster);
  const { id } = await params;
  if (!isUuid(id)) throw ApiError.badRequest("ID tidak valid");
  const data = await updateDepartment(id, await validateBody(request, departmentSchema));
  return NextResponse.json({ data, message: "Departemen berhasil diupdate" });
}, "master/departments/[id]");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.departemen);
  const { id } = await params;
  if (!isUuid(id)) throw ApiError.badRequest("ID tidak valid");
  await deleteDepartment(id);
  await auditHrisDelete(request, user, "hris.departments", id, reason);
  return NextResponse.json({ message: "Departemen berhasil dihapus" });
}, "master/departments/[id]");
