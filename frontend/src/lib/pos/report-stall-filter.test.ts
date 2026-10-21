import { beforeEach, describe, expect, it, vi } from "vitest";
import { resolveReportStallFilter, shouldExpandReportStallsToBranch } from "./report-stall-filter";

describe("shouldExpandReportStallsToBranch", () => {
  it("keeps assigned-only stalls for a regular stall cashier", () => {
    expect(
      shouldExpandReportStallsToBranch({
        isUnscoped: false,
        canCentralCheckout: false,
      })
    ).toBe(false);
  });

  it("expands to all branch stalls for kasir pusat", () => {
    expect(
      shouldExpandReportStallsToBranch({
        isUnscoped: false,
        canCentralCheckout: true,
      })
    ).toBe(true);
  });

  it("expands for unscoped / super admin", () => {
    expect(
      shouldExpandReportStallsToBranch({
        isUnscoped: true,
        canCentralCheckout: false,
      })
    ).toBe(true);
  });
});

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  loadUserWarehouses: vi.fn(),
  loadCentralCashierGate: vi.fn(),
}));
vi.mock("@/lib/db", () => ({ query: mocks.query, queryOne: vi.fn() }));
vi.mock("@/lib/users/user-warehouses", () => ({ loadUserWarehouses: mocks.loadUserWarehouses }));
vi.mock("@/lib/pos/pos-sell-stall-server", () => ({
  loadCentralCashierGate: mocks.loadCentralCashierGate,
}));

const scope = (over: object = {}) =>
  ({ userId: "u1", role: "pos", isUnscoped: false, branchId: null, ...over }) as never;

describe("resolveReportStallFilter branch labels", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.loadCentralCashierGate.mockResolvedValue({ canCentralCheckout: false });
  });

  it("picker lintas branch: label memuat nama branch dan urut per branch", async () => {
    mocks.query
      .mockResolvedValueOnce([
        { id: "z-m", code: "MAIN", name: "Main Storage", branch_id: "b-z", is_default: true },
        { id: "a-s", code: "STALL-01", name: "Stall 1", branch_id: "b-a", is_default: false },
        { id: "a-m", code: "MAIN", name: "Main Storage", branch_id: "b-a", is_default: true },
      ])
      .mockResolvedValueOnce([
        { id: "b-z", name: "Zeta" },
        { id: "b-a", name: "Alpha" },
      ]);
    const filter = await resolveReportStallFilter(scope({ isUnscoped: true }), null);
    expect(filter.stallOptions).toEqual([
      { id: "a-m", code: "MAIN", name: "Main Storage", branch_id: "b-a", branch_name: "Alpha", label: "Main Storage — Alpha" },
      { id: "a-s", code: "STALL-01", name: "Stall 1", branch_id: "b-a", branch_name: "Alpha", label: "Stall 1 — Alpha" },
      { id: "z-m", code: "MAIN", name: "Main Storage", branch_id: "b-z", branch_name: "Zeta", label: "Main Storage — Zeta" },
    ]);
  });

  it("picker satu branch: label = nama polos", async () => {
    mocks.query
      .mockResolvedValueOnce([{ id: "a-m", code: "MAIN", name: "Main Storage", branch_id: "b-a", is_default: true }])
      .mockResolvedValueOnce([{ id: "b-a", name: "Alpha" }]);
    const filter = await resolveReportStallFilter(scope({ isUnscoped: true }), null);
    expect(filter.stallOptions[0]).toMatchObject({ label: "Main Storage", branch_name: "Alpha" });
  });

  it("outlet penempatan lintas branch juga berlabel", async () => {
    mocks.loadUserWarehouses.mockResolvedValue([
      { warehouse_id: "z-m", code: "MAIN", name: "Main Storage", branch_id: "b-z", is_default: true },
      { warehouse_id: "a-m", code: "MAIN", name: "Main Storage", branch_id: "b-a", is_default: true },
    ]);
    mocks.query.mockResolvedValueOnce([
      { id: "b-z", name: "Zeta" },
      { id: "b-a", name: "Alpha" },
    ]);
    const filter = await resolveReportStallFilter(scope(), null);
    expect(filter.stallOptions.map((o) => o.label)).toEqual(["Main Storage — Alpha", "Main Storage — Zeta"]);
    expect(filter.warehouseIds).toEqual(["a-m", "z-m"]);
  });
});
