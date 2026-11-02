import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { EssPayslip } from "../types";
import { PayslipDetailDialog } from "./payslip-detail-dialog";

const slip = (over: Partial<EssPayslip> = {}): EssPayslip =>
  ({
    id: "s1",
    base_salary: 5_000_000,
    fixed_allowance: 0,
    variable_allowance: 0,
    transport_allowance: 0,
    meal_allowance: 0,
    housing_allowance: 0,
    overtime_pay: 0,
    thr: 0,
    bonus: 0,
    gross_salary: 5_000_000,
    bpjs_tk_jht_deduction: 0,
    bpjs_tk_jp_deduction: 0,
    bpjs_kes_deduction: 0,
    tapera_deduction: 0,
    pph21_deduction: 0,
    unpaid_leave_deduction: 0,
    other_deduction: 0,
    total_deductions: 0,
    net_salary: 5_000_000,
    working_days: 26,
    present_days: 26,
    late_days: 0,
    payroll_run: { id: "r1", period_month: 11, period_year: 2026, status: "approved", paid_at: null },
    ...over,
  }) as EssPayslip;

afterEach(cleanup);

describe("PayslipDetailDialog baris potongan nonaktif (EPIC-082 R5)", () => {
  it("tidak menampilkan PPh 21 / BPJS bila nilainya 0", () => {
    render(<PayslipDetailDialog selected={slip()} onClose={() => {}} />);
    expect(screen.getByText("Gaji Pokok")).toBeTruthy();
    expect(screen.queryByText(/PPh 21/)).toBeNull();
    expect(screen.queryByText(/BPJS/)).toBeNull();
  });

  it("menampilkan PPh 21 / BPJS bila nilainya > 0", () => {
    render(
      <PayslipDetailDialog
        selected={slip({
          bpjs_tk_jht_deduction: 100_000,
          bpjs_tk_jp_deduction: 50_000,
          bpjs_kes_deduction: 50_000,
          pph21_deduction: 25_000,
        })}
        onClose={() => {}}
      />
    );
    for (const l of ["PPh 21", "BPJS TK (JHT)", "BPJS TK (JP)", "BPJS Kesehatan"]) {
      expect(screen.getByText(l)).toBeTruthy();
    }
  });
});
