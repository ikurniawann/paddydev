// Fallback Next untuk Role Management mengikuti Go (EPIC-061, R10): role sistem
// hanya Super Admin; admin_hris tidak boleh kosong dan menjalankan sync HR.
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { NextRequest } from "next/server";

const h = vi.hoisted(() => ({
  role: { id: "r1", code: "go_sys", is_system: true } as Record<string, unknown>,
  superAdmin: false,
  replace: vi.fn(async () => {}),
  update: vi.fn(async () => true),
}));

vi.mock("@/lib/api/auth", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/auth")>()),
  requireIamMenuPrefix: vi.fn(async () => ({ id: "u1", full_name: "Admin", role: "admin", brand_id: null })),
}));
vi.mock("@/lib/iam/get-user-menus", () => ({ resolveRoleIds: vi.fn(async () => ["role-of-caller"]) }));
vi.mock("@/lib/iam/pg-client", () => ({ iamDbQueryOne: vi.fn(async () => ({ ok: h.superAdmin })) }));
vi.mock("@/lib/iam/role-repository", () => ({
  getIamRoleFromDb: vi.fn(async () => h.role),
  listRolePermissionsMatrixFromDb: vi.fn(async () => []),
  updateIamRoleInDb: h.update,
  replaceIamRolePermissionsInDb: h.replace,
  deleteIamRoleInDb: vi.fn(),
}));

const ID = "00000000-0000-4000-8000-000000000001";
const MENU = "00000000-0000-4000-8000-000000000002";
const ctx = { params: Promise.resolve({ id: ID }) };
const put = (body: unknown) => new Request("http://x/api", { method: "PUT", body: JSON.stringify(body) }) as unknown as NextRequest;
const SYSTEM_ERR = "Role sistem hanya bisa diubah oleh Super Admin";
const EMPTY_ERR = "admin_hris harus punya minimal satu menu: role HR lain diturunkan darinya";
const grants = { permissions: [{ menuId: MENU, isGranted: true, grantedActions: ["read"] }] };

beforeEach(() => {
  h.role = { id: ID, code: "go_sys", name: "Go", description: null, is_system: true, is_active: true, created_at: "x", updated_at: null, version: 1 };
  h.superAdmin = false;
  h.replace.mockClear();
  h.update.mockClear();
});

describe("PUT /api/settings/iam/roles/[id]", () => {
  it("bukan super_admin pada role sistem → 403", async () => {
    const { PUT } = await import("./[id]/route");
    const res = await PUT(put({ name: "Ubah" }), ctx);
    expect(res.status).toBe(403);
    expect((await res.json()).error).toBe(SYSTEM_ERR);
    expect(h.update).not.toHaveBeenCalled();
  });

  it("super_admin pada role sistem → boleh", async () => {
    h.superAdmin = true;
    const { PUT } = await import("./[id]/route");
    const res = await PUT(put({ name: "Ubah" }), ctx);
    expect(res.status).toBe(200);
    expect(h.update).toHaveBeenCalled();
  });

  it("role bukan sistem tetap boleh untuk admin biasa", async () => {
    h.role = { ...h.role, is_system: false };
    const { PUT } = await import("./[id]/route");
    expect((await PUT(put({ name: "Ubah" }), ctx)).status).toBe(200);
  });
});

describe("PUT /api/settings/iam/roles/[id]/permissions", () => {
  it("bukan super_admin pada role sistem → 403 tanpa menulis", async () => {
    const { PUT } = await import("./[id]/permissions/route");
    const res = await PUT(put(grants), ctx);
    expect(res.status).toBe(403);
    expect((await res.json()).error).toBe(SYSTEM_ERR);
    expect(h.replace).not.toHaveBeenCalled();
  });

  it("super_admin pada role sistem → boleh, tanpa sync untuk role biasa", async () => {
    h.superAdmin = true;
    const { PUT } = await import("./[id]/permissions/route");
    expect((await PUT(put(grants), ctx)).status).toBe(200);
    expect(h.replace).toHaveBeenCalledWith(ID, [{ menuId: MENU, grantedActions: ["read"] }], "u1", { syncHrisStaffRoles: false });
  });

  it("admin_hris kosong → 400", async () => {
    h.role = { ...h.role, code: "admin_hris" };
    h.superAdmin = true;
    const { PUT } = await import("./[id]/permissions/route");
    const res = await PUT(put({ permissions: [] }), ctx);
    expect(res.status).toBe(400);
    expect((await res.json()).error).toBe(EMPTY_ERR);
    expect(h.replace).not.toHaveBeenCalled();
  });

  it("simpan admin_hris menjalankan sync", async () => {
    h.role = { ...h.role, code: "admin_hris" };
    h.superAdmin = true;
    const { PUT } = await import("./[id]/permissions/route");
    expect((await PUT(put(grants), ctx)).status).toBe(200);
    expect(h.replace).toHaveBeenCalledWith(ID, [{ menuId: MENU, grantedActions: ["read"] }], "u1", { syncHrisStaffRoles: true });
  });
});
