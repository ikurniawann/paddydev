import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LeaveBalanceView } from "../types";
import { LeaveBalanceEditDialog } from "./leave-balance-edit-dialog";

const mutate = vi.hoisted(() => vi.fn());
vi.mock("../mutations", () => ({ useUpdateLeaveBalance: () => ({ mutate, isPending: false }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({ toast }));

const balance: LeaveBalanceView = {
  employee_id: "e1",
  nip: "2025001",
  full_name: "Ana Lestari",
  branch_name: "PPJA",
  year: 2026,
  annual_leave_total: 12,
  annual_leave_adjustment: 0,
  annual_leave_used: 3,
  annual_leave_remaining: 9,
  carry_over_days: 2,
  carry_over_used: 0.5,
  carry_over_remaining: 1.5,
  carry_over_expires_on: "2027-03-31",
  carry_over_active: true,
  available: 10.5,
  updated_at: null,
  exists: true,
};

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-26T03:00:00Z"));
  mutate.mockReset();
  toast.success.mockReset();
  toast.error.mockReset();
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const open = (b = balance, onOpenChange = vi.fn()) => {
  render(<LeaveBalanceEditDialog open onOpenChange={onOpenChange} balance={b} />);
  return onOpenChange;
};
const field = (label: string) => screen.getByLabelText(label) as HTMLInputElement;
const type = (label: string, value: string) => fireEvent.change(field(label), { target: { value } });
const save = () => screen.getByRole("button", { name: "Simpan" }) as HTMLButtonElement;
const preview = (id: "remaining" | "available") => screen.getByTestId(`preview-${id}`).textContent;

describe("LeaveBalanceEditDialog", () => {
  it("menampilkan isian awal, terpakai (baca saja), dan pratinjau sisa", () => {
    open();
    expect(field("Jatah").value).toBe("12");
    expect(field("Penyesuaian").value).toBe("0");
    expect(field("Sisa bawaan").value).toBe("2");
    expect(field("Kedaluwarsa").value).toBe("2027-03-31");
    expect(screen.getByTestId("used").textContent).toBe("3");
    expect(preview("remaining")).toBe("9");
    expect(preview("available")).toBe("10,5");
  });

  it("pratinjau ikut berubah saat jatah dan penyesuaian (boleh negatif) diubah", () => {
    open();
    type("Penyesuaian", "-2");
    expect(preview("remaining")).toBe("7");
    expect(preview("available")).toBe("8,5");
    type("Jatah", "14,5");
    expect(preview("remaining")).toBe("9,5");
    expect(preview("available")).toBe("11");
  });

  it("sisa bawaan yang sudah kedaluwarsa tidak masuk sisa tersedia", () => {
    open();
    type("Kedaluwarsa", "2026-03-31");
    expect(preview("available")).toBe("9");
    type("Kedaluwarsa", "");
    expect(preview("available")).toBe("9");
    expect(screen.getByText(/tanpa tanggal kedaluwarsa, sisa bawaan tidak berlaku/i)).toBeTruthy();
  });

  it("alasan wajib: Simpan mati sampai alasan diisi", () => {
    open();
    expect(save().disabled).toBe(true);
    type("Alasan", "   ");
    expect(save().disabled).toBe(true);
    type("Alasan", "Koreksi jatah sesuai kontrak");
    expect(save().disabled).toBe(false);
  });

  it("menolak angka bukan kelipatan 0,5", () => {
    open();
    type("Alasan", "Koreksi");
    type("Jatah", "9,3");
    expect(screen.getByText("Jatah harus kelipatan 0,5")).toBeTruthy();
    expect(save().disabled).toBe(true);
    type("Jatah", "9,5");
    expect(screen.queryByText("Jatah harus kelipatan 0,5")).toBeNull();
    type("Penyesuaian", "0,25");
    expect(screen.getByText("Penyesuaian harus kelipatan 0,5")).toBeTruthy();
    expect(save().disabled).toBe(true);
  });

  it("menolak sisa jatah di bawah 0 dan sisa bawaan di bawah yang terpakai", () => {
    open();
    type("Alasan", "Koreksi");
    type("Jatah", "2");
    expect(screen.getByText(/Sisa jatah tidak boleh kurang dari 0/)).toBeTruthy();
    expect(save().disabled).toBe(true);
    type("Jatah", "12");
    type("Sisa bawaan", "0");
    expect(screen.getByText(/Sisa bawaan tidak boleh kurang dari yang sudah terpakai \(0,5\)/)).toBeTruthy();
    expect(save().disabled).toBe(true);
  });

  it("mengirim PUT dengan semua angka, tanggal, dan alasan yang dipangkas", () => {
    const onOpenChange = open();
    type("Jatah", "13");
    type("Penyesuaian", "-1");
    type("Alasan", "  Potongan kebijakan  ");
    fireEvent.click(save());
    expect(mutate).toHaveBeenCalledTimes(1);
    expect(mutate.mock.calls[0][0]).toEqual({
      employeeId: "e1",
      payload: {
        year: 2026,
        annual_leave_total: 13,
        annual_leave_adjustment: -1,
        carry_over_days: 2,
        carry_over_expires_on: "2027-03-31",
        reason: "Potongan kebijakan",
      },
    });
    mutate.mock.calls[0][1].onSuccess({ message: "Saldo cuti diperbarui" });
    expect(toast.success).toHaveBeenCalledWith("Saldo cuti diperbarui");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("mengosongkan kedaluwarsa mengirim null", () => {
    open();
    type("Kedaluwarsa", "");
    type("Alasan", "Bawaan hangus");
    fireEvent.click(save());
    expect(mutate.mock.calls[0][0].payload.carry_over_expires_on).toBeNull();
  });

  it("menampilkan pesan galat server apa adanya dan dialog tetap terbuka", () => {
    const onOpenChange = open();
    type("Alasan", "Koreksi");
    fireEvent.click(save());
    act(() => mutate.mock.calls[0][1].onError(new Error("Sisa jatah tidak boleh kurang dari 0 (jatah 4 + penyesuaian 0 − terpakai 5 = -1)")));
    expect(screen.getByRole("alert").textContent).toBe(
      "Sisa jatah tidak boleh kurang dari 0 (jatah 4 + penyesuaian 0 − terpakai 5 = -1)"
    );
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});
