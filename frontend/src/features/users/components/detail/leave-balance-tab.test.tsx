import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LeaveBalanceView } from "@/features/hris/leave-balances/types";
import { LeaveBalanceTab } from "./leave-balance-tab";

const hooks = vi.hoisted(() => ({
  useLeaveBalance: vi.fn(),
  useLeaveBalanceAccess: vi.fn(),
}));
vi.mock("@/features/hris/leave-balances/queries", () => hooks);
vi.mock("@/features/hris/leave-balances/components/leave-balance-edit-dialog", () => ({
  LeaveBalanceEditDialog: ({ balance }: { balance: LeaveBalanceView }) => <div>dialog-ubah-{balance.year}</div>,
}));

const view: LeaveBalanceView = {
  employee_id: "e1",
  nip: "2025001",
  full_name: "Ana Lestari",
  branch_name: "PPJA",
  year: 2026,
  annual_leave_total: 12,
  annual_leave_adjustment: -1,
  annual_leave_used: 3,
  annual_leave_remaining: 8,
  carry_over_days: 2,
  carry_over_used: 0.5,
  carry_over_remaining: 1.5,
  carry_over_expires_on: "2027-03-31",
  carry_over_active: true,
  carry_over_source: "auto",
  available: 9.5,
  updated_at: null,
  exists: true,
};

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-26T03:00:00Z"));
  hooks.useLeaveBalance.mockReset();
  hooks.useLeaveBalanceAccess.mockReset();
  hooks.useLeaveBalance.mockReturnValue({ data: view, isLoading: false, isError: false });
  hooks.useLeaveBalanceAccess.mockReturnValue({ data: { read: true, update: true, import: false, export: false } });
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("LeaveBalanceTab", () => {
  it("meminta saldo tahun berjalan dan menampilkan rincian dari bentuk objek tunggal Go", () => {
    render(<LeaveBalanceTab employeeId="e1" />);
    expect(hooks.useLeaveBalance).toHaveBeenCalledWith("e1", 2026);
    expect(screen.getByTestId("lb-available").textContent).toBe("9,5");
    expect(screen.getByTestId("lb-total").textContent).toBe("12");
    expect(screen.getByTestId("lb-adjustment").textContent).toBe("-1");
    expect(screen.getByTestId("lb-used").textContent).toBe("3");
    expect(screen.getByTestId("lb-remaining").textContent).toBe("8");
    expect(screen.getByTestId("lb-carry").textContent).toBe("1,5");
    expect(screen.getByText(/berlaku sampai 31 Mar 2027/)).toBeTruthy();
  });

  it("sisa bawaan yang sudah tidak berlaku diberi tanda", () => {
    hooks.useLeaveBalance.mockReturnValue({
      data: { ...view, carry_over_active: false, carry_over_expires_on: "2026-03-31", available: 8 },
      isLoading: false,
      isError: false,
    });
    render(<LeaveBalanceTab employeeId="e1" />);
    expect(screen.getByText(/tidak berlaku/i)).toBeTruthy();
  });

  it("tombol Ubah hanya untuk akun dengan aksi update", () => {
    render(<LeaveBalanceTab employeeId="e1" />);
    fireEvent.click(screen.getByRole("button", { name: "Ubah saldo" }));
    expect(screen.getByText("dialog-ubah-2026")).toBeTruthy();
    cleanup();
    hooks.useLeaveBalanceAccess.mockReturnValue({ data: { read: true, update: false, import: false, export: false } });
    render(<LeaveBalanceTab employeeId="e1" />);
    expect(screen.queryByRole("button", { name: "Ubah saldo" })).toBeNull();
    cleanup();
    hooks.useLeaveBalanceAccess.mockReturnValue({ data: undefined });
    render(<LeaveBalanceTab employeeId="e1" />);
    expect(screen.queryByRole("button", { name: "Ubah saldo" })).toBeNull();
  });

  it("tahun lalu hanya dilihat: tombol Ubah tidak ada", () => {
    render(<LeaveBalanceTab employeeId="e1" />);
    fireEvent.change(screen.getByLabelText("Tahun"), { target: { value: "2025" } });
    expect(hooks.useLeaveBalance).toHaveBeenLastCalledWith("e1", 2025);
    expect(screen.queryByRole("button", { name: "Ubah saldo" })).toBeNull();
  });

  it("menampilkan galat dan keadaan memuat", () => {
    hooks.useLeaveBalance.mockReturnValue({ data: undefined, isLoading: false, isError: true, error: new Error("Forbidden") });
    render(<LeaveBalanceTab employeeId="e1" />);
    expect(screen.getByText("Forbidden")).toBeTruthy();
    cleanup();
    hooks.useLeaveBalance.mockReturnValue({ data: undefined, isLoading: true, isError: false });
    const { container } = render(<LeaveBalanceTab employeeId="e1" />);
    expect(container.querySelector("[data-testid='lb-available']")).toBeNull();
  });
});
