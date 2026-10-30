import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EssLoanRow } from "../types";
import { EssPinjamanPage } from "./pinjaman-page";

const decide = vi.fn();
const toastError = vi.fn();

vi.mock("sonner", () => ({ toast: { error: (m: string) => toastError(m), success: vi.fn() } }));

const teamLoan: EssLoanRow = {
  id: "k1", employee: { full_name: "Anggota K", nip: "N09" }, loan_type: "kasbon", principal_amount: "1500000",
  monthly_installment: "750000", remaining_balance: "1500000", paid_amount: "0", tenor_months: 2,
  first_installment_month: null, first_installment_year: null, status: "pending",
  purpose: "Biaya rumah sakit orang tua", notes: "Rawat inap 3 hari", rejection_reason: null,
  created_at: "2026-10-29T03:00:00Z",
};

vi.mock("../queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "boss" } }, isLoading: false }),
  useMyLoans: () => ({ data: [] }),
  useTeamLoanApprovals: () => ({ data: [teamLoan] }),
}));
vi.mock("../mutations", () => ({
  useSubmitLoan: () => ({ mutate: vi.fn(), isPending: false }),
  useDecideTeamLoan: () => ({ mutate: decide, isPending: false }),
}));

afterEach(() => {
  cleanup();
  decide.mockReset();
  toastError.mockReset();
});

describe("EssPinjamanPage — kasbon tim (EPIC-072)", () => {
  it("Detail menampilkan keperluan dan catatan", () => {
    render(<EssPinjamanPage />);
    fireEvent.click(screen.getByRole("button", { name: "Detail kasbon Anggota K" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Biaya rumah sakit orang tua")).toBeTruthy();
    expect(within(dialog).getByText("Rawat inap 3 hari")).toBeTruthy();
    expect(within(dialog).getByText("2 bulan")).toBeTruthy();
  });

  it("Tolak wajib alasan; alasan dikirim ke server", () => {
    render(<EssPinjamanPage />);
    fireEvent.click(screen.getByRole("button", { name: "Tolak kasbon Anggota K" }));
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Alasan penolakan wajib diisi");
    expect(decide).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("Alasan penolakan"), { target: { value: " Plafon bulan ini habis " } });
    fireEvent.click(screen.getByRole("button", { name: "Tolak pengajuan" }));
    expect(decide).toHaveBeenCalledWith(
      { id: "k1", approved: false, rejectionReason: "Plafon bulan ini habis" },
      expect.anything()
    );
  });
});
