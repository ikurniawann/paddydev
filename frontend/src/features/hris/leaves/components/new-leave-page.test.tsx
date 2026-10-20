import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LeaveQuota } from "../types";
import { NewLeavePage } from "./new-leave-page";

const mutateAsync = vi.fn();

const employees = [
  { id: "e1", full_name: "Sari Supervisor", nip: "N001", department: { name: "Ops" } },
  { id: "e2", full_name: "Budi Staff", nip: "N002", department: { name: "Ops" } },
];

function quota(id: string, leaderEligible: boolean): LeaveQuota {
  return {
    employee_id: id,
    month: "2026-10",
    position_level: leaderEligible ? "supervisor" : "staff",
    leader_eligible: leaderEligible,
    types: [],
  };
}
const quotas: Record<string, LeaveQuota> = { e1: quota("e1", true), e2: quota("e2", false) };

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("../queries", () => ({
  useLeaveEmployees: () => ({ data: employees }),
  useLeaveQuota: (_month: string, employeeId?: string) => ({
    data: employeeId ? quotas[employeeId] : undefined,
  }),
}));
vi.mock("../mutations", () => ({
  useCreateLeave: () => ({ mutateAsync, isPending: false }),
}));
vi.mock("./leave-attachment-field", () => ({ LeaveAttachmentField: () => null }));
vi.mock("./leave-success-dialog", () => ({ LeaveSuccessDialog: () => null }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

function chooseEmployee(search: string, name: string) {
  const input = screen.getByPlaceholderText("Cari nama atau NIP karyawan...");
  fireEvent.focus(input);
  fireEvent.change(input, { target: { value: search } });
  fireEvent.click(screen.getByText(name));
}

/** Base UI Select commits on the full pointer sequence, not on a bare click. */
function pickOption(name: string) {
  const option = screen.getByRole("option", { name });
  fireEvent.pointerDown(option);
  fireEvent.mouseDown(option);
  fireEvent.pointerUp(option);
  fireEvent.mouseUp(option);
  fireEvent.click(option);
}

const typeTrigger = () => screen.getByRole("combobox");

function typeOptions() {
  fireEvent.click(typeTrigger());
  return screen.getAllByRole("option").map((o) => o.textContent ?? "");
}

beforeEach(() => {
  mutateAsync.mockReset();
  mutateAsync.mockResolvedValue({ data: { id: "l1" } });
});
afterEach(cleanup);

describe("NewLeavePage", () => {
  it("offers Izin Leader only for a leader-eligible employee", () => {
    render(<NewLeavePage />);
    chooseEmployee("Budi", "Budi Staff");
    const staffOptions = typeOptions();
    expect(staffOptions).not.toContain("Izin Leader");
    expect(staffOptions).toContain("Izin Pribadi");
    expect(staffOptions).not.toContain("Cuti Haid");
  });

  it("falls back to Cuti Tahunan, and submits annual, when the employee changes to a non-eligible one", async () => {
    render(<NewLeavePage />);
    chooseEmployee("Sari", "Sari Supervisor");
    expect(typeOptions()).toContain("Izin Leader");
    pickOption("Izin Leader");
    await waitFor(() => expect(typeTrigger().textContent).toContain("Izin Leader"));

    chooseEmployee("Budi", "Budi Staff");
    await waitFor(() => expect(typeTrigger().textContent).toContain("Cuti Tahunan"));
    expect(typeTrigger().textContent).not.toContain("Izin Leader");

    fireEvent.change(document.getElementById("start_date")!, { target: { value: "2026-10-21" } });
    fireEvent.change(document.getElementById("end_date")!, { target: { value: "2026-10-22" } });
    fireEvent.change(document.getElementById("reason")!, {
      target: { value: "keperluan keluarga mendadak" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Ajukan Cuti/ }));

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1));
    expect(mutateAsync.mock.calls[0][0]).toMatchObject({ employee_id: "e2", leave_type: "annual" });
  });
});
