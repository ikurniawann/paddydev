import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LeaveQuota } from "@/features/hris/leaves/types";
import { CutiRequestDialog } from "./cuti-request-dialog";

const mutate = vi.fn();
let quotaData: LeaveQuota | undefined;

vi.mock("@/features/hris/leaves/queries", () => ({
  useLeaveQuota: () => ({ data: quotaData }),
}));
vi.mock("../mutations", () => ({
  useSubmitMyLeave: () => ({ mutate, isPending: false }),
}));
vi.mock("@/features/hris/attendance/queries", () => ({
  useHolidayIndex: () => ({ data: undefined }),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

function quota(leaderEligible: boolean): LeaveQuota {
  return {
    employee_id: "e1",
    month: "2026-10",
    position_level: leaderEligible ? "supervisor" : "staff",
    leader_eligible: leaderEligible,
    types: [
      { leave_type: "personal", label: "Izin Pribadi", quota: 3, used: 0, remaining: 3, available: true },
      ...(leaderEligible
        ? [{ leave_type: "leader", label: "Izin Leader", quota: 1, used: 0, remaining: 1, available: true }]
        : []),
    ],
  };
}

function dialog() {
  return <CutiRequestDialog open onOpenChange={() => {}} balance={null} />;
}

function openTypeOptions() {
  fireEvent.click(screen.getAllByRole("combobox")[0]);
  return screen.getAllByRole("option").map((o) => o.textContent ?? "");
}

function pickType(label: string) {
  openTypeOptions();
  fireEvent.click(screen.getByRole("option", { name: new RegExp(label) }));
}

const dateInputs = () => Array.from(document.querySelectorAll<HTMLInputElement>('input[type="date"]'));

beforeEach(() => {
  // cmdk (inside Combobox) needs these browser APIs, which jsdom lacks.
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  );
  Element.prototype.scrollIntoView = vi.fn();
  mutate.mockReset();
  quotaData = undefined;
});
afterEach(cleanup);

describe("CutiRequestDialog", () => {
  it("hides Izin Leader and the removed types when the employee is not leader-eligible", () => {
    quotaData = quota(false);
    render(dialog());
    const options = openTypeOptions();
    expect(options.join("|")).not.toContain("Izin Leader");
    for (const removed of ["Izin Darurat", "Izin Tanpa Gaji", "Cuti Haid"]) {
      expect(options.join("|")).not.toContain(removed);
    }
    expect(options.join("|")).toContain("Izin Pribadi");
  });

  it("makes Izin Leader a one-day request with the quota note when eligible", () => {
    quotaData = quota(true);
    render(dialog());
    expect(openTypeOptions().join("|")).toContain("Izin Leader");
    fireEvent.click(screen.getByRole("option", { name: /Izin Leader/ }));

    const [start, end] = dateInputs();
    expect(end.disabled).toBe(true);
    fireEvent.change(start, { target: { value: "2026-10-21" } });
    expect(dateInputs()[1].value).toBe("2026-10-21");
    expect(dateInputs()[1].disabled).toBe(true);
    expect(screen.getByText(/Sisa kuota Izin Leader bulan 2026-10: 1 dari 1 hari/)).toBeTruthy();
  });

  it("shows and submits Cuti Tahunan when the stored Izin Leader is no longer eligible", () => {
    quotaData = quota(true);
    const { rerender } = render(dialog());
    pickType("Izin Leader");
    expect(screen.getAllByRole("combobox")[0].textContent).toContain("Izin Leader");

    quotaData = undefined; // eligibility lost (quota unreadable)
    rerender(dialog());
    expect(screen.getAllByRole("combobox")[0].textContent).toContain("Cuti Tahunan");

    const [start, end] = dateInputs();
    fireEvent.change(start, { target: { value: "2026-10-21" } });
    fireEvent.change(end, { target: { value: "2026-10-22" } });
    fireEvent.change(screen.getByPlaceholderText(/keperluan keluarga/), {
      target: { value: "keperluan keluarga mendadak" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Kirim Pengajuan" }));

    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate.mock.calls[0][0].form.leave_type).toBe("annual");
  });
});
