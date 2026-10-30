import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
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
let myEmployeeId = "hr-1";
vi.mock("@/features/hris/ess/queries", () => ({ useEssMe: () => ({ data: { employee: { id: myEmployeeId } } }) }));
vi.mock("../queries", () => ({
  useLeaveList: () => ({
    isLoading: false,
    data: [
      {
        id: "l1",
        employee_id: "e1",
        status: "pending",
        leave_type: "annual",
        start_date: "2026-10-27",
        end_date: "2026-10-28",
        total_days: 2,
        reason: "Acara keluarga di Garut.\nBerangkat Senin pagi.",
        attachment_url: "leave-attachments/e1/undangan.png",
        created_at: "2026-10-26T01:00:00Z",
        employee: { full_name: "Budi Santoso", nip: "N01", department: { name: "Dapur" } },
      },
    ],
  }),
}));

afterEach(() => {
  cleanup();
  access = {};
  myEmployeeId = "hr-1";
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

describe("LeavesPage: detail & lampiran (EPIC-072)", () => {
  it("kartu menampilkan lampiran dengan pratinjau; Detail memuat alasan utuh dan tombol keputusan", () => {
    grant(true);
    render(<LeavesPage />);
    fireEvent.click(screen.getByRole("button", { name: "Lampiran 1 cuti Budi Santoso" }));
    expect(screen.getByAltText("Lampiran 1").getAttribute("src")).toBe(
      "/api/hris/leaves/attachment/leave-attachments/e1/undangan.png");
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Detail cuti Budi Santoso" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText((_, el) => el?.tagName === "P" && el.textContent === "Acara keluarga di Garut.\nBerangkat Senin pagi.")).toBeTruthy();
    expect(within(dialog).getByText("tombol-keputusan")).toBeTruthy();
  });

  it("tanpa akses approve: Detail tetap bisa dibuka, tanpa tombol keputusan", () => {
    grant(false);
    render(<LeavesPage />);
    fireEvent.click(screen.getByRole("button", { name: "Detail cuti Budi Santoso" }));
    expect(within(screen.getByRole("dialog")).queryByText("tombol-keputusan")).toBeNull();
  });
});

describe("LeavesPage: pengajuan sendiri (EPIC-072 Review Focus #4)", () => {
  it("pemegang approve tidak melihat tombol keputusan pada cutinya sendiri, di kartu maupun Detail", () => {
    grant(true);
    myEmployeeId = "e1";
    render(<LeavesPage />);
    expect(screen.queryByText("tombol-keputusan")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Detail cuti Budi Santoso" }));
    expect(within(screen.getByRole("dialog")).queryByText("tombol-keputusan")).toBeNull();
  });
});
