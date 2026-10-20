import type { z } from "zod";
import { ApiError } from "@/lib/api/auth";
import { query } from "@/lib/db";
import { createPgClient } from "@/lib/pg/create-client";
import { syncIamPrimaryRole } from "@/lib/iam/sync-user-role";
import { resetUserEmployeePassword } from "@/lib/users/user-service";
import type { createAdminUserSchema, updateAdminUserSchema } from "@/lib/admin/user-management";

type PgClient = ReturnType<typeof createPgClient>;
type CreateInput = z.infer<typeof createAdminUserSchema>;
type UpdateInput = z.infer<typeof updateAdminUserSchema>;
type ApprovalPermission = CreateInput["approval_permissions"][number];

const BAN_FOREVER = "876000h";

type AuthStatusRow = {
  id: string;
  email: string;
  banned_until: string | null;
  last_sign_in_at: string | null;
  email_verified_at: string | null;
};

/** Status akun auth: "banned" selama banned_until masih di masa depan. */
export function authStatus(bannedUntil: string | null, now = new Date()): "banned" | "enabled" {
  return bannedUntil && new Date(bannedUntil) > now ? "banned" : "enabled";
}

async function audit(db: PgClient, actorId: string, targetUserId: string, action: string, details: unknown) {
  await db.from("admin_user_audit_logs").insert({ actor_id: actorId, target_user_id: targetUserId, action, details });
}

async function insertApprovalPermissions(db: PgClient, userId: string, actorId: string, permissions: ApprovalPermission[]) {
  const active = permissions.filter((permission) => permission.is_active);
  if (active.length === 0) return;
  const { error } = await db.from("user_approval_permissions").insert(
    active.map((permission) => ({
      user_id: userId,
      module: permission.module,
      workflow: permission.workflow,
      approval_level: permission.approval_level,
      approval_limit: permission.approval_limit ?? null,
      is_active: true,
      created_by: actorId,
      updated_by: actorId,
    }))
  );
  if (error) throw error;
}

/** Profil user + status akun auth (banned, login terakhir, verifikasi email). */
export async function listAdminUsers() {
  const db = createPgClient();
  const [{ data: profiles, error }, authRows] = await Promise.all([
    db
      .from("users")
      .select("id, full_name, email, role, brand_id, status, created_at, updated_at, brands(id, name), user_approval_permissions!user_id(*)")
      .order("created_at", { ascending: false }),
    query<AuthStatusRow>(
      `SELECT id, email, banned_until::text, last_sign_in_at::text, email_verified_at::text FROM auth.users`
    ),
  ]);
  if (error) throw error;

  const authById = new Map(authRows.map((row) => [row.id, row]));
  return ((profiles ?? []) as Array<Record<string, unknown> & { id: string; email?: string | null }>).map(
    (profile) => {
      const auth = authById.get(profile.id);
      return {
        ...profile,
        email: auth?.email ?? profile.email ?? "",
        auth_status: authStatus(auth?.banned_until ?? null),
        last_sign_in_at: auth?.last_sign_in_at ?? null,
        email_confirmed_at: auth?.email_verified_at ?? null,
      };
    }
  );
}

