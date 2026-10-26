import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CarryReview, LeaveBalanceView } from "../types";
import { LeaveCarryReviewsCard } from "./leave-carry-reviews-card";

const mutate = vi.hoisted(() => vi.fn());
vi.mock("../mutations", () => ({ useResolveCarryReview: () => ({ mutate, isPending: false }) }));
const q = vi.hoisted(() => ({ useCarryReviews: vi.fn(), useLeaveBalance: vi.fn() }));
vi.mock("../queries", () => q);
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({ toast }));
vi.mock("./leave-balance-edit-dialog", () => ({
  LeaveBalanceEditDialog: ({ balance, onOpenChange }: { balance: LeaveBalanceView; onOpenChange: (o: boolean) => void }) => (
    <div>
      dialog-ubah-{balance.full_name}-{balance.year}
      <button onClick={() => onOpenChange(false)}>tutup-ubah</button>
    </div>
  ),
}));

const review = (over: Partial<CarryReview> = {}): CarryReview => ({
  id: "r1",
  employee_id: "e1",
  nip: "2025001",
  full_name: "Ana Lestari",
  branch_name: "PPJA",
  year: 2027,
  carry_over_days: 2,
  carry_over_used: 5,
  excess: 3,
  cause: { type: "leave_approve", leave_id: "L-1" },
  opened_at: "2027-01-05T03:00:00Z",
  resolved_at: null,
  resolved_by_name: null,
  resolution: null,
  ...over,
});

const balance = { employee_id: "e1", full_name: "Ana Lestari", year: 2027 } as LeaveBalanceView;

beforeEach(() => {
  mutate.mockReset();
  toast.success.mockReset();
  toast.error.mockReset();
  q.useCarryReviews.mockReset();
  q.useLeaveBalance.mockReset();
  q.useCarryReviews.mockReturnValue({
    data: [review(), review({ id: "r2", employee_id: "e2", nip: null, full_name: "Budi", branch_name: null, carry_over_days: 0, carry_over_used: 1.5, excess: 1.5, cause: { type: "settings" } })],
    isLoading: false,
    isError: false,
  });
  q.useLeaveBalance.mockReturnValue({ data: balance, isLoading: false, isError: false });
});
afterEach(cleanup);

