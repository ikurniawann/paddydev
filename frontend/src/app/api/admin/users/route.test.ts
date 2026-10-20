// /api/admin/users (fallback Next): role sistem hanya Super Admin (temuan lanjutan EPIC-061).
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { NextRequest } from "next/server";

const requireIamMenuPrefix = vi.fn();
const service = {
  listAdminUsers: vi.fn(),
  createAdminUser: vi.fn(),
  updateAdminUser: vi.fn(),
  resetAdminUserPassword: vi.fn(),
};
const iamDbQuery = vi.fn();
const iamDbQueryOne = vi.fn();
const resolveRoleIds = vi.fn();
const iam = { superAdmin: false, holdsSystem: false };

vi.mock("@/lib/api/auth", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/auth")>()),
  requireIamMenuPrefix: (...a: unknown[]) => requireIamMenuPrefix(...a),
}));
vi.mock("@/lib/admin/admin-users", () => service);
vi.mock("@/lib/iam/pg-client", () => ({
  iamDbQuery: (...a: unknown[]) => iamDbQuery(...a),
  iamDbQueryOne: (...a: unknown[]) => iamDbQueryOne(...a),
}));
vi.mock("@/lib/iam/get-user-menus", async (orig) => ({
  ...(await orig<typeof import("@/lib/iam/get-user-menus")>()),
  resolveRoleIds: (...a: unknown[]) => resolveRoleIds(...a),
}));

const ID = "00000000-0000-4000-8000-000000000001";
const ctx = { params: Promise.resolve({ id: ID }) };
const SYSTEM_MSG = "Role Super Admin/Admin hanya bisa diatur oleh Super Admin";
const req = (method: string, body?: unknown) =>
  new Request("http://x/api/admin/users", {
    method,
    body: body === undefined ? undefined : JSON.stringify(body),
  }) as unknown as NextRequest;
const newAdmin = (role: string) => ({
  email: "baru@example.com",
  password: "rahasia123",
  full_name: "Admin Baru",
  role,
});
const forbidden = async (res: Response) => {
  expect(res.status).toBe(403);
  expect(await res.json()).toEqual({ success: false, error: SYSTEM_MSG });
};

beforeEach(() => {
  Object.values(service).forEach((fn) => fn.mockReset());
  requireIamMenuPrefix.mockReset().mockResolvedValue({ id: "actor-1", role: "hrd" });
  iam.superAdmin = false;
  iam.holdsSystem = false;
  resolveRoleIds.mockReset().mockResolvedValue(["role-1"]);
  iamDbQuery.mockReset().mockResolvedValue([{ code: "super_admin" }, { code: "admin" }]);
  iamDbQueryOne.mockReset().mockImplementation(async (sql: string) => {
    if (sql.includes("AS held")) return { held: iam.holdsSystem };
    if (sql.includes("code = 'super_admin'")) return { ok: iam.superAdmin };
    return null;
  });
});

describe("POST /api/admin/users", () => {
  it.each(["super_admin", "admin"])("role %s tanpa super_admin → 403, tidak menulis", async (role) => {
    const { POST } = await import("./route");
    await forbidden(await POST(req("POST", newAdmin(role))));
    expect(service.createAdminUser).not.toHaveBeenCalled();
  });

  it("role admin oleh super_admin → 201", async () => {
    iam.superAdmin = true;
    service.createAdminUser.mockResolvedValue({ id: ID });
    const { POST } = await import("./route");
    expect((await POST(req("POST", newAdmin("admin")))).status).toBe(201);
  });

  it("role biasa → 201 tanpa cek super_admin", async () => {
    service.createAdminUser.mockResolvedValue({ id: ID });
    const { POST } = await import("./route");
    expect((await POST(req("POST", newAdmin("hrd")))).status).toBe(201);
    expect(resolveRoleIds).not.toHaveBeenCalled();
  });
});

describe("PATCH /api/admin/users/[id]", () => {
  it("mempromosikan ke super_admin → 403", async () => {
    const { PATCH } = await import("./[id]/route");
    await forbidden(await PATCH(req("PATCH", { role: "super_admin" }), ctx));
    expect(service.updateAdminUser).not.toHaveBeenCalled();
  });

  it.each([
    ["menurunkan role", { role: "hrd" }],
    ["menonaktifkan", { status: "inactive" }],
    ["mengganti nama", { full_name: "Nama Lain" }],
  ])("%s akun ber-role sistem → 403", async (_name, body) => {
    iam.holdsSystem = true;
    const { PATCH } = await import("./[id]/route");
    await forbidden(await PATCH(req("PATCH", body), ctx));
    expect(service.updateAdminUser).not.toHaveBeenCalled();
  });

  it("akun biasa → 200", async () => {
    service.updateAdminUser.mockResolvedValue({ id: ID });
    const { PATCH } = await import("./[id]/route");
    expect((await PATCH(req("PATCH", { role: "finance_staff" }), ctx)).status).toBe(200);
  });

  it("akun ber-role sistem oleh super_admin → 200", async () => {
    iam.holdsSystem = true;
    iam.superAdmin = true;
    service.updateAdminUser.mockResolvedValue({ id: ID });
    const { PATCH } = await import("./[id]/route");
    expect((await PATCH(req("PATCH", { role: "hrd" }), ctx)).status).toBe(200);
  });
});

describe("POST /api/admin/users/[id]/reset-password", () => {
  it("akun ber-role sistem → 403", async () => {
    iam.holdsSystem = true;
    const { POST } = await import("./[id]/reset-password/route");
    await forbidden(await POST(req("POST"), ctx));
    expect(service.resetAdminUserPassword).not.toHaveBeenCalled();
  });

  it("akun biasa → temp password", async () => {
    service.resetAdminUserPassword.mockResolvedValue({ tempPassword: "ArkivX!" });
    const { POST } = await import("./[id]/reset-password/route");
    const res = await POST(req("POST"), ctx);
    expect(await res.json()).toEqual({ message: "Password sementara berhasil dibuat", tempPassword: "ArkivX!" });
  });
});