/** Buat akun auth + profil + izin approval; akun auth dihapus lagi bila langkah berikutnya gagal. */
export async function createAdminUser(actorId: string, body: CreateInput) {
  const db = createPgClient();
  const { data: authData, error: authError } = await db.auth.admin.createUser({
    email: body.email,
    password: body.password,
    email_confirm: true,
    user_metadata: { full_name: body.full_name },
    app_metadata: { role: body.role },
  });
  if (authError) throw ApiError.badRequest(authError.message);
  if (!authData.user) throw ApiError.server("Gagal membuat user auth");
  const userId = authData.user.id;

  try {
    const { data: profile, error: profileError } = await db
      .from("users")
      .insert({
        id: userId,
        full_name: body.full_name,
        email: body.email,
        role: body.role,
        brand_id: body.brand_id ?? null,
        status: body.status,
      })
      .select("id, full_name, email, role, brand_id, status, created_at")
      .single();
    if (profileError) throw profileError;

    if (body.status === "inactive") {
      await db.auth.admin.updateUserById(userId, { ban_duration: BAN_FOREVER });
    }
    await insertApprovalPermissions(db, userId, actorId, body.approval_permissions);
    // Role IAM mengikuti role aplikasi (sama dengan Go createAdmin dan Manajemen User).
    await syncIamPrimaryRole(userId, body.role, actorId);
    await audit(db, actorId, userId, "create_user", { email: body.email, role: body.role, status: body.status });
    return profile;
  } catch (error) {
    await db.auth.admin.deleteUser(userId);
    throw error;
  }
}

export async function updateAdminUser(actorId: string, id: string, body: UpdateInput) {
  if (actorId === id && (body.status === "inactive" || (body.role && body.role !== "super_admin"))) {
    throw ApiError.badRequest("Super admin tidak bisa menonaktifkan atau menurunkan role dirinya sendiri");
  }
  const db = createPgClient();
  const [previous] = body.role
    ? await query<{ role: string | null }>(`SELECT role FROM configuration.users WHERE id = $1::uuid`, [id])
    : [];

  const authUpdates: Parameters<PgClient["auth"]["admin"]["updateUserById"]>[1] = {};
  if (body.email) authUpdates.email = body.email;
  if (body.full_name) authUpdates.user_metadata = { full_name: body.full_name };
  if (body.role) authUpdates.app_metadata = { role: body.role };
  if (body.status) authUpdates.ban_duration = body.status === "inactive" ? BAN_FOREVER : "none";
  if (Object.keys(authUpdates).length > 0) {
    const { error } = await db.auth.admin.updateUserById(id, authUpdates);
    if (error) throw ApiError.badRequest(error.message);
  }

  const profileUpdates: Record<string, unknown> = { updated_at: new Date().toISOString() };
  if (body.email) profileUpdates.email = body.email;
  if (body.full_name) profileUpdates.full_name = body.full_name;
  if (body.role) profileUpdates.role = body.role;
  if (body.brand_id !== undefined) profileUpdates.brand_id = body.brand_id;
  if (body.status) profileUpdates.status = body.status;

  const { data: profile, error: profileError } = await db
    .from("users")
    .update(profileUpdates)
    .eq("id", id)
    .select("id, full_name, email, role, brand_id, status, created_at, updated_at")
    .single();
  if (profileError) throw profileError;

  // Ganti role memindahkan role IAM (penurunan benar-benar mencabut), hanya bila berubah — sama dengan Go updateAdmin.
  if (body.role && previous?.role !== body.role) {
    await syncIamPrimaryRole(id, body.role, actorId);
  }

  if (body.approval_permissions) {
    const { error } = await db
      .from("user_approval_permissions")
      .update({ is_active: false, updated_by: actorId, updated_at: new Date().toISOString() })
      .eq("user_id", id);
    if (error) throw error;
    await insertApprovalPermissions(db, id, actorId, body.approval_permissions);
  }

  await audit(db, actorId, id, "update_user", { fields: Object.keys(body), role: body.role, status: body.status });
  return profile;
}

/**
 * Reset password memakai password sementara (sama dengan /api/users/[id]/reset-password):
 * aplikasi tidak punya pengiriman email, jadi link reset tidak bisa dikirim.
 */
export async function resetAdminUserPassword(actorId: string, id: string) {
  const db = createPgClient();
  const { data, error } = await db.auth.admin.getUserById(id);
  if (error || !data.user?.email) throw ApiError.notFound("Email user tidak ditemukan");
  const result = await resetUserEmployeePassword(id);
  await audit(db, actorId, id, "reset_password", { email: data.user.email });
  return result;
}
