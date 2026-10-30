import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { OvertimePage } from "./overtime-page";

let access: { data?: { attendance: boolean; leaves: boolean; overtime: boolean } } = {};
vi.mock("@/features/hris/approve-access", () => ({ useHrApproveAccess: () => access }));
vi.mock("@/features/hris/ess/queries", () => ({ useEssMe: () => ({ data: { employee: { id: "hr-1" } } }) }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("../mutations", () => ({ useDecideOvertime: () => ({ mutate: vi.fn(), isPending: false }) }));
vi.mock("../queries", () => ({
  useOvertimeRequests: () => ({
    isLoading: false,
    data: [
      {
        id: "o1", employee_id: "e1", date: "2026-10-06T00:00:00.000Z", start_time: "17:00:00", end_time: "19:00:00",
        hours: "2.00", source: "employee", status: "pending", reason: "Closing", rejection_reason: null,
        day_type: "regular", employee: { id: "e1", full_name: "Crew Koreksi", nip: "N01" },
      },
    ],
  }),
}));
vi.mock("./overtime-assign-dialog", () => ({ OvertimeAssignDialog: () => null }));
vi.mock("./overtime-correction-dialog", () => ({ OvertimeCorrectionDialog: () => null }));

afterEach(() => {
  cleanup();
  access = {};
});

describe("OvertimePage: keputusan HR mengikuti akses approve (EPIC-067 R14)", () => {
  it("overtime=true: Setujui, Tolak, dan Jenis hari tampil", () => {
    access = { data: { attendance: false, leaves: false, overtime: true } };
    render(<OvertimePage />);
    expect(screen.getByRole("button", { name: "Setujui" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Tolak" })).toBeTruthy();
    expect(screen.getByLabelText("Jenis hari")).toBeTruthy();
  });

  it("overtime=false atau data belum ada: tidak dirender", () => {
    access = { data: { attendance: true, leaves: true, overtime: false } };
    const first = render(<OvertimePage />);
    expect(screen.queryByRole("button", { name: "Setujui" })).toBeNull();
    expect(screen.queryByLabelText("Jenis hari")).toBeNull();
    first.unmount();
    access = {};
    render(<OvertimePage />);
    expect(screen.queryByRole("button", { name: "Setujui" })).toBeNull();
  });
});
