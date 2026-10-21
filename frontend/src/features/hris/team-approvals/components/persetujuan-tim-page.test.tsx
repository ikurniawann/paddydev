import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TeamApprovals, TeamLeaveRequest, TeamOvertimeRequest } from "../types";
import { EssPersetujuanTimPage } from "./persetujuan-tim-page";

const decide = vi.fn();
const toastError = vi.fn();
const toastSuccess = vi.fn();
let queue: TeamApprovals = { leaves: [], overtime: [] };
let loans: { id: string }[] = [];
let trips: { id: string }[] = [];

vi.mock("sonner", () => ({
  toast: { error: (m: string) => toastError(m), success: (m: string) => toastSuccess(m) },
}));
vi.mock("@/features/hris/ess/queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "boss" } }, isLoading: false }),
  useTeamLoanApprovals: () => ({ data: loans }),
}));
vi.mock("@/features/hris/business-trips", () => ({
  TeamTripApprovals: () => (trips.length > 0 ? <p>Antrean Perjalanan Dinas</p> : null),
  useBusinessTrips: () => ({ data: trips }),
}));
vi.mock("../queries", () => ({ useTeamApprovals: () => ({ data: queue, isLoading: false }) }));
vi.mock("../mutations", () => ({
  useDecideTeamRequest: () => ({ mutate: decide, isPending: false, variables: undefined }),
}));

const leave: TeamLeaveRequest = {
  id: "l1", employee_id: "e1", leave_type: "annual", start_date: "2031-03-04", end_date: "2031-03-05",
  total_days: "2.00", reason: "Acara keluarga", attachment_url: null, created_at: "2031-03-01T00:00:00Z",
  employee: { id: "e1", full_name: "Anggota A", nip: "N01" },
};
const overtime: TeamOvertimeRequest = {
  id: "o1", employee_id: "e2", date: "2026-10-06", start_time: "17:00:00", end_time: "19:30:00", hours: "2.50",
  day_type: "rest_day", reason: "Stock opname", created_at: "2026-10-05T00:00:00Z",
  employee: { id: "e2", full_name: "Anggota B", nip: "N02" },
};

afterEach(() => {
  cleanup();
  decide.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
  queue = { leaves: [], overtime: [] };
  loans = [];
  trips = [];
});

describe("EssPersetujuanTimPage", () => {
  it("menampilkan cuti & lembur tim dan menyetujui cuti langsung", () => {
    queue = { leaves: [leave], overtime: [overtime] };
    decide.mockImplementation((_v, opts) => opts.onSuccess());
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText(/Anggota A · Cuti Tahunan/)).toBeTruthy();
    expect(screen.getByText(/17:00–19:30 \(2,5 jam\) · Weekend \/ Libur/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Setujui cuti Anggota A" }));
    expect(decide).toHaveBeenCalledWith(
      { kind: "leave", id: "l1", action: "approve", rejectionReason: undefined },
      expect.anything()
    );
    expect(toastSuccess).toHaveBeenCalledWith("Cuti disetujui");
  });

  it("menolak lembur wajib alasan lewat dialog", () => {
    queue = { leaves: [], overtime: [overtime] };
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Tolak lembur Anggota B" }));
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Alasan penolakan wajib diisi");
    expect(decide).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Alasan penolakan"), { target: { value: "  Tidak ada stok masuk " } });
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(decide).toHaveBeenCalledWith(
      { kind: "overtime", id: "o1", action: "reject", rejectionReason: "Tidak ada stok masuk" },
      expect.anything()
    );
  });

  it("menampilkan pesan error server apa adanya", () => {
    queue = { leaves: [leave], overtime: [] };
    decide.mockImplementation((_v, opts) => opts.onError(new Error("Tidak bisa memutuskan pengajuan cuti sendiri")));
    render(<EssPersetujuanTimPage />);
    fireEvent.click(screen.getByRole("button", { name: "Setujui cuti Anggota A" }));
    expect(toastError).toHaveBeenCalledWith("Tidak bisa memutuskan pengajuan cuti sendiri");
  });

  it("Perjalanan Dinas memakai antrean bersama, kasbon ditautkan ke Pinjaman", () => {
    loans = [{ id: "k1" }, { id: "k2" }];
    trips = [{ id: "t1" }];
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText("Antrean Perjalanan Dinas")).toBeTruthy();
    const link = screen.getByRole("link", { name: /2 kasbon tim menunggu persetujuan Anda/ });
    expect(link.getAttribute("href")).toBe("/dashboard/me/pinjaman");
    expect(screen.queryByText("Tidak ada pengajuan tim yang menunggu persetujuan Anda.")).toBeNull();
  });

  it("menampilkan keadaan kosong", () => {
    render(<EssPersetujuanTimPage />);
    expect(screen.getByText("Tidak ada pengajuan tim yang menunggu persetujuan Anda.")).toBeTruthy();
  });
});
