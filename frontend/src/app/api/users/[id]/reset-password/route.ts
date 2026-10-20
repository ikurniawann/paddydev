import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { requireCanonicalId } from "@/lib/api/canonical-uuid";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { resetEmployeeAppPassword } from "@/lib/hris/users-app-password";
import { employeeUserId, requireSuperAdminForSystemAccount } from "@/lib/users/system-account-guard";

interface RouteParams {
  params: Promise<{ id: string }>;
}

export const POST = apiHandler(async (_request: NextRequest, { params }: RouteParams) => {
  const actor = await requireIamMenuPrefix(IAM.settingsUsers);
  const id = requireCanonicalId((await params).id);
  await requireSuperAdminForSystemAccount(actor, { userId: await employeeUserId(id) });
  const { message, tempPassword } = await resetEmployeeAppPassword(id);
  return NextResponse.json({ message, tempPassword });
}, "api/users/:id/reset-password");
