// Fallback Next untuk Role Management mengikuti Go (EPIC-061, R10): role sistem
// hanya Super Admin; admin_hris tidak boleh kosong dan menjalankan sync HR.
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { NextRequest } from "next/server";

const h = vi.hoisted(() => ({
  role: { id: "r1", code: "go_sys", is_system: true } as Record<string, unknown>,
  superAdmin: false,
  addsDelete: false,
  remove: vi.fn(async () => true),
  written: [] as unknown[],
  guardSql: [] as string[],
  // Seperti replaceIamRolePermissionsInDb: guard jalan di transaksi setelah baris role dikunci, baru menulis.
  replace: vi.fn(
    async (
      _id: string,
      perms: unknown,
      _by: string,
      opts?: { guard?: (q: (sql: string, params: unknown[]) => Promise<{ rows: unknown[] }>, code: string) => Promise<void> }
    ) => {
      await opts?.guard?.(async (sql, params) => {
        h.guardSql.push(sql);
        const ids = (params[1] as string[]) ?? [];
        return { rows: [{ added: h.addsDelete || ids.includes(h.leaf) }] };
      }, String(h.role.code));
      h.written.push(perms);
    }
  ),
  leaf: "0000000a-0000-4000-8000-0000000000aa",
  update: vi.fn(async () => true),
}));

vi.mock("@/lib/api/auth", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/auth")>()),
  requireIamMenuPrefix: vi.fn(async () => ({ id: "u1", full_name: "Admin", role: "admin", brand_id: null })),
}));
vi.mock("@/lib/iam/get-user-menus", () => ({ resolveRoleIds: vi.fn(async () => ["role-of-caller"]) }));
vi.mock("@/lib/iam/pg-client", () => ({
  iamDbQueryOne: vi.fn(async (sql: string) => (sql.includes("granted_actions ? 'delete'") ? { added: h.addsDelete } : { ok: h.superAdmin })),
}));
vi.mock("@/lib/iam/role-repository", () => ({
  getIamRoleFromDb: vi.fn(async () => h.role),
  listRolePermissionsMatrixFromDb: vi.fn(async () => []),
  updateIamRoleInDb: h.update,
  replaceIamRolePermissionsInDb: h.replace,
  deleteIamRoleInDb: h.remove,
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
  h.addsDelete = false;
  h.remove.mockClear();
  h.written = [];
  h.guardSql = [];
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
    expect(h.replace).toHaveBeenCalledWith(ID, [{ menuId: MENU, grantedActions: ["read"] }], "u1", expect.objectContaining({ syncHrisStaffRoles: false }));
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
    expect(h.replace).toHaveBeenCalledWith(ID, [{ menuId: MENU, grantedActions: ["read"] }], "u1", expect.objectContaining({ syncHrisStaffRoles: true }));
  });
});

// Role HR (admin_hris + hris_*) dan hak hapus HRIS hanya lewat Super Admin di Role Management (sama dengan Go).
const HR_ERR = "Role HR hanya bisa diubah oleh Super Admin; AdminHRIS memakai halaman Hak Akses HRIS";
const DELETE_ERR = "Hak hapus HRIS hanya bisa diberikan oleh Super Admin";
const del = () => new Request("http://x/api", { method: "DELETE" }) as unknown as NextRequest;

describe("Role HR di Role Management", () => {
  for (const code of ["admin_hris", "hris_manager", "hris_non_payroll", "hris_non_rekrutmen_kpi"]) {
    it(`${code}: ubah, grant, dan hapus oleh bukan super_admin → 403`, async () => {
      h.role = { ...h.role, code, is_system: false };
      const roleRoute = await import("./[id]/route");
      const permRoute = await import("./[id]/permissions/route");
      for (const res of [
        await roleRoute.PUT(put({ name: "Ubah" }), ctx),
        await permRoute.PUT(put(grants), ctx),
        await roleRoute.DELETE(del(), ctx),
      ]) {
        expect(res.status).toBe(403);
        expect((await res.json()).error).toBe(HR_ERR);
      }
      expect(h.update).not.toHaveBeenCalled();
      expect(h.written).toEqual([]);
      expect(h.remove).not.toHaveBeenCalled();
    });
  }

  it("super_admin boleh mengubah role HR", async () => {
    h.role = { ...h.role, code: "hris_manager", is_system: false };
    h.superAdmin = true;
    const { PUT, DELETE } = await import("./[id]/route");
    expect((await PUT(put({ name: "HR Manager" }), ctx)).status).toBe(200);
    expect((await DELETE(del(), ctx)).status).toBe(200);
  });

  it("mengganti kode role lain menjadi kode role HR → 403", async () => {
    h.role = { ...h.role, code: "kasir", is_system: false };
    const { PUT } = await import("./[id]/route");
    const res = await PUT(put({ code: "HRIS_Manager " }), ctx);
    expect(res.status).toBe(403);
    expect((await res.json()).error).toBe(HR_ERR);
    expect(h.update).not.toHaveBeenCalled();
  });
});

