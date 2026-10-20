import { NextRequest, NextResponse } from "next/server";
import { ApiError, requireIamMenuPrefix, validateBody } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import {
  HRIS_ROLE_SUPER_ADMIN_ERROR,
  isHrisStaffRole,
  requireIamSuperAdmin,
  requireSuperAdminForSystemRole,
} from "@/lib/iam/system-role-guard";
import { mapRoleDetail } from "@/lib/iam/role-mapper";
import {
  deleteIamRoleInDb,
  getIamRoleFromDb,
  listRolePermissionsMatrixFromDb,
  updateIamRoleInDb,
} from "@/lib/iam/role-repository";
import { roleUpdateSchema } from "@/lib/settings/iam-schemas";
import { rethrowUserFacing } from "@/lib/settings/route-errors";

type RouteContext = { params: Promise<{ id: string }> };

const notFound = () => ApiError.notFound("Role not found");

export const GET = apiHandler(async (_request: NextRequest, { params }: RouteContext) => {
  await requireIamMenuPrefix(IAM.settingsRoles);
  const { id } = await params;
  const row = await getIamRoleFromDb(id);
  if (!row) throw notFound();
  return NextResponse.json(mapRoleDetail(row, await listRolePermissionsMatrixFromDb(id)));
}, "GET /api/settings/iam/roles/[id]");

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteContext) => {
  const user = await requireIamMenuPrefix(IAM.settingsRoles);
  const { id } = await params;
  const body = await validateBody(request, roleUpdateSchema);
  const existing = await getIamRoleFromDb(id);
  if (!existing) throw notFound();
  if (existing.is_system) await requireSuperAdminForSystemRole(user);
  const newCode = body.code?.trim()?.toLowerCase();
  if (isHrisStaffRole(existing.code) || (newCode && newCode !== existing.code && isHrisStaffRole(newCode))) {
    await requireIamSuperAdmin(user, HRIS_ROLE_SUPER_ADMIN_ERROR);
  }
  if (existing.is_system && body.code && body.code !== existing.code) {
    throw ApiError.badRequest("System role code cannot be changed");
  }

  const updated = await updateIamRoleInDb(id, {
    code: existing.is_system ? undefined : newCode,
    name: body.name?.trim(),
    description: body.description,
    isActive: body.isActive,
  });
  if (!updated) throw notFound();
  return NextResponse.json({ id });
}, "PUT /api/settings/iam/roles/[id]");

export const DELETE = apiHandler(async (_request: NextRequest, { params }: RouteContext) => {
  const user = await requireIamMenuPrefix(IAM.settingsRoles);
  const { id } = await params;
  const existing = await getIamRoleFromDb(id);
  if (existing && isHrisStaffRole(existing.code)) await requireIamSuperAdmin(user, HRIS_ROLE_SUPER_ADMIN_ERROR);
  const deleted = await deleteIamRoleInDb(id).catch(rethrowUserFacing(/System roles/));
  if (!deleted) throw notFound();
  return NextResponse.json({ success: true });
}, "DELETE /api/settings/iam/roles/[id]");
