import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { LeaveSettings } from "../types";
import { LeaveSettingsCard } from "./leave-settings-card";

const mutate = vi.hoisted(() => vi.fn());
const previewMutate = vi.hoisted(() => vi.fn());
vi.mock("../mutations", () => ({
  useSaveLeaveSettings: () => ({ mutate, isPending: false }),
  usePreviewLeaveSettings: () => ({ mutate: previewMutate, isPending: false }),
}));
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
  previewMutate.mockReset();
  toast.success.mockReset();
});
afterEach(cleanup);

const saveBtn = () => screen.getByRole("button", { name: "Simpan pengaturan" }) as HTMLButtonElement;
const previewBtn = () => screen.getByRole("button", { name: "Pratinjau dampak" }) as HTMLButtonElement;
const impact = { employees_changed: 12, days_increase: 30.5, days_decrease: 4, needs_review: 2 };
/** Jalankan pratinjau dan jawab berhasil, seperti server. */
const runPreview = (result = impact) => {
  fireEvent.click(previewBtn());
  act(() => previewMutate.mock.calls[previewMutate.mock.calls.length - 1][1].onSuccess(result));
};
const typeReason = (value = "Kebijakan baru 2026") => fireEvent.change(screen.getByLabelText("Alasan perubahan"), { target: { value } });

