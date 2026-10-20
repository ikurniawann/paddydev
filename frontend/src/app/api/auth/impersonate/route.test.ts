// @vitest-environment node
// Login As (fallback Next): akun ber-role sistem hanya untuk Super Admin, dan tidak
// dari dalam sesi Login As — sama dengan Go identity.Impersonate (temuan EPIC-061).
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";

const TARGET = "22222222-2222-4222-8222-222222222222";
const SYSTEM_MSG = "Login As ke akun Super Admin/Admin hanya untuk Super Admin";
const NESTED_MSG = "Tidak bisa Login As dari sesi Login As; keluar lalu masuk dengan akun sendiri";

const mocks = vi.hoisted(() => ({
  requireIamMenuPrefix: vi.fn(),
  queryOne: vi.fn(),
  createSession: vi.fn(),
  isImpersonationSession: vi.fn(),
  iamDbQuery: vi.fn(),
  iamDbQueryOne: vi.fn(),
  resolveRoleIds: vi.fn(),
  iam: { superAdmin: false, holdsSystem: false },
}));

vi.mock("@/lib/api/auth", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/auth")>()),
  requireIamMenuPrefix: mocks.requireIamMenuPrefix,
}));
vi.mock("@/lib/db", () => ({ queryOne: mocks.queryOne, query: vi.fn() }));
vi.mock("@/lib/auth/session", () => ({
  createSession: mocks.createSession,
  setSessionCookie: vi.fn(),
  isImpersonationSession: mocks.isImpersonationSession,
}));
vi.mock("@/lib/iam/pg-client", () => ({
  iamDbQuery: mocks.iamDbQuery,
  iamDbQueryOne: mocks.iamDbQueryOne,
}));
vi.mock("@/lib/iam/get-user-menus", async (orig) => ({
  ...(await orig<typeof import("@/lib/iam/get-user-menus")>()),
  resolveRoleIds: mocks.resolveRoleIds,
}));

import { POST } from "./route";

const post = (cookie = "nuhabit_session=tok") =>
  POST(
    new NextRequest("https://dashboard.test/api/auth/impersonate", {
      method: "POST",
      headers: { "Content-Type": "application/json", cookie },
      body: JSON.stringify({ user_id: TARGET }),
    })
  );

beforeEach(() => {
  vi.clearAllMocks();
  mocks.iam.superAdmin = false;
  mocks.iam.holdsSystem = false;
  mocks.requireIamMenuPrefix.mockResolvedValue({ id: "actor-1", role: "hrd" });
  mocks.queryOne.mockResolvedValue({ id: TARGET, email: "t@x", role: "admin", full_name: "Target", is_banned: false });
  mocks.createSession.mockResolvedValue({ token: "new", expiresAt: new Date(Date.now() + 1000) });
  mocks.isImpersonationSession.mockResolvedValue(false);
  mocks.resolveRoleIds.mockResolvedValue(["role-1"]);
  mocks.iamDbQuery.mockResolvedValue([{ code: "super_admin" }, { code: "admin" }]);
  mocks.iamDbQueryOne.mockImplementation(async (sql: string) => {
    if (sql.includes("AS held")) return { held: mocks.iam.holdsSystem };
    if (sql.includes("code = 'super_admin'")) return { ok: mocks.iam.superAdmin };
    return null;
  });
});

describe("POST /api/auth/impersonate", () => {
  it("akun ber-role sistem oleh non-super_admin → 403, tanpa sesi", async () => {
    mocks.iam.holdsSystem = true;
    const res = await post();
    expect(res.status).toBe(403);
    expect(await res.json()).toEqual({ success: false, error: SYSTEM_MSG });
    expect(mocks.createSession).not.toHaveBeenCalled();
  });

  it("akun ber-role sistem oleh super_admin → 200", async () => {
    mocks.iam.holdsSystem = true;
    mocks.iam.superAdmin = true;
    expect((await post()).status).toBe(200);
    expect(mocks.createSession).toHaveBeenCalledTimes(1);
  });

  it("akun biasa → 200 tanpa cek super_admin", async () => {
    mocks.queryOne.mockResolvedValue({ id: TARGET, email: "t@x", role: "pos", full_name: "Kasir", is_banned: false });
    expect((await post()).status).toBe(200);
    expect(mocks.resolveRoleIds).not.toHaveBeenCalled();
  });

  it("dari dalam sesi Login As → 403", async () => {
    mocks.isImpersonationSession.mockResolvedValue(true);
    const res = await post();
    expect(res.status).toBe(403);
    expect(await res.json()).toEqual({ success: false, error: NESTED_MSG });
    expect(mocks.isImpersonationSession).toHaveBeenCalledWith("tok");
    expect(mocks.createSession).not.toHaveBeenCalled();
  });
});
