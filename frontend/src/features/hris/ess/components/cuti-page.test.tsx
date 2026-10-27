import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EssLeaveBalance, EssMe } from "../types";
import { EssCutiPage } from "./cuti-page";

let me: EssMe | undefined;
vi.mock("../queries", () => ({
  useEssMe: () => ({ data: me, isLoading: false }),
  useMyLeaves: () => ({ data: [] }),
}));
vi.mock("./cuti-request-dialog", () => ({ CutiRequestDialog: () => null }));

const balance: EssLeaveBalance = {
  year: 2026,
  annual_leave_total: 12,
  annual_leave_adjustment: 1,
  annual_leave_used: 4,
  annual_leave_remaining: 9,
  carry_over_days: 2,
  carry_over_used: 0,
  carry_over_remaining: 2,
  carry_over_expires_on: "2026-12-31",
  carry_over_active: true,
  available: 11,
};

const employee = { id: "e1", full_name: "Ayu", nip: "N1", position_title: null, department_name: null };

afterEach(cleanup);

describe("EssCutiPage", () => {
  it("headlines the available balance with the breakdown", () => {
    me = { employee, leave_balance: balance };
    render(<EssCutiPage />);
    expect(screen.getByText("11")).toBeTruthy();
    expect(screen.getByText(/hari tersedia/)).toBeTruthy();
    expect(screen.getByText("Penyesuaian")).toBeTruthy();
    expect(screen.getByText("Sisa bawaan")).toBeTruthy();
    expect(screen.getByText(/Kuota Cuti Tahunan 2026/)).toBeTruthy();
  });

  it("shows an empty state instead of an invented 12 days when there is no balance", () => {
    me = { employee, leave_balance: null };
    render(<EssCutiPage />);
    expect(screen.getByText(/Kuota cuti tahunan Anda belum tersedia/)).toBeTruthy();
    expect(screen.queryByText("12")).toBeNull();
    expect(screen.queryByText(/hari tersedia/)).toBeNull();
  });
});
