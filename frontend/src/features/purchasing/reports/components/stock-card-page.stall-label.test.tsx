import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const mocks = vi.hoisted(() => ({ useStockCard: vi.fn(), useStockWarehouses: vi.fn(), search: "" }));
vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(mocks.search),
  usePathname: () => "/dashboard/purchasing/reports/stock-card",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("../queries", () => ({
  useStockCard: mocks.useStockCard,
  useStockWarehouses: mocks.useStockWarehouses,
}));

import { StockCardPage } from "./stock-card-page";

afterEach(cleanup);

describe("StockCardPage outlet filter", () => {
  it("menampilkan label outlet (nama + branch) pada filter yang terpilih", () => {
    mocks.search = "warehouse_id=b";
    mocks.useStockCard.mockReturnValue({ data: undefined, isLoading: false, isFetching: false, error: null });
    mocks.useStockWarehouses.mockReturnValue({
      isLoading: false,
      error: null,
      data: [
        { id: "a", name: "Main Storage", code: "MAIN", label: "Main Storage — Alpha" },
        { id: "b", name: "Main Storage", code: "MAIN", label: "Main Storage — Beta" },
      ],
    });
    render(<StockCardPage />);
    expect(screen.getByText("Main Storage — Beta")).toBeInTheDocument();
    expect(screen.queryByText("Main Storage")).not.toBeInTheDocument();
  });
});
