import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { LoanRow } from "../types";
import { LoansTable } from "./loans-table";

const row: LoanRow = {
  id: "k1", employee_id: "e1", loan_type: "kasbon", principal_amount: "1500000", interest_rate: 0, tenor_months: 2,
  monthly_installment: "750000", remaining_balance: "1500000", paid_amount: "0", first_installment_month: null,
  first_installment_year: null, status: "manager_approved", manager_approver_id: "m1",
  purpose: "Biaya rumah sakit orang tua, rawat inap tiga hari di RS Hasan Sadikin", notes: "Kuitansi menyusul",
  rejection_reason: null, created_at: "2026-10-29T03:00:00Z",
  employee: { id: "e1", full_name: "Peminjam K", nip: "N09", department: { name: "Toko" } },
};

afterEach(cleanup);

describe("LoansTable — Detail (EPIC-072)", () => {
  it("Detail menampilkan keperluan utuh & catatan; Tolak dari dialog memanggil onDecide(false)", () => {
    const onDecide = vi.fn();
    render(<LoansTable rows={[row]} loading={false} decidingId={null} onDecide={onDecide} />);
    fireEvent.click(screen.getByRole("button", { name: "Detail pinjaman Peminjam K" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText(row.purpose!)).toBeTruthy();
    expect(within(dialog).getByText("Kuitansi menyusul")).toBeTruthy();
    fireEvent.click(within(dialog).getByRole("button", { name: "Tolak" }));
    expect(onDecide).toHaveBeenCalledWith("k1", false);
  });
});

describe("LoansTable — lampiran & bulan potong (EPIC-076)", () => {
  it("HR melihat dokumen pendukung, jaminan, dan bulan potong yang tersimpan", () => {
    const pinjaman: LoanRow = {
      ...row, id: "p1", loan_type: "loan", status: "approved", first_installment_month: 12, first_installment_year: 2026,
      attachments: ["loan-attachments/e1/1-0123456789abcdef.pdf"],
      collateral_attachments: ["loan-attachments/e1/2-0123456789abcdef.jpg", "loan-attachments/e1/3-0123456789abcdef.pdf"],
    };
    render(<LoansTable rows={[pinjaman]} loading={false} decidingId={null} onDecide={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Detail pinjaman Peminjam K" }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Pinjaman Karyawan")).toBeTruthy();
    expect(within(dialog).getByText("Desember 2026")).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: /Jaminan 2/ })).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: /Dokumen 1/ })).toBeTruthy();
  });
});

describe("LoansTable — HR mewakili atasan (EPIC-092 R5)", () => {
  const pending: LoanRow = { ...row, id: "k2", status: "pending", manager_approver_id: "m1" };

  it("pengajuan di tahap atasan: HR bisa Setujui (mewakili atasan) dan Tolak", () => {
    const onDecide = vi.fn();
    render(<LoansTable rows={[pending]} loading={false} decidingId={null} onDecide={onDecide} />);
    expect(screen.getByText("Menunggu atasan (SPV/Manager)")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Setujui (mewakili atasan)" }));
    expect(onDecide).toHaveBeenCalledWith("k2", true);
    fireEvent.click(screen.getByRole("button", { name: "Tolak" }));
    expect(onDecide).toHaveBeenLastCalledWith("k2", false);
  });

  it("SPV sudah setuju: label menunggu Manager", () => {
    render(<LoansTable rows={[{ ...pending, supervisor_approver_id: "s1", supervisor_approved_at: "2026-11-05T03:00:00Z" }]}
      loading={false} decidingId={null} onDecide={vi.fn()} />);
    expect(screen.getByText("Disetujui SPV — menunggu Manager")).toBeTruthy();
  });

  it("Detail tahap atasan: tombol mewakili ikut tampil", () => {
    const onDecide = vi.fn();
    render(<LoansTable rows={[pending]} loading={false} decidingId={null} onDecide={onDecide} />);
    fireEvent.click(screen.getByRole("button", { name: "Detail pinjaman Peminjam K" }));
    fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Setujui (mewakili atasan)" }));
    expect(onDecide).toHaveBeenCalledWith("k2", true);
  });
});
