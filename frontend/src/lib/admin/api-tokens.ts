import { ApiError, requireIamMenuPrefix, type ApiUser } from "@/lib/api/auth";
import { getSessionUserFromCookies } from "@/lib/auth/session";
import { API_SCOPE_MODULES, isApiTokenSession, mintApiToken } from "@/lib/auth/api-token";
import { query, queryOne } from "@/lib/db";
import { canonicalUuid } from "@/lib/api/canonical-uuid";
import { IAM } from "@/lib/iam/prefixes";
import { requireIamSuperAdmin } from "@/lib/iam/system-role-guard";
import { accountHoldsSystemRole, systemRoleCodes } from "@/lib/users/system-account-guard";

/** Token berjalan sebagai akunnya: token untuk akun ber-role sistem hanya oleh Super Admin (Go apitokens). */
export const SYSTEM_ACCOUNT_TOKEN_ERROR = "Token API untuk akun Super Admin/Admin hanya bisa dibuat oleh Super Admin";

/**
 * EPIC-042: kelola Open API token hanya lewat sesi manusia (cookie) dengan menu
 * settings.integrations — token TIDAK bisa membuat/mencabut token (mencegah
 * agent memperbanyak aksesnya sendiri). Mengembalikan pemanggil.
 */
export async function requireHumanTokenAdmin(): Promise<ApiUser> {
  const sessionUser = await getSessionUserFromCookies();
  if (isApiTokenSession(sessionUser)) {
    throw ApiError.forbidden("Kelola token hanya lewat login dashboard, bukan token");
  }
  return requireIamMenuPrefix(IAM.settingsIntegrations);
}

/** '*' atau '<modul>:read|write' dengan modul dari API_SCOPE_MODULES. */
export function isValidScope(scope: string): boolean {
  if (scope === "*") return true;
  const [moduleKey, access] = scope.split(":");
  return (API_SCOPE_MODULES as readonly string[]).includes(moduleKey ?? "") && (access === "read" || access === "write");
}

/** Tabel belum dimigrasi (42P01). */
export function isMissingTable(error: unknown): boolean {
  return (error as { code?: unknown } | null)?.code === "42P01";
}

export function listApiTokens() {
  return query(
    `SELECT t.id, t.name, t.token_prefix, t.scopes, t.user_id,
            u.full_name AS user_name, t.created_at, t.expires_at,
            t.revoked_at, t.last_used_at
     FROM configuration.api_tokens t
     LEFT JOIN configuration.users u ON u.id = t.user_id
     ORDER BY t.created_at DESC`
  );
}

export interface ApiTokenInput {
  name?: unknown;
  scopes?: unknown;
  user_id?: unknown;
  expires_in_days?: unknown;
}

/** Buat token; nilai token mentah hanya dikembalikan sekali (DB menyimpan hash). */
export async function createApiToken(admin: ApiUser | null, body: ApiTokenInput) {
  const adminId = admin?.id ?? null;
  const name = String(body.name || "").trim();
  if (!name) throw ApiError.badRequest("Nama token wajib diisi");
  const scopes = Array.isArray(body.scopes) ? body.scopes.map(String) : [];
  if (scopes.length === 0 || !scopes.every(isValidScope)) {
    throw ApiError.badRequest(
      `Scope tidak valid. Pakai '*' atau '<modul>:read|write' (modul: ${API_SCOPE_MODULES.join(", ")})`
    );
  }

  // Token berjalan sebagai akun ini — default: admin yang membuatnya.
  // Dikanonikkan sebagai uuid sebelum guard & insert (temuan re-review EPIC-061).
  const userId = canonicalUuid(String(body.user_id || adminId || ""));
  if (!userId) throw ApiError.badRequest("user_id tidak valid");
  const userRow = await queryOne<{ id: string }>(`SELECT id FROM configuration.users WHERE id = $1`, [userId]);
  if (!userRow) throw ApiError.badRequest("user_id tidak dikenal");
  // Token untuk akun sendiri tidak menambah kuasa; akun lain ber-role sistem hanya oleh Super Admin.
  if (userId !== adminId && (await accountHoldsSystemRole(userId, await systemRoleCodes()))) {
    if (!admin) throw ApiError.forbidden(SYSTEM_ACCOUNT_TOKEN_ERROR);
    await requireIamSuperAdmin(admin, SYSTEM_ACCOUNT_TOKEN_ERROR);
  }

  const days = Number(body.expires_in_days);
  const expiresAt = days > 0 ? new Date(Date.now() + days * 86_400_000).toISOString() : null;

  const minted = mintApiToken();
  const rows = await query(
    `INSERT INTO configuration.api_tokens
       (name, token_hash, token_prefix, user_id, scopes, created_by, expires_at)
     VALUES ($1, $2, $3, $4, $5, $6, $7)
     RETURNING id, name, token_prefix, scopes, user_id, created_at, expires_at`,
    [name, minted.hash, minted.prefix, userId, scopes, adminId, expiresAt]
  );
  return { ...rows[0], token: minted.token };
}

/** Soft revoke — jejak audit tetap utuh. */
export async function revokeApiToken(id: string) {
  const rows = await query(
    `UPDATE configuration.api_tokens
     SET revoked_at = now()
     WHERE id = $1 AND revoked_at IS NULL
     RETURNING id, name`,
    [id]
  );
  if (rows.length === 0) throw ApiError.notFound("Token tidak ditemukan atau sudah dicabut");
  return rows[0];
}
