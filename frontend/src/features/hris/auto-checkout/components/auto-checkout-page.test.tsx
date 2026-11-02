import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api-client";
import type { AutoCheckoutRule } from "../types";
import { AutoCheckoutPage } from "./auto-checkout-page";

const api = vi.hoisted(() => ({
  fetchAutoCheckoutRules: vi.fn(),
  fetchAutoCheckoutOptions: vi.fn(),
  createAutoCheckoutRule: vi.fn(),
  setAutoCheckoutRuleActive: vi.fn(),
  deleteAutoCheckoutRule: vi.fn(),
}));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/components/ui/combobox", () => ({
  Combobox: ({ options, value, onChange, placeholder }: {
    options: { value: string; label: string }[];
    value?: string;
    onChange: (v: string) => void;
    placeholder?: string;
  }) => (
    <select aria-label={placeholder} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{placeholder}</option>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  ),
}));

const rule = (over: Partial<AutoCheckoutRule>): AutoCheckoutRule => ({
  id: "r1",
  position_id: "p1",
  position_title: "Shopkeeper",
  branch_id: "b1",
  branch_name: "Paddy Kopo",
  is_active: true,
  editable: true,
  ...over,
});
const rows = [
  rule({}),
  rule({ id: "r2", position_id: "p2", position_title: "Host Live", branch_id: null, branch_name: null, editable: false }),
];

