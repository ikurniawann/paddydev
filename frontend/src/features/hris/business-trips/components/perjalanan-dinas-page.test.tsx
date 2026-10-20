import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { BusinessTrip } from "../types";
import { EssPerjalananDinasPage } from "./perjalanan-dinas-page";

const decide = vi.fn();
const toastError = vi.fn();
const toastSuccess = vi.fn();
const mine: BusinessTrip[] = [];
const team: BusinessTrip[] = [];

vi.mock("sonner", () => ({
  toast: { error: (m: string) => toastError(m), success: (m: string) => toastSuccess(m) },
}));
vi.mock("@/features/hris/ess/queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "me1" } }, isLoading: false }),
}));
vi.mock("../queries", () => ({
  useBusinessTrips: (f: { scope?: string }) => ({ data: f.scope === "approvals" ? team : mine }),
}));
vi.mock("../mutations", () => ({
  useDecideBusinessTrip: () => ({ mutate: decide, isPending: false, variables: undefined }),
  useCreateBusinessTrip: () => ({ mutate: vi.fn(), isPending: false }),
}));

function trip(over: Partial<BusinessTrip>): BusinessTrip {
  return {
    id: "t1", employee_id: "e1", start_date: "2099-03-02", end_date: "2099-03-04", destination: "Gudang Cikarang",
    purpose: "Stock opname", status: "pending", requested_by: "e1", decided_by: null, decided_by_name: null,
    decided_at: null, rejection_reason: null, created_at: "2099-03-01T00:00:00Z",
    employee: { id: "e1", full_name: "Budi Santoso", nip: "N01", reporting_to: "me1" }, ...over,
  };
}

afterEach(() => {
  cleanup();
  decide.mockReset();
  toastError.mockReset();
  toastSuccess.mockReset();
  mine.length = 0;
  team.length = 0;
});

describe("EssPerjalananDinasPage", () => {
  it("atasan melihat antrean tim dan menyetujui langsung", () => {
    team.push(trip({}));
    decide.mockImplementation((_v, opts) => opts.onSuccess({ message: "Perjalanan Dinas disetujui" }));
    render(<EssPerjalananDinasPage />);
    expect(screen.getByText(/Budi Santoso/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(decide).toHaveBeenCalledWith({ id: "t1", action: "approve", rejectionReason: undefined }, expect.anything());
    expect(toastSuccess).toHaveBeenCalledWith("Perjalanan Dinas disetujui");
  });

  it("menolak wajib alasan lewat dialog konfirmasi, lalu mengirim alasan", () => {
    team.push(trip({}));
    render(<EssPerjalananDinasPage />);
    fireEvent.click(screen.getByRole("button", { name: "Tolak" }));
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Alasan penolakan wajib diisi");
    expect(decide).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Alasan penolakan"), { target: { value: "  Jadwal bentrok " } });
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(decide).toHaveBeenCalledWith({ id: "t1", action: "reject", rejectionReason: "Jadwal bentrok" }, expect.anything());
  });

  it("pemohon hanya bisa membatalkan saat menunggu, lewat dialog konfirmasi", () => {
    mine.push(
      trip({ id: "p", status: "pending" }),
      trip({ id: "a", status: "approved" }),
      trip({ id: "r", status: "rejected", rejection_reason: "Bentrok" }),
      trip({ id: "c", status: "cancelled" })
    );
    render(<EssPerjalananDinasPage />);
    const cancels = screen.getAllByRole("button", { name: "Batalkan" });
    expect(cancels).toHaveLength(1);
    expect(screen.getByText(/Ditolak: Bentrok/)).toBeTruthy();
    fireEvent.click(cancels[0]);
    expect(decide).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Batalkan pengajuan" }));
    expect(decide).toHaveBeenCalledWith({ id: "p", action: "cancel", rejectionReason: undefined }, expect.anything());
  });

  it("menampilkan pesan error server apa adanya", () => {
    team.push(trip({}));
    decide.mockImplementation((_v, opts) => opts.onError(new Error("Pengajuan sudah diproses oleh orang lain")));
    render(<EssPerjalananDinasPage />);
    fireEvent.click(screen.getByRole("button", { name: "Setujui" }));
    expect(toastError).toHaveBeenCalledWith("Pengajuan sudah diproses oleh orang lain");
  });
});
