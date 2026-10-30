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
