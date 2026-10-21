// Route test daftar stall: guard IAM (403) dan user company tanpa cabang → [].
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createFakeDb, jsonRequest } from "@/lib/purchasing/item-fake-db.test-util";

const state = vi.hoisted(() => ({
  fake: null as null | ReturnType<typeof import("@/lib/purchasing/item-fake-db.test-util").createFakeDb>,
  allowed: true,
}));

vi.mock("@/lib/pg/create-client", () => ({
  createServerPgClient: vi.fn(async () => state.fake!.db),
}));
vi.mock("@/lib/api/scope", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/scope")>()),
  getApiUserScope: vi.fn(async () => ({
    userId: "user-1",
    role: "admin",
    businessScope: "company",
    holdingId: null,
    companyId: "company-1",
    branchId: null,
    isUnscoped: false,
  })),
}));
vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return {
    ...actual,
    requireIamMenuPrefix: vi.fn(async () => {
      if (!state.allowed) throw actual.ApiError.forbidden();
      return { id: "user-1", full_name: "Admin", role: "admin", brand_id: null };
    }),
  };
});

describe("/api/purchasing/warehouses", () => {
  beforeEach(() => {
    state.fake = createFakeDb({});
    state.allowed = true;
  });

  it("tanpa grant → 403", async () => {
    state.allowed = false;
    const { GET } = await import("./route");
    const res = await GET(jsonRequest("http://localhost/api/purchasing/warehouses"));
    expect(res.status).toBe(403);
    expect(await res.json()).toEqual({ success: false, error: "Insufficient permissions" });
  });

  it("branch_id bukan uuid → 400", async () => {
    const { GET } = await import("./route");
    const res = await GET(jsonRequest("http://localhost/api/purchasing/warehouses?branch_id=x"));
    expect(res.status).toBe(400);
    expect((await res.json()).error).toBe("Branch ID harus valid");
  });

  it("company tanpa cabang → { success, data: [] }", async () => {
    state.fake = createFakeDb({ branches: [{ data: [], error: null }] });
    const { GET } = await import("./route");
    const res = await GET(jsonRequest("http://localhost/api/purchasing/warehouses"));
    expect(await res.json()).toEqual({ success: true, data: [] });
  });

  it("company dengan cabang → filter branch_id IN, baris diberi branch_name + label", async () => {
    state.fake = createFakeDb({
      branches: [
        { data: [{ id: "branch-1" }], error: null },
        { data: [{ id: "branch-1", name: "Alpha" }], error: null },
      ],
      warehouses: [{ data: [{ id: "w-1", name: "Stall A", branch_id: "branch-1" }], error: null }],
    });
    const { GET } = await import("./route");
    const res = await GET(jsonRequest("http://localhost/api/purchasing/warehouses"));
    expect(await res.json()).toEqual({
      success: true,
      data: [{ id: "w-1", name: "Stall A", branch_id: "branch-1", branch_name: "Alpha", label: "Stall A" }],
    });
    const warehouseCall = state.fake!.calls.find((c) => c.table === "warehouses");
    expect(warehouseCall?.filters).toContainEqual(["in", "branch_id", ["branch-1"]]);
  });

  it("lintas cabang → label memuat nama branch, urut per branch, Main Storage dulu", async () => {
    state.fake = createFakeDb({
      branches: [
        { data: [{ id: "b-z" }, { id: "b-a" }], error: null },
        {
          data: [
            { id: "b-z", name: "Zeta" },
            { id: "b-a", name: "Alpha" },
          ],
          error: null,
        },
      ],
      warehouses: [
        {
          data: [
            { id: "z-s", name: "Stall 1", branch_id: "b-z", is_default: false },
            { id: "z-m", name: "Main Storage", branch_id: "b-z", is_default: true },
            { id: "a-m", name: "Main Storage", branch_id: "b-a", is_default: true },
          ],
          error: null,
        },
      ],
    });
    const { GET } = await import("./route");
    const { data } = await (await GET(jsonRequest("http://localhost/api/purchasing/warehouses"))).json();
    expect(data.map((w: { id: string; label: string }) => [w.id, w.label])).toEqual([
      ["a-m", "Main Storage — Alpha"],
      ["z-m", "Main Storage — Zeta"],
      ["z-s", "Stall 1 — Zeta"],
    ]);
    expect(data[0]).toMatchObject({ branch_id: "b-a", branch_name: "Alpha", name: "Main Storage" });
  });
});
