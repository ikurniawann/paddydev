import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PlacementSearchRow, StoreBranch } from "@/features/hris/placements/types";
import type { ScheduleDelegate } from "../types";
import { DelegateFormDialog } from "./delegate-form-dialog";

const api = vi.hoisted(() => ({
  fetchScheduleDelegates: vi.fn(),
  fetchDelegateSuggest: vi.fn(),
  saveScheduleDelegate: vi.fn(),
  revokeScheduleDelegate: vi.fn(),
}));
const placementsApi = vi.hoisted(() => ({ fetchStoreBranches: vi.fn(), searchPlacements: vi.fn() }));
vi.mock("../api", () => api);
vi.mock("@/features/hris/placements/api", () => placementsApi);
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
// Combobox asli (popover + cmdk) diganti <select> supaya pilihan bisa dipakai di jsdom.
vi.mock("@/components/ui/combobox", () => ({
  Combobox: ({
    options,
    value,
    onChange,
    placeholder,
    disabled,
  }: {
    options: { value: string; label: string }[];
    value?: string;
    onChange: (v: string) => void;
    placeholder?: string;
    disabled?: boolean;
  }) => (
    <select aria-label={placeholder} value={value} disabled={disabled} onChange={(e) => onChange(e.target.value)}>
      <option value="">{placeholder}</option>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  ),
}));

const branch = (id: string, code: string, name: string): StoreBranch => ({
  id,
  code,
  name,
  company_id: "c1",
  company_name: "Paddy",
  lat: null,
  lng: null,
  geofence_radius_m: null,
});
const branches = [branch("b1", "PVJ", "Paddy PVJ"), branch("b2", "KPO", "Paddy Kopo"), branch("b3", "PIK", "Paddy PIK")];

const employee = (over: Partial<PlacementSearchRow> = {}): PlacementSearchRow => ({
  employee_id: "e1",
  full_name: "Ani Wijaya",
  nip: "1001",
  photo_url: null,
  department_name: "Operations",
  position_title: "Shopkeeper",
  placements: [],
  department_id: "d1",
  position_id: "p1",
  schedule_type: "shift",
  has_schedule: false,
  schedule_until: null,
  ...over,
});

const delegate: ScheduleDelegate = {
  employee_id: "e9",
  nip: "9009",
  full_name: "Budi Santoso",
  position_title: "Area Manager",
  has_login: true,
  can_place: true,
  direct_reports: 3,
  is_active: true,
  branches: [{ id: "b3", code: "PIK", name: "Paddy PIK" }],
  updated_at: "2026-10-29T03:00:00Z",
  updated_by_name: "HR Admin",
};

function renderDialog(props: Partial<React.ComponentProps<typeof DelegateFormDialog>> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <DelegateFormDialog open delegate={null} onOpenChange={onOpenChange} {...props} />
    </QueryClientProvider>
  );
  return { client, onOpenChange };
}

beforeEach(() => {
  Object.values(api).forEach((fn) => fn.mockReset());
  Object.values(placementsApi).forEach((fn) => fn.mockReset());
  placementsApi.fetchStoreBranches.mockResolvedValue(branches);
  placementsApi.searchPlacements.mockResolvedValue({
    rows: [employee()],
    total: 1,
    page: 1,
    page_size: 25,
    counts: { all: 1, unplaced: 0, no_schedule: 0, multi: 0 },
  });
  api.fetchDelegateSuggest.mockResolvedValue({ branch_ids: ["b1", "b2"] });
  api.fetchScheduleDelegates.mockResolvedValue([]);
});
afterEach(cleanup);

