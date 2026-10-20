import { NextRequest, NextResponse } from "next/server";
import { z } from "zod";
import { ApiError, validateBody, requireIamMenuPrefix } from "@/lib/api/auth";
import { IAM } from "@/lib/iam/prefixes";
import { createSession, isImpersonationSession, setSessionCookie } from "@/lib/auth/session";
import { readSessionToken } from "@/lib/auth/constants";
import { queryOne } from "@/lib/db";
import { requireIamSuperAdmin } from "@/lib/iam/system-role-guard";
import { accountHoldsSystemRole, systemRoleCodes } from "@/lib/users/system-account-guard";

const SYSTEM_IMPERSONATE_ERROR = "Login As ke akun Super Admin/Admin hanya untuk Super Admin";
const TOKEN_IMPERSONATE_ERROR = "Login As hanya dari sesi login biasa";
const NESTED_IMPERSONATE_ERROR = "Tidak bisa Login As dari sesi Login As; keluar lalu masuk dengan akun sendiri";

const impersonateSchema = z.object({
  user_id: z.string().uuid(),
});

/**
 * Login As (impersonation) — pemegang menu Manajemen User. Mengganti session
 * cookie dengan session baru milik user target. Kembali ke akun sendiri dengan
 * logout lalu login ulang. Akun ber-role sistem (super_admin/admin sebagai role
 * aplikasi atau IAM) hanya oleh pemegang role IAM super_admin; hanya dari sesi
 * login biasa (bukan token API) dan tidak dari dalam sesi Login As — sama dengan Go identity.Impersonate (temuan EPIC-061).
 */
export async function POST(request: NextRequest) {
  try {
    const admin = await requireIamMenuPrefix(IAM.settingsUsers);
    const { user_id } = await validateBody(request, impersonateSchema);

    // Tanpa cookie sesi, pemanggil masuk lewat token API: token tidak pernah jadi sesi.
    const currentToken = readSessionToken(request.cookies);
    if (!currentToken) throw ApiError.forbidden(TOKEN_IMPERSONATE_ERROR);
    if (await isImpersonationSession(currentToken)) {
      throw ApiError.forbidden(NESTED_IMPERSONATE_ERROR);
    }

    if (user_id === admin.id) {
      throw ApiError.badRequest("You are already logged in as this account");
    }

    const target = await queryOne<{
      id: string;
      email: string;
      role: string;
      full_name: string;
      is_banned: boolean;
    }>(
      `SELECT au.id, au.email, cu.role, cu.full_name,
              (au.banned_until IS NOT NULL AND au.banned_until > NOW()) AS is_banned
       FROM auth.users au
       JOIN configuration.users cu ON cu.id = au.id
       WHERE au.id = $1`,
      [user_id]
    );

    if (!target) throw ApiError.notFound("User account not found");
    if (target.is_banned) throw ApiError.badRequest("User account is banned");
    if (target.role === "super_admin") {
      throw ApiError.forbidden("Cannot impersonate a super admin account");
    }
    if (await accountHoldsSystemRole(target.id, await systemRoleCodes())) {
      await requireIamSuperAdmin(admin, SYSTEM_IMPERSONATE_ERROR);
    }

    const { token, expiresAt } = await createSession(target.id, {
      userAgent: `impersonated-by:${admin.id} ${request.headers.get("user-agent") ?? ""}`.trim(),
    });

    const response = NextResponse.json({
      success: true,
      data: { id: target.id, email: target.email, role: target.role, full_name: target.full_name },
      message: `Logged in as ${target.full_name}`,
    });
    setSessionCookie(response, token, expiresAt, request);
    return response;
  } catch (error) {
    if (error instanceof ApiError) return error.toResponse();
    const message = error instanceof Error ? error.message : "Failed to impersonate user";
    console.error("[api/auth/impersonate] failed:", error);
    return NextResponse.json({ error: message }, { status: 500 });
  }
}
