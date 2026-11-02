import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { PayrollSettingsPage } from "./payroll-settings-page";

const save = vi.fn();
let settingsRow: Record<string, unknown> = {};

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/features/hris/attendance", () => ({ AttendancePeriodSettingsCard: () => null }));
vi.mock("../queries", () => ({
  usePayrollSettings: () => ({ data: { settings: settingsRow, tax_config: {} }, isLoading: false }),
}));
vi.mock("../mutations", () => ({
  useSavePayrollSettings: () => ({ mutateAsync: save, isPending: false }),
}));

beforeEach(() => {
  save.mockReset();
  save.mockResolvedValue({});
  settingsRow = { pph21_enabled: true, bpjs_tk_enabled: true, bpjs_kes_enabled: true };
});
afterEach(cleanup);

const LABELS = ["PPh 21 dihitung", "BPJS Ketenagakerjaan dihitung", "BPJS Kesehatan dihitung"];

describe("PayrollSettingsPage saklar PPh 21 / BPJS (EPIC-082)", () => {
  it("menampilkan 3 saklar dengan label dan catatan", () => {
    render(<PayrollSettingsPage />);
    for (const label of LABELS) expect(screen.getByLabelText(label)).toBeTruthy();
    expect(
      screen.getByText(
        "Nonaktif: tidak dipotong dari gaji dan tidak ada iuran perusahaan. Berlaku untuk payroll yang dihitung setelah disimpan."
      )
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Bila PPh 21 dinyalakan kembali di tengah tahun, perhitungan Desember (Pasal 17, metode TER) menyesuaikan pajak setahun penuh."
      )
    ).toBeTruthy();
  });

  it("mematikan PPh 21 lalu simpan mengirim pph21_enabled: false", async () => {
    render(<PayrollSettingsPage />);
    fireEvent.click(screen.getByLabelText("PPh 21 dihitung"));
    fireEvent.click(screen.getByRole("button", { name: "Simpan Pengaturan" }));
    await vi.waitFor(() => expect(save).toHaveBeenCalledTimes(1));
    expect(save.mock.calls[0][0]).toEqual({ settings: { pph21_enabled: false } });
  });

  it("memuat nilai false dari server dan tidak mengirimnya bila tidak diubah", async () => {
    settingsRow = { pph21_enabled: false, bpjs_tk_enabled: false, bpjs_kes_enabled: true };
    render(<PayrollSettingsPage />);
    expect((screen.getByLabelText("PPh 21 dihitung") as HTMLInputElement).checked).toBe(false);
    expect((screen.getByLabelText("BPJS Ketenagakerjaan dihitung") as HTMLInputElement).checked).toBe(false);
    expect((screen.getByLabelText("BPJS Kesehatan dihitung") as HTMLInputElement).checked).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Simpan Pengaturan" }));
    await vi.waitFor(() => expect(toast.error).toHaveBeenCalledWith("Tidak ada perubahan untuk disimpan"));
    expect(save).not.toHaveBeenCalled();
  });

  const field = (id: string) => document.getElementById(id) as HTMLInputElement;

  it("input tarif BPJS TK nonaktif bila saklarnya mati, nilainya tetap, BPJS Kes tetap aktif", () => {
    settingsRow = { bpjs_tk_enabled: false, bpjs_kes_enabled: true, bpjs_tk_jht_employee: 2, bpjs_kes_employee: 1 };
    render(<PayrollSettingsPage />);
    expect(field("bpjs_tk_jht_employee").disabled).toBe(true);
    expect(field("bpjs_tk_jkm").disabled).toBe(true);
    expect(field("bpjs_tk_jht_employee").value).toBe("2");
    expect(field("bpjs_kes_employee").disabled).toBe(false);
  });

  it("input tarif BPJS Kes nonaktif bila saklarnya mati, subsidi tetap aktif", () => {
    settingsRow = { bpjs_tk_enabled: true, bpjs_kes_enabled: false };
    render(<PayrollSettingsPage />);
    expect(field("bpjs_kes_employee").disabled).toBe(true);
    expect(field("bpjs_kes_employer").disabled).toBe(true);
    expect(field("bpjs_kes_max_upah").disabled).toBe(true);
    expect(field("bpjs_kes_subsidy_amount").disabled).toBe(false);
    expect(field("bpjs_tk_jht_employee").disabled).toBe(false);
  });

  it("PPh 21 mati menonaktifkan metode, dasar TER, PTKP, dan bracket", () => {
    settingsRow = { pph21_enabled: false };
    render(<PayrollSettingsPage />);
    expect((document.getElementById("pph21_method") as HTMLSelectElement).disabled).toBe(true);
    expect((document.getElementById("pph21_bpjs_basis") as HTMLSelectElement).disabled).toBe(true);
    expect(field("ptkp_tk_0").disabled).toBe(true);
    expect(field("bracket_1_rate").disabled).toBe(true);
  });

  it("semua input aktif bila saklar menyala", () => {
    render(<PayrollSettingsPage />);
    expect(field("bpjs_tk_jht_employee").disabled).toBe(false);
    expect(field("ptkp_tk_0").disabled).toBe(false);
    expect((document.getElementById("pph21_method") as HTMLSelectElement).disabled).toBe(false);
  });

  it("header menumpuk di HP dan tombol simpan tidak terpotong", () => {
    render(<PayrollSettingsPage />);
    const header = screen.getByTestId("settings-header");
    expect(header.className).toContain("flex-col");
    expect(header.className).toContain("sm:flex-row");
    const saveBtn = screen.getByRole("button", { name: "Simpan Pengaturan" });
    expect(saveBtn.className).toContain("w-full");
    expect(saveBtn.className).toContain("sm:w-auto");
  });
});
