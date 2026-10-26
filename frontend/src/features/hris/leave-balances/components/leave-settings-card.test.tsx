import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LeaveSettings } from "../types";
import { LeaveSettingsCard } from "./leave-settings-card";

const mutate = vi.hoisted(() => vi.fn());
vi.mock("../mutations", () => ({ useSaveLeaveSettings: () => ({ mutate, isPending: false }) }));
let settings: LeaveSettings | undefined;
vi.mock("../queries", () => ({
  useLeaveSettings: () => ({ data: settings, isLoading: !settings }),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({ toast }));

const stored: LeaveSettings = {
  carry_over_enabled: false,
  carry_over_max_days: null,
  carry_over_expiry_month: 3,
  carry_over_expiry_day: 31,
  updated_at: null,
  updated_by_name: null,
  carry_over_ready: true,
};

beforeEach(() => {
  settings = stored;
  mutate.mockReset();
  toast.success.mockReset();
});
afterEach(cleanup);

const saveBtn = () => screen.getByRole("button", { name: "Simpan pengaturan" }) as HTMLButtonElement;

describe("LeaveSettingsCard", () => {
  it("menjelaskan jalannya otomatis tiap 1 Januari (WIB) dan berlaku mulai 1 Januari berikutnya", () => {
    render(<LeaveSettingsCard canUpdate />);
    expect(screen.getByText(/otomatis setiap 1 Januari \(WIB\)/)).toBeTruthy();
    expect(screen.getByText(/berlaku mulai 1 Januari berikutnya/)).toBeTruthy();
    expect(screen.getByText(/tahun yang sudah berjalan.*manual.*impor Excel/i)).toBeTruthy();
  });

  it("sebelum perbaikan hitung ulang terpasang, sakelar terkunci dan alasannya ditampilkan", () => {
    settings = { ...stored, carry_over_ready: false };
    render(<LeaveSettingsCard canUpdate />);
    expect((screen.getByRole("switch") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(/belum bisa diaktifkan.*perhitungan ulang/i)).toBeTruthy();
  });

  it("tanpa hak update semua isian terkunci dan tombol simpan tidak ada", () => {
    render(<LeaveSettingsCard canUpdate={false} />);
    expect((screen.getByRole("switch") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByRole("button", { name: "Simpan pengaturan" })).toBeNull();
  });

  it("Simpan mati sampai ada perubahan; mengirim patch lengkap dengan batas kosong = null", () => {
    render(<LeaveSettingsCard canUpdate />);
    expect(saveBtn().disabled).toBe(true);
    fireEvent.click(screen.getByRole("switch"));
    expect(saveBtn().disabled).toBe(false);
    fireEvent.click(saveBtn());
    expect(mutate.mock.calls[0][0]).toEqual({
      carry_over_enabled: true,
      carry_over_max_days: null,
      carry_over_expiry_month: 3,
      carry_over_expiry_day: 31,
    });
    mutate.mock.calls[0][1].onSuccess({ message: "Pengaturan sisa cuti tersimpan" });
    expect(toast.success).toHaveBeenCalledWith("Pengaturan sisa cuti tersimpan");
  });

  it("mengirim batas maksimal dan tanggal kedaluwarsa baru", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.change(screen.getByLabelText("Maksimal hari yang dibawa"), { target: { value: "5,5" } });
    fireEvent.change(screen.getByLabelText("Bulan kedaluwarsa"), { target: { value: "6" } });
    fireEvent.change(screen.getByLabelText("Tanggal kedaluwarsa"), { target: { value: "30" } });
    fireEvent.click(saveBtn());
    expect(mutate.mock.calls[0][0]).toMatchObject({
      carry_over_max_days: 5.5,
      carry_over_expiry_month: 6,
      carry_over_expiry_day: 30,
    });
  });

  it("menolak batas bukan kelipatan 0,5 dan tanggal yang tidak ada (29 Feb, 31 Apr)", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.change(screen.getByLabelText("Maksimal hari yang dibawa"), { target: { value: "5,3" } });
    expect(screen.getByText("Batas harus kelipatan 0,5")).toBeTruthy();
    expect(saveBtn().disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Maksimal hari yang dibawa"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Bulan kedaluwarsa"), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Tanggal kedaluwarsa"), { target: { value: "29" } });
    expect(screen.getByText("Tanggal 29 Februari tidak dipakai")).toBeTruthy();
    expect(saveBtn().disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Bulan kedaluwarsa"), { target: { value: "4" } });
    fireEvent.change(screen.getByLabelText("Tanggal kedaluwarsa"), { target: { value: "31" } });
    expect(screen.getByText("Tanggal kedaluwarsa tidak valid")).toBeTruthy();
  });

  it("menampilkan galat server apa adanya", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.click(screen.getByRole("switch"));
    fireEvent.click(saveBtn());
    act(() => mutate.mock.calls[0][1].onError(new Error("Batas sisa bawaan harus kelipatan 0,5")));
    expect(screen.getByRole("alert").textContent).toBe("Batas sisa bawaan harus kelipatan 0,5");
  });
});
