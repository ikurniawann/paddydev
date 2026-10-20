import { beforeEach, describe, expect, it, vi } from "vitest";

const client = vi.hoisted(() => ({
  query: vi.fn(async (sql?: string) => ({ rows: String(sql).includes("FOR UPDATE") ? [{ code: "kasir" }] : [] })),
  release: vi.fn(),
}));
vi.mock("./pg-client", () => ({
  getIamPool: () => ({ connect: async () => client }),
  iamDbQuery: vi.fn(),
  iamDbQueryOne: vi.fn(),
}));

import { replaceIamRolePermissionsInDb } from "./role-repository";

const sqls = () => client.query.mock.calls.map((c) => String((c as unknown[])[0]));

beforeEach(() => client.query.mockClear());

describe("replaceIamRolePermissionsInDb", () => {
  it("syncHrisStaffRoles menjalankan sync sebelum COMMIT di transaksi yang sama", async () => {
    await replaceIamRolePermissionsInDb("r1", [{ menuId: "m1", grantedActions: ["read"] }], "u1", { syncHrisStaffRoles: true });
    const out = sqls();
    const sync = out.findIndex((s) => s.includes("iam.sync_hris_staff_roles()"));
    expect(sync).toBeGreaterThan(out.indexOf("BEGIN"));
    expect(sync).toBeLessThan(out.indexOf("COMMIT"));
  });

  it("tanpa opsi tidak menjalankan sync", async () => {
    await replaceIamRolePermissionsInDb("r1", [{ menuId: "m1", grantedActions: ["read"] }], "u1");
    expect(sqls().some((s) => s.includes("sync_hris_staff_roles"))).toBe(false);
  });
});

describe("replaceIamRolePermissionsInDb guard", () => {
  it("guard jalan setelah baris role dikunci (FOR UPDATE), sebelum menulis, dengan kode role terkunci", async () => {
    const order: string[] = [];
    await replaceIamRolePermissionsInDb("r1", [{ menuId: "m1", grantedActions: ["read"] }], "u1", {
      guard: async (_q, code) => {
        order.push(`guard:${code}:${sqls().length}`);
      },
    });
    const out = sqls();
    const lock = out.findIndex((s) => s.includes("FOR UPDATE"));
    expect(lock).toBeGreaterThan(out.indexOf("BEGIN"));
    expect(order).toEqual([`guard:kasir:${lock + 1}`]);
    expect(out.findIndex((s) => s.includes("UPDATE iam.role_menu_permissions"))).toBeGreaterThan(lock);
  });

  it("guard menolak → ROLLBACK tanpa menulis", async () => {
    await expect(
      replaceIamRolePermissionsInDb("r1", [{ menuId: "m1", grantedActions: ["read"] }], "u1", {
        guard: async () => {
          throw new Error("ditolak");
        },
      })
    ).rejects.toThrow("ditolak");
    const out = sqls();
    expect(out).toContain("ROLLBACK");
    expect(out.some((s) => s.includes("role_menu_permissions"))).toBe(false);
  });
});
