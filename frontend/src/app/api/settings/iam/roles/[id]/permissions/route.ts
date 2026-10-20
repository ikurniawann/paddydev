import { NextRequest, NextResponse } from "next/server";
import { ApiError, requireIamMenuPrefix, validateBody } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import {
  ADMIN_HRIS_EMPTY_ERROR,
  HRIS_DELETE_SUPER_ADMIN_ERROR,
  HRIS_ROLE_SUPER_ADMIN_ERROR,
  addsHrisDelete,
  isHrisStaffRole,
  requireIamSuperAdmin,
  requireSuperAdminForSystemRole,
} from "@/lib/iam/system-role-guard";
import {
  getIamRoleFromDb,
  listRolePermissionsMatrixFromDb,
  replaceIamRolePermissionsInDb,
} from "@/lib/iam/role-repository";
import { mapRoleDetail } from "@/lib/iam/role-mapper";
import { normalizeRolePermissions, rolePermissionsSchema } from "@/lib/settings/iam-schemas";

type RouteContext = { params: Promise<{ id: string }> };

async function getRoleOr404(id: string) {
  const row = await getIamRoleFromDb(id);
  if (!row) throw ApiError.notFound("Role not found");
  return row;
}

async function permissionsResponse(id: string, row: Awaited<ReturnType<typeof getRoleOr404>>) {
  const matrix = await listRolePermissionsMatrixFromDb(id);
  return NextResponse.json({ roleId: id, permissions: mapRoleDetail(row, matrix).permissions });
}

export const GET = apiHandler(async (_request: NextRequest, { params }: RouteContext) => {
  await requireIamMenuPrefix(IAM.settingsRoles);
  const { id } = await params;
  return permissionsResponse(id, await getRoleOr404(id));
}, "GET /api/settings/iam/roles/[id]/permissions");

export const PUT = apiHandler(async (request: NextRequest, { params }: RouteContext) => {
  const user = await requireIamMenuPrefix(IAM.settingsRoles);
  const { id } = await params;
  const body = await validateBody(request, rolePermissionsSchema);
  const row = await getRoleOr404(id);
  if (row.is_system) await requireSuperAdminForSystemRole(user);
  const permissions = normalizeRolePermissions(body.permissions);
  if (isHrisStaffRole(row.code)) await requireIamSuperAdmin(user, HRIS_ROLE_SUPER_ADMIN_ERROR);
  else if (await addsHrisDelete(id, permissions)) await requireIamSuperAdmin(user, HRIS_DELETE_SUPER_ADMIN_ERROR);
  // admin_hris adalah sumber role HR turunan: tidak boleh kosong, dan sync jalan di transaksi yang sama.
  const isAdminHris = row.code === "admin_hris";
  if (isAdminHris && permissions.length === 0) throw ApiError.badRequest(ADMIN_HRIS_EMPTY_ERROR);
  await replaceIamRolePermissionsInDb(id, permissions, user.id, { syncHrisStaffRoles: isAdminHris });
  return permissionsResponse(id, row);
}, "PUT /api/settings/iam/roles/[id]/permissions");
