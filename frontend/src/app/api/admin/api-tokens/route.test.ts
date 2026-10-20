import { beforeEach, describe, expect, it, vi } from "vitest";
import type { NextRequest } from "next/server";

const requireIamMenuPrefix = vi.fn();
vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return { ...actual, requireIamMenuPrefix: (...args: unknown[]) => requireIamMenuPrefix(...args) };
});

let sessionUser: { id: string; via?: string } | null = { id: "admin-1" };
vi.mock("@/lib/auth/session", () => ({ getSessionUserFromCookies: async () => sessionUser }));
vi.mock("@/lib/auth/api-token", () => ({
  API_SCOPE_MODULES: ["pos", "crm"],
  isApiTokenSession: (user: { via?: string } | null) => user?.via === "token",
  mintApiToken: () => ({ token: "nh_secret", hash: "hash", prefix: "nh_sec" }),
}));

const query = vi.fn();
const queryOne = vi.fn();
vi.mock("@/lib/db", () => ({
  query: (...args: unknown[]) => query(...args),
  queryOne: (...args: unknown[]) => queryOne(...args),
}));

const iamDbQuery = vi.fn();
const iamDbQueryOne = vi.fn();
const resolveRoleIds = vi.fn();
const iam = { superAdmin: false, holdsSystem: false };
vi.mock("@/lib/iam/pg-client", () => ({
  iamDbQuery: (...a: unknown[]) => iamDbQuery(...a),
  iamDbQueryOne: (...a: unknown[]) => iamDbQueryOne(...a),
}));
vi.mock("@/lib/iam/get-user-menus", async (orig) => ({
  ...(await orig<typeof import("@/lib/iam/get-user-menus")>()),
  resolveRoleIds: (...a: unknown[]) => resolveRoleIds(...a),
}));

const SYSTEM_TOKEN_MSG = "Token API untuk akun Super Admin/Admin hanya bisa dibuat oleh Super Admin";
const TARGET = "00000000-0000-4000-8000-0000000000aa";

function post(body: unknown): NextRequest {
  return { json: async () => body } as unknown as NextRequest;
}

beforeEach(() => {
  vi.clearAllMocks();
  sessionUser = { id: "admin-1" };
  requireIamMenuPrefix.mockResolvedValue({ id: "admin-1", role: "hrd" });
  iam.superAdmin = false;
  iam.holdsSystem = false;
  resolveRoleIds.mockResolvedValue(["role-1"]);
  iamDbQuery.mockResolvedValue([{ code: "super_admin" }, { code: "admin" }]);
  iamDbQueryOne.mockImplementation(async (sql: string) => {
    if (sql.includes("AS held")) return { held: iam.holdsSystem };
    if (sql.includes("code = 'super_admin'")) return { ok: iam.superAdmin };
    return null;
  });
});

describe("/api/admin/api-tokens", () => {
  it("sesi token ditolak 403 sebelum cek menu", async () => {
    sessionUser = { id: "bot", via: "token" };
    const { GET } = await import("./route");
    const res = await GET();
    expect(res.status).toBe(403);
    expect((await res.json()).error).toBe("Kelola token hanya lewat login dashboard, bukan token");
    expect(requireIamMenuPrefix).not.toHaveBeenCalled();
  });

  it("tabel belum dimigrasi → daftar kosong + migration_pending", async () => {
    query.mockRejectedValue({ code: "42P01" });
    const { GET } = await import("./route");
    expect(await (await GET()).json()).toEqual({ success: true, data: [], migration_pending: true });
  });

  it("scope tidak valid → 400", async () => {
    const { POST } = await import("./route");
    const res = await POST(post({ name: "Agent", scopes: ["hr:delete"] }));
    expect(res.status).toBe(400);
    expect((await res.json()).error).toMatch(/Scope tidak valid/);
  });

  it("201 dengan token mentah sekali, user default = admin", async () => {
    queryOne.mockResolvedValue({ id: "admin-1" });
    query.mockResolvedValue([{ id: "t-1", name: "Agent" }]);
    const { POST } = await import("./route");
    const res = await POST(post({ name: " Agent ", scopes: ["pos:read", "*"], expires_in_days: 0 }));
    expect(res.status).toBe(201);
    expect(await res.json()).toEqual({ success: true, data: { id: "t-1", name: "Agent", token: "nh_secret" } });
    expect(query.mock.calls[0][1]).toEqual(["Agent", "hash", "nh_sec", "admin-1", ["pos:read", "*"], "admin-1", null]);
  });

  it("token untuk akun ber-role sistem oleh non-super_admin → 403, tanpa insert", async () => {
    iam.holdsSystem = true;
    queryOne.mockResolvedValue({ id: TARGET });
    const { POST } = await import("./route");
    const res = await POST(post({ name: "Agent", scopes: ["*"], user_id: TARGET }));
    expect(res.status).toBe(403);
    expect((await res.json()).error).toBe(SYSTEM_TOKEN_MSG);
    expect(query).not.toHaveBeenCalled();
  });

  it("token untuk akun ber-role sistem oleh super_admin → 201", async () => {
    iam.holdsSystem = true;
    iam.superAdmin = true;
    queryOne.mockResolvedValue({ id: TARGET });
    query.mockResolvedValue([{ id: "t-2", name: "Agent" }]);
    const { POST } = await import("./route");
    expect((await POST(post({ name: "Agent", scopes: ["*"], user_id: TARGET }))).status).toBe(201);
  });

  it("token untuk akun biasa → 201 tanpa cek super_admin", async () => {
    queryOne.mockResolvedValue({ id: TARGET });
    query.mockResolvedValue([{ id: "t-3", name: "Agent" }]);
    const { POST } = await import("./route");
    expect((await POST(post({ name: "Agent", scopes: ["*"], user_id: TARGET }))).status).toBe(201);
    expect(resolveRoleIds).not.toHaveBeenCalled();
  });
});
