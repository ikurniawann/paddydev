import crypto from "crypto";
import { cookies } from "next/headers";
import type { NextRequest, NextResponse } from "next/server";
import { query, queryOne } from "@/lib/db";
import {
  LEGACY_SESSION_COOKIE,
  SESSION_COOKIE,
  SESSION_TTL_DAYS,
  readSessionToken,
} from "@/lib/auth/constants";
import { extractBearerToken } from "@/lib/auth/api-token-format";
import { isSecureRequest } from "@/lib/auth/secure-cookie";

export interface SessionUser {
  id: string;
  email: string;
  user_metadata?: Record<string, unknown>;
  app_metadata?: Record<string, unknown>;
}

function hashToken(token: string) {
  return crypto.createHash("sha256").update(token).digest("hex");
}

function newToken() {
  return crypto.randomBytes(32).toString("hex");
}

export function sessionCookieOptions(expires: Date, secure: boolean) {
  return {
    httpOnly: true,
    secure,
    sameSite: "lax" as const,
    path: "/",
    expires,
  };
}

export async function createSession(
  userId: string,
  meta?: { userAgent?: string; ip?: string }
): Promise<{ token: string; expiresAt: Date }> {
  const token = newToken();
  const expiresAt = new Date(Date.now() + SESSION_TTL_DAYS * 24 * 60 * 60 * 1000);
  await query(
    `INSERT INTO auth.sessions (user_id, token_hash, user_agent, ip_address, expires_at)
     VALUES ($1, $2, $3, $4, $5)`,
    [userId, hashToken(token), meta?.userAgent ?? null, meta?.ip ?? null, expiresAt.toISOString()]
  );
  await query(`UPDATE auth.users SET last_sign_in_at = NOW() WHERE id = $1`, [userId]);
  return { token, expiresAt };
}

/** Sesi dibuat oleh Login As (user agent "impersonated-by:<id> <ua>"). */
export async function isImpersonationSession(token: string): Promise<boolean> {
  const row = await queryOne<{ nested: boolean }>(
    `SELECT EXISTS (SELECT 1 FROM auth.sessions WHERE token_hash = $1 AND user_agent LIKE 'impersonated-by:%') AS nested`,
    [hashToken(token)]
  );
  return Boolean(row?.nested);
}

export async function destroySession(token: string | undefined | null) {
  if (!token) return;
  await query(`DELETE FROM auth.sessions WHERE token_hash = $1`, [hashToken(token)]);
}

export async function destroyAllUserSessions(userId: string) {
  await query(`DELETE FROM auth.sessions WHERE user_id = $1`, [userId]);
}

async function loadUserBySessionToken(token: string): Promise<SessionUser | null> {
  const row = await queryOne<{
    id: string;
    email: string;
    raw_user_meta_data: Record<string, unknown>;
    raw_app_meta_data: Record<string, unknown>;
  }>(
    `SELECT u.id, u.email, u.raw_user_meta_data, u.raw_app_meta_data
     FROM auth.sessions s
     JOIN auth.users u ON u.id = s.user_id
     WHERE s.token_hash = $1
       AND s.expires_at > NOW()
       AND (u.banned_until IS NULL OR u.banned_until < NOW())`,
    [hashToken(token)]
  );
  if (!row) return null;
  return {
    id: row.id,
    email: row.email,
    user_metadata: row.raw_user_meta_data ?? {},
    app_metadata: row.raw_app_meta_data ?? {},
  };
}

/**
 * Validasi cepat token sesi cookie ke DB (audit 2026-09-17): dipakai middleware
 * agar cookie sampah tidak lolos hanya karena "ada". Query 1 baris pada indeks
 * unik token_hash + cek kedaluwarsa/ban. Mengembalikan boolean saja — tidak
 * memuat data user, jadi ringan untuk dijalankan tiap request terproteksi.
 */
export async function sessionTokenIsValid(token: string): Promise<boolean> {
  const row = await queryOne<{ one: number }>(
    `SELECT 1 AS one
     FROM auth.sessions s
     JOIN auth.users u ON u.id = s.user_id
     WHERE s.token_hash = $1
       AND s.expires_at > NOW()
       AND (u.banned_until IS NULL OR u.banned_until < NOW())`,
    [hashToken(token)]
  );
  return Boolean(row);
}

export async function getSessionUserFromRequest(request: NextRequest) {
  const token = readSessionToken(request.cookies);
  if (token) return loadUserBySessionToken(token);
  // EPIC-042: fallback Bearer token (Open API) — path+method langsung dari
  // request, scope dicek di dalam loadUserByApiToken.
  const bearer = extractBearerToken(request.headers.get("authorization"));
  if (!bearer) return null;
  const { loadUserByApiToken } = await import("@/lib/auth/api-token");
  // audit=false: audit kanonik sudah terjadi di middleware.
  return loadUserByApiToken(bearer, {
    pathname: request.nextUrl.pathname,
    method: request.method,
  });
}

export async function getSessionUserFromCookies() {
  const token = readSessionToken(await cookies());
  if (token) return loadUserBySessionToken(token);
  // EPIC-042: fallback Bearer token. Route handler tidak tahu path/method-nya
  // sendiri — proxy (middleware) menyuntik x-pathname + x-request-method.
  const { headers } = await import("next/headers");
  const hdrs = await headers();
  const bearer = extractBearerToken(hdrs.get("authorization"));
  if (!bearer) return null;
  const { loadUserByApiToken } = await import("@/lib/auth/api-token");
  return loadUserByApiToken(bearer, {
    pathname: hdrs.get("x-pathname") || "/api",
    method: hdrs.get("x-request-method") || "POST",
  });
}

function expireCookie(response: NextResponse, name: string, request: Request) {
  response.cookies.set(name, "", {
    ...sessionCookieOptions(new Date(0), isSecureRequest(request)),
    maxAge: 0,
  });
}

/** Tulis cookie sesi baru; cookie nama lama ikut dihapus agar tidak tersisa. */
export function setSessionCookie(
  response: NextResponse,
  token: string,
  expiresAt: Date,
  request: Request
) {
  response.cookies.set(
    SESSION_COOKIE,
    token,
    sessionCookieOptions(expiresAt, isSecureRequest(request))
  );
  expireCookie(response, LEGACY_SESSION_COOKIE, request);
}

export function clearSessionCookie(response: NextResponse, request: Request) {
  expireCookie(response, SESSION_COOKIE, request);
  expireCookie(response, LEGACY_SESSION_COOKIE, request);
}

export async function authenticateCredentials(email: string, password: string) {
  const { verifyPassword } = await import("@/lib/auth/password");
  const row = await queryOne<{
    id: string;
    email: string;
    password_hash: string;
    banned_until: string | null;
  }>(
    `SELECT id, email, password_hash, banned_until
     FROM auth.users WHERE lower(email) = lower($1)`,
    [email.trim()]
  );
  if (!row) return { user: null, error: { message: "Invalid login credentials" } };
  const ok = await verifyPassword(password, row.password_hash);
  if (!ok) return { user: null, error: { message: "Invalid login credentials" } };
  if (row.banned_until && new Date(row.banned_until) > new Date()) {
    return { user: null, error: { message: "This account is inactive. Contact your administrator." } };
  }
  return { user: { id: row.id, email: row.email }, error: null };
}
