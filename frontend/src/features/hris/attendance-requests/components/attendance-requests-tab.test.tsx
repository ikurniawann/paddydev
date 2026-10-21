import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AttendanceRequest } from "../types";
import { AttendanceRequestsTab } from "./attendance-requests-tab";

const decide = vi.fn();
const toastError = vi.fn();
const rows: AttendanceRequest[] = [];
vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));
vi.mock("@/features/hris/ess/queries", () => ({ useEssMe: () => ({ data: { employee: { id: "hr1" } } }) }));
vi.mock("../queries", () => ({ useAttendanceRequests: () => ({ data: rows, isError: false }) }));
vi.mock("../mutations", () => ({
  useDecideAttendanceRequest: () => ({ mutate: decide, isPending: false, variables: undefined }),
}));
vi.mock("@/components/ui/select", () => ({
  Select: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

function req(over: Partial<AttendanceRequest>): AttendanceRequest {
  return {
    id: "r1", employee_id: "e1", date: "2026-10-03", kind: "correction", clock_in: null, clock_out: "17:00",
    reason: "Lupa absen pulang, HP mati", attachment_path: "attendance-requests/e1/a.png", status: "pending",
    attendance_id: null, decided_by: null, decided_by_name: null, decided_at: null, rejection_reason: null,
    created_at: "2026-10-04T01:00:00Z", updated_at: "2026-10-04T01:00:00Z", employee: { id: "e1", full_name: "Budi Santoso", nip: "N01" },
    recorded: { id: "a1", clock_in: "2026-10-03T01:05:00Z", clock_out: null, status: "present" }, ...over,
  };
}

afterEach(() => {
  cleanup();
  decide.mockReset();
  toastError.mockReset();
  rows.length = 0;
});

describe("AttendanceRequestsTab", () => {
  it("menampilkan pengajuan, jam yang diajukan, dan foto bukti; Setujui langsung dikirim", () => {
    rows.push(req({}));
    render(<AttendanceRequestsTab />);
    expect(screen.getByText("Budi Santoso")).toBeTruthy();
    expect(screen.getByText("Koreksi absen")).toBeTruthy();
    expect(screen.getByText("Pulang 17:00")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Lihat foto bukti" }).getAttribute("href")).toBe(
      "/api/hris/attendance-requests/attachment/attendance-requests/e1/a.png"
    );
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(decide).toHaveBeenCalledWith({ id: "r1", action: "approve", rejectionReason: undefined }, expect.anything());
  });

  it("Tolak wajib alasan", () => {
    rows.push(req({}));
    render(<AttendanceRequestsTab />);
    fireEvent.click(screen.getByRole("button", { name: "Tolak" }));
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Alasan penolakan wajib diisi");
    expect(decide).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Alasan penolakan"), { target: { value: "CCTV menunjukkan tidak hadir" } });
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(decide).toHaveBeenCalledWith(
      { id: "r1", action: "reject", rejectionReason: "CCTV menunjukkan tidak hadir" },
      expect.anything()
    );
  });

  it("tanpa tombol keputusan untuk pengajuan HRD sendiri atau yang sudah diputus", () => {
    rows.push(req({ id: "own", employee_id: "hr1" }), req({ id: "done", status: "approved", decided_by_name: "HR Pemutus" }));
    render(<AttendanceRequestsTab />);
    expect(screen.queryByRole("button", { name: "Setujui" })).toBeNull();
    expect(screen.getByText("oleh HR Pemutus")).toBeTruthy();
  });
});
