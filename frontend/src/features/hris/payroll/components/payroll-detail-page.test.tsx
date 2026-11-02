import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PayrollRunDetail } from "../types";
import { PayrollDetailPage } from "./payroll-detail-page";

let run: Partial<PayrollRunDetail>;

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("../queries", () => ({ usePayrollRun: () => ({ data: run, isLoading: false }) }));
vi.mock("../mutations", () => ({ useNotifyPayslip: () => ({ mutate: vi.fn(), isPending: false }) }));
vi.mock("./store-breakdown", () => ({ AllocationChips: () => null }));
vi.mock("./calculation-panel", () => ({ CalculationPanel: () => null }));
vi.mock("./manual-inputs-card", () => ({ ManualInputsCard: () => null }));
vi.mock("./store-allocation-card", () => ({ StoreAllocationCard: () => null }));

// Promise yang sudah selesai agar React `use()` tidak suspend.
const params = Object.assign(Promise.resolve({ id: "run-1" }), {
  status: "fulfilled",
  value: { id: "run-1" },
}) as Promise<{ id: string }>;

const base = (over: Partial<PayrollRunDetail> = {}): Partial<PayrollRunDetail> => ({
  id: "run-1",
  run_name: "Payroll Nov",
  period_month: 11,
  period_year: 2026,
  status: "draft",
  total_employees: 1,
  total_gross: 5_000_000,
  total_net: 5_000_000,
  total_deductions: 0,
  total_pph21: 0,
  total_bjtk_employee: 0,
  total_bjtk_employer: 0,
  company_id: null,
  payroll_details: [],
  ...over,
});

afterEach(cleanup);

describe("PayrollDetailPage ringkasan tanpa komponen nonaktif (EPIC-082 R5)", () => {
  it("menyembunyikan baris PPh 21 dan kartu Employer BPJS bila totalnya 0", () => {
    run = base();
    render(<PayrollDetailPage params={params} />);
    expect(screen.getByText("Total Potongan")).toBeTruthy();
    expect(screen.queryByText(/PPh 21/)).toBeNull();
    expect(screen.queryByText("Employer BPJS")).toBeNull();
  });

  it("menampilkannya bila totalnya > 0", () => {
    run = base({ total_pph21: 25_000, total_bjtk_employer: 300_000, total_bjtk_employee: 200_000 });
    render(<PayrollDetailPage params={params} />);
    expect(screen.getByText(/PPh 21:/)).toBeTruthy();
    expect(screen.getByText("Employer BPJS")).toBeTruthy();
  });
});
