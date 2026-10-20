import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import {
  deactivateSalary,
  getSalary,
  salaryUpdateSchema,
  updateSalary,
} from "@/lib/hris/employee-salary-repo";
import { readJson } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * GET    /api/hris/employee-salary/[id] — detail versi gaji
 * PUT    /api/hris/employee-salary/[id] — ubah struktur gaji (allowlist)
 * DELETE /api/hris/employee-salary/[id] — nonaktifkan versi gaji
 */

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const GET = apiHandler(async (_request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisCompensation);
  const { id } = await params;
  return NextResponse.json({ data: await getSalary(id) });
}, "hris/employee-salary/[id] GET");

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(IAM.hrisCompensation);
  const { id } = await params;
  const patch = await readJson(request, salaryUpdateSchema, "Data salary tidak valid");
  const data = await updateSalary(id, patch);
  return NextResponse.json({ data, message: "Data salary berhasil diupdate" });
}, "hris/employee-salary/[id] PUT");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.salary);
  const { id } = await params;
  await deactivateSalary(id);
  await auditHrisDelete(request, user, "hris.employee_salary", id, reason);
  return NextResponse.json({ message: "Data salary berhasil dihapus" });
}, "hris/employee-salary/[id] DELETE");
