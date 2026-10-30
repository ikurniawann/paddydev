import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { BusinessTrip } from "../types";
import { TeamTripApprovals } from "./team-trip-approvals";

const request = vi.fn();
const trip: BusinessTrip = {
  id: "t1", employee_id: "e1", start_date: "2026-11-03", end_date: "2026-11-05", destination: "Surabaya",
  purpose: "Pembukaan toko baru.\nMemeriksa display dan stok awal bersama tim regional.", status: "pending",
  requested_by: null, decided_by: null, decided_by_name: null, decided_at: null, rejection_reason: null,
  created_at: "2026-10-30T02:00:00Z", employee: { id: "e1", full_name: "Anggota D", nip: "N04", reporting_to: "boss" },
};

vi.mock("../queries", () => ({ useBusinessTrips: () => ({ data: [trip] }) }));
vi.mock("../use-trip-decision", () => ({
  useTripDecision: () => ({ request, busyId: null, dialog: null }),
}));

afterEach(() => {
  cleanup();
  request.mockReset();
});

describe("TeamTripApprovals (EPIC-072)", () => {
  it("Detail menampilkan tujuan & keperluan utuh, lalu Setujui/Tolak memakai alur yang sama", () => {
    render(<TeamTripApprovals />);
    fireEvent.click(screen.getByRole("button", { name: "Detail perjalanan dinas Anggota D" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Surabaya")).toBeTruthy();
    expect(within(dialog).getByText((_, el) => el?.tagName === "P" && el.textContent === trip.purpose)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Tolak" }));
    expect(request).toHaveBeenCalledWith(trip, "reject");
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
