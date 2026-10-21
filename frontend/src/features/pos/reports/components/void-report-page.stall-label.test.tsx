import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const mocks = vi.hoisted(() => ({ useVoidReport: vi.fn() }));
vi.mock("../queries", () => ({ useVoidReport: mocks.useVoidReport }));
vi.mock("../utils/load-order-detail", () => ({ loadOrderTransactionDetail: vi.fn() }));
vi.mock("./transaction-detail-body", () => ({ TransactionDetailBody: () => null }));

import { VoidReportPage } from "./void-report-page";

afterEach(cleanup);

describe("VoidReportPage outlet filter", () => {
  it("menampilkan label outlet (nama + branch) dari stall_options", () => {
    mocks.useVoidReport.mockReturnValue({
      isLoading: false,
      isFetching: false,
      error: null,
      data: {
        rows: [],
        summary: { voids: 0, amount: 0 },
        stall_locked: false,
        filters: { date_from: "2031-05-01", date_to: "2031-05-31", warehouse_id: null },
        stall_options: [
          { id: "a", code: "MAIN", name: "Main Storage", branch_name: "Alpha", label: "Main Storage — Alpha" },
          { id: "b", code: "MAIN", name: "Main Storage", branch_name: "Beta", label: "Main Storage — Beta" },
        ],
      },
    });
    render(<VoidReportPage />);
    expect(screen.getByRole("option", { name: "Main Storage — Alpha (MAIN)" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Main Storage — Beta (MAIN)" })).toBeInTheDocument();
  });

  it("tanpa label (satu branch / data lama) jatuh ke nama", () => {
    mocks.useVoidReport.mockReturnValue({
      isLoading: false,
      isFetching: false,
      error: null,
      data: {
        rows: [],
        summary: { voids: 0, amount: 0 },
        stall_locked: false,
        filters: { date_from: "2031-05-01", date_to: "2031-05-31", warehouse_id: null },
        stall_options: [{ id: "a", code: "STALL-01", name: "Stall 1" }],
      },
    });
    render(<VoidReportPage />);
    expect(screen.getByRole("option", { name: "Stall 1 (STALL-01)" })).toBeInTheDocument();
  });
});
