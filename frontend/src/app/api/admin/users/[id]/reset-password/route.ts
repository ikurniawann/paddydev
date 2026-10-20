import { NextRequest, NextResponse } from "next/server";
import { requireIamMenuPrefix } from "@/lib/api/auth";
import { apiHandler } from "@/lib/api/handler";
import { IAM } from "@/lib/iam/prefixes";
import { resetAdminUserPassword } from "@/lib/admin/admin-users";
import { requireSuperAdminForSystemAccount } from "@/lib/users/system-account-guard";

/** Reset ke password sementara; nilai sementara hanya ada di respons ini. */
export const POST = apiHandler(
  async (_request: NextRequest, { params }: { params: Promise<{ id: string }> }) => {
    const { id } = await params;
    const actor = await requireIamMenuPrefix(IAM.settingsUsers);
    await requireSuperAdminForSystemAccount(actor, { userId: id });
    const { tempPassword } = await resetAdminUserPassword(actor.id, id);
    return NextResponse.json({ message: "Password sementara berhasil dibuat", tempPassword });
  },
  "POST /api/admin/users/[id]/reset-password"
);