describe("LeaveCarryReviewsCard", () => {
  it("memuat tinjauan tahun yang dipilih (hanya dengan hak baca) dan tidak tampil bila kosong, memuat, atau galat", () => {
    render(<LeaveCarryReviewsCard year={2027} canUpdate canCorrect />);
    expect(q.useCarryReviews).toHaveBeenLastCalledWith(2027);
    expect(screen.getByText("Perlu ditinjau HR (2)")).toBeTruthy();
    cleanup();
    for (const state of [
      { data: [], isLoading: false, isError: false },
      { data: undefined, isLoading: true, isError: false },
      { data: undefined, isLoading: false, isError: true },
    ]) {
      q.useCarryReviews.mockReturnValue(state);
      const r = render(<LeaveCarryReviewsCard year={2027} canUpdate canCorrect />);
      expect(r.container.textContent).toBe("");
      cleanup();
    }
  });

  it("menampilkan NIP, nama, cabang, bawaan, terpakai, kelebihan, penyebab terbaca, dan sejak kapan", () => {
    render(<LeaveCarryReviewsCard year={2027} canUpdate canCorrect />);
    const rows = screen.getAllByRole("row");
    const ana = within(rows[1]);
    expect(ana.getAllByRole("cell").map((c) => c.textContent)).toEqual([
      "2025001",
      "Ana Lestari",
      "PPJA",
      "2",
      "5",
      "3",
      "Cuti disetujui",
      "5 Jan 2027",
      "Tandai selesaiKoreksi",
    ]);
    const budi = within(rows[2]);
    expect(budi.getByText("Batas sisa bawaan diubah")).toBeTruthy();
    expect(budi.getAllByRole("cell").map((c) => c.textContent)).toEqual([
      "-", // NIP kosong
      "Budi",
      "-", // cabang kosong
      "0",
      "1,5",
      "1,5",
      "Batas sisa bawaan diubah",
      "5 Jan 2027",
      "Tandai selesaiKoreksi",
    ]);
  });

  it("tanpa hak update tidak ada aksi", () => {
    render(<LeaveCarryReviewsCard year={2027} canUpdate={false} canCorrect={false} />);
    expect(screen.queryByRole("button", { name: "Tandai selesai" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Koreksi" })).toBeNull();
  });

  it("Tandai selesai: catatan wajib, dipangkas, dikirim bersama id, lalu toast dan dialog tertutup", () => {
    render(<LeaveCarryReviewsCard year={2027} canUpdate canCorrect />);
    fireEvent.click(screen.getAllByRole("button", { name: "Tandai selesai" })[0]);
    const submit = () => screen.getByRole("button", { name: "Simpan" }) as HTMLButtonElement;
    expect(submit().disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Catatan"), { target: { value: "   " } });
    expect(submit().disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Catatan"), { target: { value: "  Dibahas dengan atasan  " } });
    expect(submit().disabled).toBe(false);
    fireEvent.click(submit());
    expect(mutate.mock.calls[0][0]).toEqual({ id: "r1", note: "Dibahas dengan atasan" });
    act(() => mutate.mock.calls[0][1].onSuccess({ message: "Tinjauan ditandai selesai" }));
    expect(toast.success).toHaveBeenCalledWith("Tinjauan ditandai selesai");
    expect(screen.queryByLabelText("Catatan")).toBeNull();
  });

  it("catatan lebih dari 500 karakter ditolak; galat server ditampilkan dan dialog tetap terbuka", () => {
    render(<LeaveCarryReviewsCard year={2027} canUpdate canCorrect />);
    fireEvent.click(screen.getAllByRole("button", { name: "Tandai selesai" })[1]);
    fireEvent.change(screen.getByLabelText("Catatan"), { target: { value: "x".repeat(501) } });
    expect((screen.getByRole("button", { name: "Simpan" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("Catatan maksimal 500 karakter")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Catatan"), { target: { value: "Selesai" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    expect(mutate.mock.calls[0][0]).toEqual({ id: "r2", note: "Selesai" });
    act(() => mutate.mock.calls[0][1].onError(new Error("Tinjauan sudah diselesaikan")));
    expect(screen.getByRole("alert").textContent).toBe("Tinjauan sudah diselesaikan");
    expect(screen.getByLabelText("Catatan")).toBeTruthy();
  });

  it("Koreksi membuka dialog ubah saldo karyawan dan tahun itu; hanya dengan hak koreksi", () => {
    render(<LeaveCarryReviewsCard year={2027} canUpdate canCorrect />);
    expect(screen.queryByText(/dialog-ubah/)).toBeNull();
    fireEvent.click(screen.getAllByRole("button", { name: "Koreksi" })[0]);
    expect(q.useLeaveBalance).toHaveBeenLastCalledWith("e1", 2027);
    expect(screen.getByText(/dialog-ubah-Ana Lestari-2027/)).toBeTruthy();
    fireEvent.click(screen.getByText("tutup-ubah"));
    expect(screen.queryByText(/dialog-ubah/)).toBeNull();
    cleanup();
    render(<LeaveCarryReviewsCard year={2027} canUpdate canCorrect={false} />);
    expect(screen.queryByRole("button", { name: "Koreksi" })).toBeNull();
    expect(screen.getAllByRole("button", { name: "Tandai selesai" }).length).toBe(2);
  });

  it("saldo gagal dimuat saat Koreksi: toast galat dan tidak ada dialog", () => {
    q.useLeaveBalance.mockReturnValue({ data: undefined, isLoading: false, isError: true, error: new Error("Gagal memuat saldo") });
    render(<LeaveCarryReviewsCard year={2027} canUpdate canCorrect />);
    fireEvent.click(screen.getAllByRole("button", { name: "Koreksi" })[0]);
    expect(toast.error).toHaveBeenCalledWith("Gagal memuat saldo");
    expect(screen.queryByText(/dialog-ubah/)).toBeNull();
  });
});
