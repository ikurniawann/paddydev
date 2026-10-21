import { NextRequest, NextResponse } from "next/server";
import { ApiError, requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { EMPLOYEE_RECORD_MANAGERS, requireEmployeeAccess } from "@/lib/hris/employee-access";
import { employeeUpdateSchema } from "@/lib/hris/employee-update-schema";
import { isSelfManager, SELF_MANAGER_ERROR } from "@/lib/hris/reporting-to";
import { deactivateEmployee, getEmployee, updateEmployee } from "@/lib/hris/employees-repo";
import { readJson } from "@/lib/hris/workforce-route";
import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "@/lib/hris/delete-guard";

/**
 * GET    /api/hris/employees/[id] — detail; pengelola atau karyawan itu sendiri (ESS).
 * PUT    /api/hris/employees/[id] — update via allowlist Zod, bukan spread body.
 * DELETE /api/hris/employees/[id] — soft delete (is_active=false).
 */

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const GET = apiHandler(async (_request: NextRequest, { params }: RouteParams) => {
  const { id } = await params;
  await requireEmployeeAccess(id);
  return NextResponse.json({ data: await getEmployee(id) });
}, "hris/employees/[id] GET");

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix(EMPLOYEE_RECORD_MANAGERS);
  const { id } = await params;
  const body = await readJson(request, employeeUpdateSchema, "Data karyawan tidak valid");
  if (isSelfManager(id, body.reporting_to)) throw ApiError.badRequest(SELF_MANAGER_ERROR);
  const data = await updateEmployee(id, body);
  return NextResponse.json({ data, message: "Data karyawan berhasil diupdate" });
}, "hris/employees/[id] PUT");

export const DELETE = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const { user, reason } = await requireHrisDelete(request, HRIS_LEAF.karyawan);
  const { id } = await params;
  const data = await deactivateEmployee(id);
  await auditHrisDelete(request, user, "hris.employees", id, reason);
  return NextResponse.json({ data, message: "Karyawan berhasil dihapus (non-aktif)" });
}, "hris/employees/[id] DELETE");
