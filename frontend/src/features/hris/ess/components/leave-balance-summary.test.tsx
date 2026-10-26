import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { EssLeaveBalance } from "../types";
import { LeaveBalanceBreakdown, balanceSubtitle } from "./leave-balance-summary";

const base: EssLeaveBalance = {
  year: 2026,
  annual_leave_total: 12,
  annual_leave_adjustment: 0,
  annual_leave_used: 3,
  annual_leave_remaining: 9,
  carry_over_days: 0,
  carry_over_used: 0,
  carry_over_remaining: 0,
  carry_over_expires_on: null,
  carry_over_active: false,
  available: 9,
};

afterEach(cleanup);

describe("LeaveBalanceBreakdown", () => {
  it("shows jatah and terpakai, hides penyesuaian and sisa bawaan when absent", () => {
    render(<LeaveBalanceBreakdown balance={base} />);
    expect(screen.getByText("Jatah")).toBeTruthy();
    expect(screen.getByText("12 hari")).toBeTruthy();
    expect(screen.getByText("Terpakai")).toBeTruthy();
    expect(screen.getByText("3 hari")).toBeTruthy();
    expect(screen.queryByText("Penyesuaian")).toBeNull();
    expect(screen.queryByText(/Sisa bawaan/)).toBeNull();
  });

  it("shows a signed penyesuaian when not zero", () => {
    render(<LeaveBalanceBreakdown balance={{ ...base, annual_leave_adjustment: -1.5, annual_leave_remaining: 7.5, available: 7.5 }} />);
    expect(screen.getByText("Penyesuaian")).toBeTruthy();
    expect(screen.getByText("-1,5 hari")).toBeTruthy();
  });

  it("shows sisa bawaan with its expiry only while active", () => {
    const active = { ...base, carry_over_days: 2, carry_over_remaining: 2, carry_over_expires_on: "2026-03-31", carry_over_active: true, available: 11 };
    const { rerender } = render(<LeaveBalanceBreakdown balance={active} />);
    expect(screen.getByText("Sisa bawaan")).toBeTruthy();
    expect(screen.getByText(/2 hari, berlaku s\/d/)).toBeTruthy();
    expect(screen.getByText(/2026/)).toBeTruthy();

    rerender(<LeaveBalanceBreakdown balance={{ ...active, carry_over_active: false }} />);
    expect(screen.queryByText("Sisa bawaan")).toBeNull();
  });
});

describe("balanceSubtitle", () => {
  it("summarises jatah and terpakai", () => {
    expect(balanceSubtitle(base)).toBe("Jatah 12 hari · terpakai 3 hari");
  });

  it("mentions the active carry-over", () => {
    expect(
      balanceSubtitle({ ...base, carry_over_active: true, carry_over_remaining: 2, carry_over_expires_on: "2026-03-31", available: 11 })
    ).toContain("termasuk 2 hari sisa bawaan");
  });
});
