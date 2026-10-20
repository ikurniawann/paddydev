import { beforeEach, describe, expect, it, vi } from "vitest";

const client = vi.hoisted(() => ({ query: vi.fn(async () => ({ rows: [] })), release: vi.fn() }));
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
