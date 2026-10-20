import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix, validateBody } from "@/lib/api/auth";
import { requireCanonicalId } from "@/lib/api/canonical-uuid";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { updateAdminUser } from "@/lib/admin/admin-users";
import { updateAdminUserSchema } from "@/lib/admin/user-management";
import { requireSuperAdminForSystemAccount } from "@/lib/users/system-account-guard";

export const PATCH = apiHandler(
  async (request: NextRequest, { params }: { params: Promise<{ id: string }> }) => {
    const id = requireCanonicalId((await params).id);
    const actor = await requireIamMenuPrefix(IAM.settingsUsers);
    const body = await validateBody(request, updateAdminUserSchema);
    await requireSuperAdminForSystemAccount(actor, { role: body.role, userId: id });
    const profile = await updateAdminUser(actor.id, id, body);
    return NextResponse.json({ data: profile, message: "User berhasil diperbarui" });
  },
  "PATCH /api/admin/users/[id]"
);