describe("Hak hapus HRIS di Role Management", () => {
  it("menambah delete pada menu HRIS oleh bukan super_admin → 403 tanpa menulis", async () => {
    h.role = { ...h.role, code: "kasir", is_system: false };
    h.addsDelete = true;
    const { PUT } = await import("./[id]/permissions/route");
    const res = await PUT(put({ permissions: [{ menuId: MENU, isGranted: true, grantedActions: ["read", "delete"] }] }), ctx);
    expect(res.status).toBe(403);
    expect((await res.json()).error).toBe(DELETE_ERR);
    expect(h.written).toEqual([]);
  });

  it("delete yang sudah ada (bukan penambahan) tetap boleh", async () => {
    h.role = { ...h.role, code: "kasir", is_system: false };
    const { PUT } = await import("./[id]/permissions/route");
    expect((await PUT(put({ permissions: [{ menuId: MENU, isGranted: true, grantedActions: ["read", "delete"] }] }), ctx)).status).toBe(200);
    expect(h.written).toHaveLength(1);
  });

  it("super_admin boleh menambah delete HRIS", async () => {
    h.role = { ...h.role, code: "kasir", is_system: false };
    h.addsDelete = true;
    h.superAdmin = true;
    const { PUT } = await import("./[id]/permissions/route");
    expect((await PUT(put({ permissions: [{ menuId: MENU, isGranted: true, grantedActions: ["read", "delete"] }] }), ctx)).status).toBe(200);
  });
});

// Re-review: menuId dikanonikkan (huruf kecil, bertanda hubung) dan dibandingkan sebagai uuid, jadi bentuk
// uuid lain tidak lolos guard hak hapus HRIS; id tidak valid → 400.
describe("menuId bentuk uuid lain", () => {
  const withDelete = (menuId: string) => put({ permissions: [{ menuId, isGranted: true, grantedActions: ["read", "delete"] }] });

  for (const form of [h.leaf.toUpperCase(), h.leaf.replaceAll("-", ""), `{${h.leaf.toUpperCase()}}`]) {
    it(`${form} oleh bukan super_admin → 403 tanpa menulis`, async () => {
      h.role = { ...h.role, code: "kasir", is_system: false };
      const { PUT } = await import("./[id]/permissions/route");
      const res = await PUT(withDelete(form), ctx);
      expect(res.status).toBe(403);
      expect((await res.json()).error).toBe(DELETE_ERR);
      expect(h.written).toEqual([]);
      expect(h.guardSql.join(" ")).toContain("::uuid[]");
    });
  }

  for (const bad of ["not-a-uuid", `${h.leaf}0`, `{${h.leaf}`]) {
    it(`${bad} → 400`, async () => {
      h.role = { ...h.role, code: "kasir", is_system: false };
      const { PUT } = await import("./[id]/permissions/route");
      const res = await PUT(withDelete(bad), ctx);
      expect(res.status).toBe(400);
      expect((await res.json()).error).toBe("Validation failed");
      expect(h.written).toEqual([]);
    });
  }

  it("super_admin boleh, menuId ditulis dalam bentuk kanonik", async () => {
    h.role = { ...h.role, code: "kasir", is_system: false };
    h.superAdmin = true;
    const { PUT } = await import("./[id]/permissions/route");
    expect((await PUT(withDelete(h.leaf.toUpperCase()), ctx)).status).toBe(200);
    expect(h.written).toEqual([[{ menuId: h.leaf, grantedActions: ["read", "delete"] }]]);
  });
});
