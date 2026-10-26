import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LeavesPage } from "./leaves-page";

let access: { data?: { attendance: boolean; leaves: boolean; overtime: boolean } } = {};
const grant = (leaves: boolean) => {
  access = { data: { attendance: false, leaves, overtime: false } };
};

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/features/hris/approve-access", () => ({ useHrApproveAccess: () => access }));
vi.mock("@/components/hris/ApprovalButtons", () => ({
  ApprovalButtons: () => <button>tombol-keputusan</button>,
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("../queries", () => ({
  useLeaveList: () => ({
    isLoading: false,
    data: [
      {
        id: "l1",
        status: "pending",
        leave_type: "annual",
        start_date: "2026-10-27",
        end_date: "2026-10-28",
        total_days: 2,
        reason: "Acara keluarga",
        created_at: "2026-10-26T01:00:00Z",
        employee: { full_name: "Budi Santoso", nip: "N01", department: { name: "Dapur" } },
      },
    ],
  }),
}));

afterEach(() => {
  cleanup();
  access = {};
});

describe("LeavesPage: tombol keputusan mengikuti akses approve (EPIC-067 R14)", () => {
  it("leaves=true: tombol keputusan tampil untuk pengajuan pending", () => {
    grant(true);
    render(<LeavesPage />);
    expect(screen.getByText("tombol-keputusan")).toBeTruthy();
  });

  it("leaves=false: tombol keputusan tidak dirender, daftar tetap tampil", () => {
    grant(false);
    render(<LeavesPage />);
    expect(screen.queryByText("tombol-keputusan")).toBeNull();
    expect(screen.getByText("Budi Santoso")).toBeTruthy();
  });

  it("selama memuat atau gagal (data kosong): tombol tidak tampil", () => {
    access = {};
    render(<LeavesPage />);
    expect(screen.queryByText("tombol-keputusan")).toBeNull();
  });
});
