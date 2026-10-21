import { beforeEach, describe, expect, it, vi } from "vitest";
import { createFakeDb } from "@/lib/purchasing/item-fake-db.test-util";

const state = vi.hoisted(() => ({
  fake: null as null | ReturnType<typeof import("@/lib/purchasing/item-fake-db.test-util").createFakeDb>,
}));

vi.mock("@/lib/pg/create-client", () => ({ createPgClient: () => state.fake!.db }));
vi.mock("@/lib/api/auth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/auth")>()),
  requireIamMenuPrefix: vi.fn(async () => ({ id: "u1", role: "admin" })),
}));
vi.mock("@/lib/pos/receipt-settings", () => ({
  loadPosReceiptSettingsRows: async () => [{ id: "r1" }],
  normalizeReceiptSettings: (row: { id: string }) => ({ id: row.id }),
}));

describe("GET /api/settings/receipt", () => {
  beforeEach(() => {
    state.fake = null;
  });

  it("stalls memuat branch_name + label; lintas branch dibedakan dan diurut per branch", async () => {
    state.fake = createFakeDb({
      warehouses: [
        {
          data: [
            { id: "m-z", name: "Main Storage", branch_id: "b-z", is_default: true },
            { id: "m-a", name: "Main Storage", branch_id: "b-a", is_default: true },
            { id: "s-a", name: "Stall 1", branch_id: "b-a", is_default: false },
          ],
          error: null,
        },
      ],
      branches: [
        {
          data: [
            { id: "b-z", name: "Zeta" },
            { id: "b-a", name: "Alpha" },
          ],
          error: null,
        },
      ],
    });
    const { GET } = await import("./route");
    const json = await (await GET()).json();
    expect(json.data).toEqual([{ id: "r1" }]);
    expect(json.stalls).toEqual([
      { id: "m-a", name: "Main Storage", branch_id: "b-a", branch_name: "Alpha", label: "Main Storage — Alpha" },
      { id: "s-a", name: "Stall 1", branch_id: "b-a", branch_name: "Alpha", label: "Stall 1 — Alpha" },
      { id: "m-z", name: "Main Storage", branch_id: "b-z", branch_name: "Zeta", label: "Main Storage — Zeta" },
    ]);
  });

  it("satu branch: label = nama polos", async () => {
    state.fake = createFakeDb({
      warehouses: [{ data: [{ id: "s1", name: "Stall 1", branch_id: "b1", is_default: false }], error: null }],
      branches: [{ data: [{ id: "b1", name: "Alpha" }], error: null }],
    });
    const { GET } = await import("./route");
    const json = await (await GET()).json();
    expect(json.stalls).toEqual([
      { id: "s1", name: "Stall 1", branch_id: "b1", branch_name: "Alpha", label: "Stall 1" },
    ]);
  });
});
