import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { PayslipDetail } from "../types";
import { PayslipPage } from "./payslip-page";

let detail: PayslipDetail;

vi.mock("next/navigation", () => ({
  useParams: () => ({ id: "run-1", employeeId: "emp-1" }),
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock("../queries", () => ({
  usePayslip: () => ({ data: detail, isLoading: false }),
}));

const slip = (over: Partial<PayslipDetail> = {}): PayslipDetail =>
  ({
    id: "s1",
    employee_id: "emp-1",
    payroll_run_id: "run-1",
    status: "draft",
    created_at: "2026-11-01",
    base_salary: 5_000_000,
    fixed_allowance: 0,
    variable_allowance: 0,
    transport_allowance: 0,
    meal_allowance: 0,
    housing_allowance: 0,
    other_earning: 0,
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
    unpaid_leave_days: 0,
    employee: { id: "emp-1", full_name: "Budi", nip: "100", email: "", phone: "" },
    payroll_run: { id: "run-1", period_month: 11, period_year: 2026, run_name: "Nov" },
    ...over,
  }) as PayslipDetail;

afterEach(cleanup);

describe("PayslipPage baris potongan nonaktif (EPIC-082 R5)", () => {
  it("menyembunyikan PPh 21 dan BPJS bila nilainya 0", () => {
    detail = slip();
    render(<PayslipPage />);
    expect(screen.queryByText(/PPh 21/)).toBeNull();
    expect(screen.queryByText(/BPJS/)).toBeNull();
    expect(screen.getByText("Total Potongan")).toBeTruthy();
  });

  it("menampilkan PPh 21 dan BPJS bila nilainya > 0", () => {
    detail = slip({
      bpjs_tk_jht_deduction: 100_000,
      bpjs_tk_jp_deduction: 50_000,
      bpjs_kes_deduction: 50_000,
      pph21_deduction: 25_000,
      total_deductions: 225_000,
    });
    render(<PayslipPage />);
    expect(screen.getByText("PPh 21")).toBeTruthy();
    expect(screen.getByText("BPJS TK (JHT)")).toBeTruthy();
    expect(screen.getByText("BPJS TK (JP)")).toBeTruthy();
    expect(screen.getByText("BPJS Kesehatan")).toBeTruthy();
  });

  it("hanya baris yang bernilai tampil saat sebagian saklar mati", () => {
    detail = slip({ pph21_deduction: 25_000, total_deductions: 25_000 });
    render(<PayslipPage />);
    expect(screen.getByText("PPh 21")).toBeTruthy();
    expect(screen.queryByText(/BPJS/)).toBeNull();
  });

  it("catatan TER tetap tampil saat metode ter dan PPh 21 > 0", () => {
    detail = slip({
      pph21_deduction: 25_000,
      pph21_method: "ter",
      ter_rate: 0.005,
      ter_category: "A",
      pph21_dpp: 5_000_000,
    });
    render(<PayslipPage />);
    expect(screen.getByText(/TER kategori A/)).toBeTruthy();
  });

  it("catatan TER disembunyikan saat metode ter tetapi PPh 21 = 0", () => {
    detail = slip({
      pph21_deduction: 0,
      pph21_method: "ter",
      ter_rate: 0,
      ter_category: "A",
      pph21_dpp: 1_000_000,
    });
    render(<PayslipPage />);
    expect(screen.queryByText(/TER kategori/)).toBeNull();
    expect(screen.queryByText(/PPh 21/)).toBeNull();
  });

  it("run dengan metode nonaktif tidak menampilkan catatan PPh 21 sama sekali", () => {
    detail = slip({ pph21_method: "nonaktif", pph21_dpp: 0, ter_rate: null });
    render(<PayslipPage />);
    expect(screen.queryByText(/PPh 21/)).toBeNull();
    expect(screen.queryByText(/Koreksi Desember/)).toBeNull();
  });
});

describe("PayslipPage tanpa potongan keterlambatan (EPIC-094 R8)", () => {
  it("baris Potongan Keterlambatan tidak tampil bila nol; Izin Pribadi tetap", () => {
    detail = slip({ late_days: 7, late_deduction: 0, personal_leave_deduction: 200_000, total_deductions: 200_000 });
    render(<PayslipPage />);
    expect(screen.queryByText("Potongan Keterlambatan")).toBeNull();
    expect(screen.getByText("Potongan Izin Pribadi")).toBeTruthy();
  });

  it("slip lama yang masih memuat potongan telat tetap menampilkannya", () => {
    detail = slip({ late_deduction: 15_000, total_deductions: 15_000 });
    render(<PayslipPage />);
    expect(screen.getByText("Potongan Keterlambatan")).toBeTruthy();
  });
});
