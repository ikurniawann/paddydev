import { NextRequest, NextResponse } from "next/server";
import { ApiError, validateBody, requireIamMenuPrefix } from "@/lib/api/auth";
import { requireCanonicalId } from "@/lib/api/canonical-uuid";
import { apiHandler } from "@/lib/api/handler";
import { isSelfManager, SELF_MANAGER_ERROR } from "@/lib/hris/reporting-to";
import { IAM } from "@/lib/iam/prefixes";
import { rethrowUserServiceError } from "@/lib/hris/users-service-errors";
import { getUserEmployeeById, updateUserEmployee } from "@/lib/users/user-service";
import { updateUserEmployeeSchema } from "@/lib/users/schemas";
import {
  employeeUserId,
  requireSuperAdminForSystemAccount,
  touchesLogin,
} from "@/lib/users/system-account-guard";

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const GET = apiHandler(async (_request: NextRequest, { params }: RouteParams) => {
  await requireIamMenuPrefix([...IAM.settingsUsers, ...IAM.hrisKaryawan]);
  const { id } = await params;
  const data = await getUserEmployeeById(id);
  if (!data) throw ApiError.notFound("Employee not found");
  return NextResponse.json({ data });
}, "api/users/:id");

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteParams) => {
  const actor = await requireIamMenuPrefix(IAM.settingsUsers);
  // Dikanonikkan sebagai uuid sebelum guard & tulisan (temuan re-review EPIC-061).
  const id = requireCanonicalId((await params).id);
  const body = await validateBody(request, updateUserEmployeeSchema);
  if (isSelfManager(id, body.reporting_to)) throw ApiError.badRequest(SELF_MANAGER_ERROR);
  // Role sistem & akun yang memegangnya hanya oleh Super Admin (sama dengan Go).
  const userId = touchesLogin(body) ? await employeeUserId(id) : null;
  await requireSuperAdminForSystemAccount(actor, { role: body.role, userId });
  const data = await updateUserEmployee(actor.id, id, body).catch(rethrowUserServiceError);
  return NextResponse.json({
    data,
    message: "Employee updated successfully",
  });
}, "api/users/:id");
