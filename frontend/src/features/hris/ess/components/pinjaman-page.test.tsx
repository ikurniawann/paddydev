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

const teamPinjaman: EssLoanRow = {
  ...teamLoan, id: "p1", employee: { full_name: "Anggota P", nip: "N10" }, loan_type: "loan",
  principal_amount: "6000000", tenor_months: 6, purpose: "Renovasi rumah", notes: null,
  created_at: "2026-11-20T17:30:00Z", attachments: [],
  collateral_attachments: ["loan-attachments/p/1-0123456789abcdef.pdf"],
};

vi.mock("@/lib/dates", () => ({ todayWib: () => "2026-11-21" }));
vi.mock("../queries", () => ({
  useEssMe: () => ({ data: { employee: { id: "boss" } }, isLoading: false }),
  useMyLoans: () => ({ data: [] }),
  useTeamLoanApprovals: () => ({ data: [teamLoan, teamPinjaman] }),
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
    expect(within(dialog).getByText("2 kali × Rp 750.000", { exact: false })).toBeTruthy();
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

  it("baris kasbon tim boleh turun baris di HP agar nama & keperluan tidak terjepit (review #8)", () => {
    render(<EssPinjamanPage />);
    const row = screen.getByRole("button", { name: "Detail kasbon Anggota K" }).closest("li");
    expect(row?.className).toContain("flex-wrap");
  });
});

describe("EssPinjamanPage — prosedur kasbon & Pinjaman Karyawan (EPIC-076)", () => {
  it("atasan melihat detail Pinjaman Karyawan beserta jaminan dan bulan potong", () => {
    render(<EssPinjamanPage />);
    expect(screen.getByText("Persetujuan Kasbon & Pinjaman Tim")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Detail pinjaman Anggota P" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Detail pinjaman")).toBeTruthy();
    expect(within(dialog).getByText("Pinjaman Karyawan")).toBeTruthy();
    expect(within(dialog).getByText("Jaminan (surat berharga)")).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: /Jaminan 1/ })).toBeTruthy();
    // Diajukan 21 Nov 00.30 WIB → payroll Desember.
    expect(within(dialog).getByText("Desember 2026 (perkiraan)")).toBeTruthy();
  });

  it("form kasbon menampilkan syarat, perkiraan bulan potong, dan wajib dokumen pendukung", () => {
    render(<EssPinjamanPage />);
    fireEvent.click(screen.getByRole("button", { name: /Ajukan Pinjaman/ }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(/masa kerja minimal 3 bulan/)).toBeTruthy();
    expect(within(dialog).getByText("Desember 2026")).toBeTruthy();
    expect(within(dialog).getByText("Dokumen pendukung (wajib, 1–3 file PDF/foto)")).toBeTruthy();
    fireEvent.change(within(dialog).getByRole("spinbutton"), { target: { value: "1000000" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "Kirim Pengajuan" }));
    expect(toastError).toHaveBeenCalledWith("Lampirkan minimal 1 dokumen pendukung kasbon");
  });
});