describe("LeaveSettingsCard", () => {
  it("menjelaskan jalannya otomatis tiap 1 Januari (WIB), hitung ulang tahun berjalan, dan bawaan manual tidak ditimpa", () => {
    render(<LeaveSettingsCard canUpdate />);
    expect(screen.getByText(/otomatis setiap 1 Januari \(WIB\)/)).toBeTruthy();
    expect(screen.getByText(/dihitung ulang otomatis.*Pratinjau dampak/i)).toBeTruthy();
    expect(screen.getByText(/diisi manual.*impor Excel.*tidak ditimpa/i)).toBeTruthy();
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

  it("Simpan mati sampai ada perubahan; patch lengkap dengan batas kosong = null dan alasan dipangkas dikirim setelah pratinjau", () => {
    render(<LeaveSettingsCard canUpdate />);
    expect(saveBtn().disabled).toBe(true);
    expect(previewBtn().disabled).toBe(true);
    fireEvent.click(screen.getByRole("switch"));
    expect(saveBtn().disabled).toBe(true); // belum dipratinjau, belum ada alasan
    runPreview();
    typeReason("  Kebijakan baru 2026  ");
    expect(saveBtn().disabled).toBe(false);
    fireEvent.click(saveBtn());
    expect(mutate.mock.calls[0][0]).toEqual({
      carry_over_enabled: true,
      carry_over_max_days: null,
      carry_over_expiry_month: 3,
      carry_over_expiry_day: 31,
      reason: "Kebijakan baru 2026",
    });
    mutate.mock.calls[0][1].onSuccess({ message: "Pengaturan sisa cuti tersimpan" });
    expect(toast.success).toHaveBeenCalledWith("Pengaturan sisa cuti tersimpan");
  });

  it("alasan wajib: Simpan mati walau sudah dipratinjau sampai alasan terisi (spasi saja tidak cukup)", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.click(screen.getByRole("switch"));
    runPreview();
    expect(saveBtn().disabled).toBe(true);
    typeReason("   ");
    expect(saveBtn().disabled).toBe(true);
    typeReason("Alasan");
    expect(saveBtn().disabled).toBe(false);
    typeReason("x".repeat(501));
    expect(saveBtn().disabled).toBe(true);
    expect(screen.getByText("Alasan maksimal 500 karakter")).toBeTruthy();
  });

  it("Pratinjau dampak mengirim patch tanpa alasan dan menampilkan angka dampak", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.change(screen.getByLabelText("Maksimal hari yang dibawa"), { target: { value: "5,5" } });
    typeReason();
    fireEvent.click(previewBtn());
    expect(previewMutate.mock.calls[0][0]).toEqual({
      carry_over_enabled: false,
      carry_over_max_days: 5.5,
      carry_over_expiry_month: 3,
      carry_over_expiry_day: 31,
    });
    expect(screen.queryByTestId("impact")).toBeNull();
    act(() => previewMutate.mock.calls[0][1].onSuccess(impact));
    const box = screen.getByTestId("impact").textContent ?? "";
    expect(box).toContain("12 karyawan berubah");
    expect(box).toContain("bertambah 30,5 hari");
    expect(box).toContain("berkurang 4 hari");
    expect(box).toContain("2 perlu ditinjau HR");
  });

  it("setiap perubahan isian setelah pratinjau membatalkannya: hasil hilang dan Simpan mati lagi", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.click(screen.getByRole("switch"));
    runPreview();
    typeReason();
    expect(saveBtn().disabled).toBe(false);
    expect(screen.getByTestId("impact")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Maksimal hari yang dibawa"), { target: { value: "3" } });
    expect(screen.queryByTestId("impact")).toBeNull();
    expect(saveBtn().disabled).toBe(true);
    runPreview();
    expect(saveBtn().disabled).toBe(false);
    fireEvent.change(screen.getByLabelText("Bulan kedaluwarsa"), { target: { value: "6" } });
    expect(saveBtn().disabled).toBe(true);
    runPreview();
    fireEvent.change(screen.getByLabelText("Tanggal kedaluwarsa"), { target: { value: "30" } });
    expect(saveBtn().disabled).toBe(true);
    runPreview();
    fireEvent.click(screen.getByRole("switch"));
    expect(saveBtn().disabled).toBe(true);
  });

  it("jawaban pratinjau yang datang setelah isian berubah dibuang", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.click(screen.getByRole("switch"));
    fireEvent.click(previewBtn());
    fireEvent.change(screen.getByLabelText("Maksimal hari yang dibawa"), { target: { value: "3" } });
    act(() => previewMutate.mock.calls[0][1].onSuccess(impact));
    expect(screen.queryByTestId("impact")).toBeNull();
    typeReason();
    expect(saveBtn().disabled).toBe(true);
  });

  it("pratinjau gagal menampilkan galat server dan Simpan tetap mati", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.click(screen.getByRole("switch"));
    fireEvent.click(previewBtn());
    act(() => previewMutate.mock.calls[0][1].onError(new Error("Maksimal sisa bawaan harus kelipatan 0,5")));
    expect(screen.getByRole("alert").textContent).toBe("Maksimal sisa bawaan harus kelipatan 0,5");
    typeReason();
    expect(saveBtn().disabled).toBe(true);
  });

  it("mengirim batas maksimal dan tanggal kedaluwarsa baru", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.change(screen.getByLabelText("Maksimal hari yang dibawa"), { target: { value: "5,5" } });
    fireEvent.change(screen.getByLabelText("Bulan kedaluwarsa"), { target: { value: "6" } });
    fireEvent.change(screen.getByLabelText("Tanggal kedaluwarsa"), { target: { value: "30" } });
    runPreview();
    typeReason();
    fireEvent.click(saveBtn());
    expect(mutate.mock.calls[0][0]).toMatchObject({
      carry_over_max_days: 5.5,
      carry_over_expiry_month: 6,
      carry_over_expiry_day: 30,
    });
  });

  it("menolak batas bukan kelipatan 0,5 dan tanggal yang tidak ada (29 Feb, 31 Apr): pratinjau dan simpan mati", () => {
    render(<LeaveSettingsCard canUpdate />);
    typeReason();
    fireEvent.change(screen.getByLabelText("Maksimal hari yang dibawa"), { target: { value: "5,3" } });
    expect(screen.getByText("Batas harus kelipatan 0,5")).toBeTruthy();
    expect(saveBtn().disabled).toBe(true);
    expect(previewBtn().disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Maksimal hari yang dibawa"), { target: { value: "" } });
    fireEvent.change(screen.getByLabelText("Bulan kedaluwarsa"), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Tanggal kedaluwarsa"), { target: { value: "29" } });
    expect(screen.getByText("Tanggal 29 Februari tidak dipakai")).toBeTruthy();
    expect(saveBtn().disabled).toBe(true);
    expect(previewBtn().disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Bulan kedaluwarsa"), { target: { value: "4" } });
    fireEvent.change(screen.getByLabelText("Tanggal kedaluwarsa"), { target: { value: "31" } });
    expect(screen.getByText("Tanggal kedaluwarsa tidak valid")).toBeTruthy();
  });

  it("menampilkan galat server apa adanya", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.click(screen.getByRole("switch"));
    runPreview();
    typeReason();
    fireEvent.click(saveBtn());
    act(() => mutate.mock.calls[0][1].onError(new Error("Batas sisa bawaan harus kelipatan 0,5")));
    expect(screen.getByRole("alert").textContent).toBe("Batas sisa bawaan harus kelipatan 0,5");
  });

  it("setelah tersimpan isian, alasan, dan hasil pratinjau dibersihkan", () => {
    render(<LeaveSettingsCard canUpdate />);
    fireEvent.click(screen.getByRole("switch"));
    runPreview();
    typeReason();
    fireEvent.click(saveBtn());
    act(() => mutate.mock.calls[0][1].onSuccess({ message: "ok" }));
    expect(screen.queryByTestId("impact")).toBeNull();
    expect((screen.getByLabelText("Alasan perubahan") as HTMLTextAreaElement).value).toBe("");
    expect(saveBtn().disabled).toBe(true);
  });

  it("tanpa hak update tidak ada tombol pratinjau dan alasan terkunci", () => {
    render(<LeaveSettingsCard canUpdate={false} />);
    expect(screen.queryByRole("button", { name: "Pratinjau dampak" })).toBeNull();
  });
});
