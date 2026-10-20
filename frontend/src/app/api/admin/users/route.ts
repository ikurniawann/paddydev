import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix, validateBody } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { createAdminUser, listAdminUsers } from "@/lib/admin/admin-users";
import { createAdminUserSchema } from "@/lib/admin/user-management";
import { requireSuperAdminForSystemAccount } from "@/lib/users/system-account-guard";

export const GET = apiHandler(async () => {
  await requireIamMenuPrefix(IAM.settingsUsers);
  return NextResponse.json({ data: await listAdminUsers() });
}, "GET /api/admin/users");

export const POST = apiHandler(async (request: NextRequest) => {
  const actor = await requireIamMenuPrefix(IAM.settingsUsers);
  const body = await validateBody(request, createAdminUserSchema);
  await requireSuperAdminForSystemAccount(actor, { role: body.role });
  const profile = await createAdminUser(actor.id, body);
  return NextResponse.json({ data: profile, message: "User berhasil dibuat" }, { status: 201 });
}, "POST /api/admin/users");