describe("DelegateFormDialog (EPIC-071)", () => {
  it("pilih karyawan -> toko terisi dari saran -> simpan mengirim PUT dengan can_place false", async () => {
    api.saveScheduleDelegate.mockResolvedValue({ ...delegate, employee_id: "e1" });
    const { onOpenChange } = renderDialog();
    expect(screen.getByText("Pengatur hanya bisa mengubah jadwal dan penempatan di toko yang dipilih")).toBeTruthy();

    await waitFor(() => expect(screen.getByRole("option", { name: /Ani Wijaya/ })).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Pilih karyawan"), { target: { value: "e1" } });

    await waitFor(() => expect((screen.getByLabelText("PVJ - Paddy PVJ") as HTMLInputElement).checked).toBe(true));
    expect((screen.getByLabelText("KPO - Paddy Kopo") as HTMLInputElement).checked).toBe(true);
    expect((screen.getByLabelText("PIK - Paddy PIK") as HTMLInputElement).checked).toBe(false);
    expect(api.fetchDelegateSuggest).toHaveBeenCalledWith("e1");

    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(api.saveScheduleDelegate).toHaveBeenCalled());
    expect(api.saveScheduleDelegate).toHaveBeenCalledWith("e1", { branch_ids: ["b1", "b2"], can_place: false });
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("toko bisa diubah dan hak penempatan dicentang sebelum simpan", async () => {
    api.saveScheduleDelegate.mockResolvedValue(delegate);
    renderDialog();
    await waitFor(() => expect(screen.getByRole("option", { name: /Ani Wijaya/ })).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Pilih karyawan"), { target: { value: "e1" } });
    await waitFor(() => expect((screen.getByLabelText("PVJ - Paddy PVJ") as HTMLInputElement).checked).toBe(true));

    fireEvent.click(screen.getByLabelText("PVJ - Paddy PVJ"));
    fireEvent.click(screen.getByLabelText("PIK - Paddy PIK"));
    fireEvent.click(screen.getByLabelText("Boleh atur penempatan karyawan"));
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(api.saveScheduleDelegate).toHaveBeenCalled());
    expect(api.saveScheduleDelegate).toHaveBeenCalledWith("e1", { branch_ids: ["b2", "b3"], can_place: true });
  });

  it("respons 400 ditampilkan di dalam dialog dan dialog tetap terbuka", async () => {
    api.saveScheduleDelegate.mockRejectedValue(new Error("Karyawan belum punya akun login"));
    const { onOpenChange } = renderDialog();
    await waitFor(() => expect(screen.getByRole("option", { name: /Ani Wijaya/ })).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Pilih karyawan"), { target: { value: "e1" } });
    await waitFor(() => expect((screen.getByLabelText("PVJ - Paddy PVJ") as HTMLInputElement).checked).toBe(true));
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toBe("Karyawan belum punya akun login"));
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
  });

  it("tanpa toko terpilih tidak memanggil server", async () => {
    api.fetchDelegateSuggest.mockResolvedValue({ branch_ids: [] });
    renderDialog();
    await waitFor(() => expect(screen.getByRole("option", { name: /Ani Wijaya/ })).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Pilih karyawan"), { target: { value: "e1" } });
    await waitFor(() => expect(api.fetchDelegateSuggest).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    expect(api.saveScheduleDelegate).not.toHaveBeenCalled();
    expect(screen.getByRole("alert").textContent).toBe("Pilih minimal satu toko");
  });

  it("mode ubah: karyawan terkunci, toko dan hak terisi dari baris, tanpa memanggil saran", async () => {
    api.saveScheduleDelegate.mockResolvedValue(delegate);
    renderDialog({ delegate });
    expect(screen.getByText("Budi Santoso")).toBeTruthy();
    expect(screen.queryByLabelText("Pilih karyawan")).toBeNull();
    await waitFor(() => expect((screen.getByLabelText("PIK - Paddy PIK") as HTMLInputElement).checked).toBe(true));
    expect((screen.getByLabelText("Boleh atur penempatan karyawan") as HTMLInputElement).checked).toBe(true);
    expect(api.fetchDelegateSuggest).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(api.saveScheduleDelegate).toHaveBeenCalled());
    expect(api.saveScheduleDelegate).toHaveBeenCalledWith("e9", { branch_ids: ["b3"], can_place: true });
  });

  it("memilih karyawan yang sudah jadi pengatur memakai toko dan hak lamanya, dengan catatan", async () => {
    api.fetchScheduleDelegates.mockResolvedValue([{ ...delegate, employee_id: "e1", can_place: true }]);
    api.saveScheduleDelegate.mockResolvedValue(delegate);
    renderDialog();
    await waitFor(() => expect(screen.getByRole("option", { name: /Ani Wijaya/ })).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Pilih karyawan"), { target: { value: "e1" } });

    expect(
      await screen.findByText("Karyawan ini sudah menjadi pengatur — perubahan akan menggantikan toko sebelumnya.")
    ).toBeTruthy();
    expect((screen.getByLabelText("PIK - Paddy PIK") as HTMLInputElement).checked).toBe(true);
    expect((screen.getByLabelText("PVJ - Paddy PVJ") as HTMLInputElement).checked).toBe(false);
    expect((screen.getByLabelText("Boleh atur penempatan karyawan") as HTMLInputElement).checked).toBe(true);
    expect(api.fetchDelegateSuggest).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(api.saveScheduleDelegate).toHaveBeenCalled());
    expect(api.saveScheduleDelegate).toHaveBeenCalledWith("e1", { branch_ids: ["b3"], can_place: true });
  });

  it("karyawan baru tidak menampilkan catatan pengatur yang sudah ada", async () => {
    renderDialog();
    await waitFor(() => expect(screen.getByRole("option", { name: /Ani Wijaya/ })).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Pilih karyawan"), { target: { value: "e1" } });
    await waitFor(() => expect((screen.getByLabelText("PVJ - Paddy PVJ") as HTMLInputElement).checked).toBe(true));
    expect(screen.queryByText(/sudah menjadi pengatur/)).toBeNull();
  });

  it("saran toko gagal dimuat menampilkan petunjuk dan toko bisa dipilih manual", async () => {
    api.fetchDelegateSuggest.mockRejectedValue(new Error("boom"));
    api.saveScheduleDelegate.mockResolvedValue(delegate);
    renderDialog();
    await waitFor(() => expect(screen.getByRole("option", { name: /Ani Wijaya/ })).toBeTruthy());
    fireEvent.change(screen.getByLabelText("Pilih karyawan"), { target: { value: "e1" } });
    expect(await screen.findByText("Saran toko gagal dimuat — pilih toko secara manual.")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("KPO - Paddy Kopo"));
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(api.saveScheduleDelegate).toHaveBeenCalled());
    expect(api.saveScheduleDelegate).toHaveBeenCalledWith("e1", { branch_ids: ["b2"], can_place: false });
    expect(api.fetchDelegateSuggest).toHaveBeenCalledTimes(1);
  });

  it("daftar pengatur gagal dimuat: Simpan nonaktif saat menambah (pengatur lama tidak bisa dikenali)", async () => {
    api.fetchScheduleDelegates.mockRejectedValue(new Error("boom"));
    renderDialog();
    expect(await screen.findByText("Daftar pengatur gagal dimuat. Tutup lalu buka lagi dialog ini.")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Simpan" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
