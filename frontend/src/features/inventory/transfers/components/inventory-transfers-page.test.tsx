import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";

const mocks = vi.hoisted(() => ({ warehouses: [] as unknown[], sourceStock: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => "/dashboard/inventory/transfers",
}));
vi.mock("../queries", () => ({
  useTransferWarehouses: () => ({ data: mocks.warehouses, isLoading: false }),
  useTransferSourceStock: (id: string) => {
    mocks.sourceStock(id);
    return { data: undefined, isLoading: false, isError: false, error: null, refetch: vi.fn() };
  },
}));
vi.mock("../api", () => ({ createStockTransfer: vi.fn() }));
vi.mock("./transfer-history-section", () => ({ TransferHistorySection: () => null }));
vi.mock("@/features/purchasing/components/shared/purchasing-page-header", () => ({
  PurchasingFormHeader: () => null,
}));

import { InventoryTransfersPage } from "./inventory-transfers-page";

const wh = (id: string, code: string, branch: string, label: string, isDefault = false) => ({
  id,
  name: isDefault ? "Main Storage" : "Stall 1",
  code,
  is_default: isDefault,
  branch_id: branch,
  label,
});
const wrap = (ui: React.ReactElement) => {
  return render(<QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>);
};

// cmdk (Combobox) needs these browser APIs, which jsdom lacks.
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);
Element.prototype.scrollIntoView = vi.fn();

afterEach(() => {
  cleanup();
  mocks.sourceStock.mockClear();
});

const triggers = () => screen.getAllByRole("combobox");

describe("InventoryTransfersPage Main Storage lock", () => {
  it("lintas branch: memilih stall branch B mengunci Main Storage branch B", () => {
    mocks.warehouses = [
      wh("m-a", "MAIN", "a", "Main Storage — Alpha", true),
      wh("s-a", "STALL-01", "a", "Stall 1 — Alpha"),
      wh("m-b", "MAIN", "b", "Main Storage — Beta", true),
      wh("s-b", "STALL-01", "b", "Stall 1 — Beta"),
    ];
    wrap(<InventoryTransfersPage />);
    // Nothing is locked until a stall is chosen.
    expect(screen.getByText("Pilih outlet tujuan dulu")).toBeInTheDocument();
    expect(screen.queryByText("Main Storage — Alpha")).not.toBeInTheDocument();

    fireEvent.click(triggers()[1]);
    fireEvent.click(screen.getByText("Stall 1 — Beta"));

    const source = triggers()[0];
    expect(within(source).getByText("Main Storage — Beta")).toBeInTheDocument();
    expect(mocks.sourceStock).toHaveBeenLastCalledWith("m-b");
  });

  it("satu branch: Main Storage terkunci sejak awal (tidak berubah)", () => {
    mocks.warehouses = [
      wh("m-a", "MAIN", "a", "Main Storage", true),
      wh("s-a", "STALL-01", "a", "Stall 1"),
    ];
    wrap(<InventoryTransfersPage />);
    expect(within(triggers()[0]).getByText("Main Storage")).toBeInTheDocument();
    expect(mocks.sourceStock).toHaveBeenLastCalledWith("m-a");
  });
});