function renderPage(embedded = false) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <AutoCheckoutPage embedded={embedded} />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  api.fetchAutoCheckoutRules.mockResolvedValue(rows);
  api.fetchAutoCheckoutOptions.mockResolvedValue({
    positions: [{ id: "p1", title: "Shopkeeper" }, { id: "p2", title: "Host Live" }],
    branches: [{ id: "b1", code: "KPO", name: "KPO - Paddy Kopo" }],
    all_locations: true,
  });
  api.setAutoCheckoutRuleActive.mockResolvedValue(rows[0]);
  api.createAutoCheckoutRule.mockResolvedValue(rows[0]);
  api.deleteAutoCheckoutRule.mockResolvedValue({});
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AutoCheckoutPage", () => {
  it("menampilkan catatan dan baris Jabatan / Lokasi", async () => {
    renderPage();
    const table = await screen.findByTestId("rules-table");
    expect(screen.getByText("Absen karyawan yang belum clock-out ditutup otomatis di jam selesai shift.")).toBeTruthy();
    expect(within(table).getByText("Shopkeeper")).toBeTruthy();
    expect(within(table).getByText("Paddy Kopo")).toBeTruthy();
    expect(within(table).getByText("Host Live")).toBeTruthy();
    expect(within(table).getByText("Semua lokasi")).toBeTruthy();
  });

  it("di 360 px tabel menjadi kartu bertumpuk (tabel hanya md ke atas, kartu di bawahnya)", async () => {
    renderPage();
    const table = await screen.findByTestId("rules-table");
    const cards = screen.getByTestId("rules-cards");
    expect(table.className).toContain("hidden md:block");
    expect(cards.className).toContain("md:hidden");
    expect(within(cards).getAllByRole("listitem")).toHaveLength(2);
    expect(within(cards).getByText("Semua lokasi")).toBeTruthy();
  });

  it("saklar mengirim PATCH is_active", async () => {
    renderPage();
    const table = await screen.findByTestId("rules-table");
    fireEvent.click(within(table).getByRole("switch", { name: /Matikan checkout otomatis Shopkeeper Paddy Kopo/ }));
    await waitFor(() => expect(api.setAutoCheckoutRuleActive).toHaveBeenCalledWith("r1", false));
  });

  it("aturan semua lokasi hanya baca: saklar nonaktif, tanpa tombol hapus", async () => {
    renderPage();
    const table = await screen.findByTestId("rules-table");
    const allSwitch = within(table).getByRole("switch", { name: /Host Live Semua lokasi/ }) as HTMLButtonElement;
    expect(allSwitch.disabled).toBe(true);
    fireEvent.click(allSwitch);
    expect(api.setAutoCheckoutRuleActive).not.toHaveBeenCalled();
    expect(within(table).queryByRole("button", { name: /Hapus aturan Host Live/ })).toBeNull();
    expect(within(table).getByRole("button", { name: /Hapus aturan Shopkeeper/ })).toBeTruthy();
  });

  it("hapus meminta konfirmasi lalu memanggil DELETE", async () => {
    renderPage();
    const table = await screen.findByTestId("rules-table");
    fireEvent.click(within(table).getByRole("button", { name: /Hapus aturan Shopkeeper/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Ya, hapus" }));
    await waitFor(() => expect(api.deleteAutoCheckoutRule).toHaveBeenCalledWith("r1"));
  });

  it("Tambah aturan: pilih jabatan dan lokasi, simpan mengirim payload", async () => {
    renderPage();
    await screen.findByTestId("rules-table");
    fireEvent.click(screen.getByRole("button", { name: /Tambah aturan/ }));
    fireEvent.change(await screen.findByLabelText("Pilih jabatan"), { target: { value: "p2" } });
    fireEvent.change(screen.getByLabelText("Pilih lokasi"), { target: { value: "all" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(api.createAutoCheckoutRule).toHaveBeenCalledWith({ position_id: "p2", branch_id: null }));
  });

  it("Tambah aturan: pengatur toko tanpa opsi Semua lokasi; satu toko langsung terpilih", async () => {
    api.fetchAutoCheckoutOptions.mockResolvedValue({
      positions: [{ id: "p1", title: "Shopkeeper" }],
      branches: [{ id: "b1", code: "KPO", name: "KPO - Paddy Kopo" }],
      all_locations: false,
    });
    renderPage();
    await screen.findByTestId("rules-table");
    fireEvent.click(screen.getByRole("button", { name: /Tambah aturan/ }));
    const location = (await screen.findByLabelText("Pilih lokasi")) as HTMLSelectElement;
    expect(within(location).queryByText("Semua lokasi")).toBeNull();
    expect(location.value).toBe("b1");
    fireEvent.change(screen.getByLabelText("Pilih jabatan"), { target: { value: "p1" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(api.createAutoCheckoutRule).toHaveBeenCalledWith({ position_id: "p1", branch_id: "b1" }));
  });

  it("tanpa pilihan jabatan: simpan menampilkan galat dan tidak memanggil API", async () => {
    renderPage();
    await screen.findByTestId("rules-table");
    fireEvent.click(screen.getByRole("button", { name: /Tambah aturan/ }));
    await screen.findByLabelText("Pilih jabatan");
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Pilih jabatan");
    expect(api.createAutoCheckoutRule).not.toHaveBeenCalled();
  });

  it("duplikat: pesan server ditampilkan di dialog", async () => {
    api.createAutoCheckoutRule.mockRejectedValue(new Error("Aturan untuk jabatan dan lokasi ini sudah ada"));
    renderPage();
    await screen.findByTestId("rules-table");
    fireEvent.click(screen.getByRole("button", { name: /Tambah aturan/ }));
    fireEvent.change(await screen.findByLabelText("Pilih jabatan"), { target: { value: "p1" } });
    fireEvent.change(screen.getByLabelText("Pilih lokasi"), { target: { value: "b1" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Aturan untuk jabatan dan lokasi ini sudah ada");
  });

  it("daftar kosong dan galat muat (403 tidak dicoba ulang)", async () => {
    api.fetchAutoCheckoutRules.mockResolvedValue([]);
    renderPage(true);
    expect(await screen.findByText("Belum ada aturan checkout otomatis.")).toBeTruthy();
    cleanup();
    api.fetchAutoCheckoutRules.mockRejectedValue(new ApiError("Gagal", 403, null));
    renderPage();
    expect(await screen.findByText("Gagal")).toBeTruthy();
  });
});
