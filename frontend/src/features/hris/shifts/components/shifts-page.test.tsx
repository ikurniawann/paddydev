import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ShiftRow } from "../types";
import { ShiftsPage } from "./shifts-page";

const api = vi.hoisted(() => ({
  fetchShifts: vi.fn(),
  saveShift: vi.fn(),
  deleteShift: vi.fn(),
}));
const toast = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast }));
vi.mock("@/lib/hris/delete-reason", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/hris/delete-reason")>()),
  // dialog alasan global tidak dipasang di test: langsung beri alasan
  withDeleteReason: (_title: string, fn: (arg: string, reason: string) => Promise<unknown>) => (arg: string) =>
    fn(arg, "alasan uji coba"),
}));
vi.mock("./shift-form-dialog", () => ({ ShiftFormDialog: () => null }));

const shift = (over: Partial<ShiftRow>): ShiftRow => ({
  id: over.code ?? "x",
  code: "X",
  name: "X",
  start_time: "08:00:00",
  end_time: "16:00:00",
  break_minutes: 60,
  late_tolerance_minutes: 10,
  is_overnight: false,
  is_active: true,
  sort_order: 0,
  is_default: false,
  employees_using: 0,
  ...over,
});

const rows: ShiftRow[] = [
  shift({ code: "SPG13", name: "SPG13 11:00-19:00", start_time: "11:00:00", end_time: "19:00:00", employees_using: 12 }),
  shift({ code: "PAGI8", name: "Pagi 8", start_time: "07:00:00", end_time: "15:00:00", employees_using: 0 }),
  shift({ code: "PAGI2", name: "Pagi 2", start_time: "09:00:00", end_time: "17:00:00", employees_using: 3, is_active: false }),
  shift({ code: "CS2", name: "CS siang", start_time: "10:00:00", end_time: "18:00:00", employees_using: 4 }),
  shift({ code: "DEF", name: "Default", is_default: true, employees_using: 40 }),
  shift({ code: "NS", name: "Non-Shift", is_default: true, employees_using: 5 }),
  shift({ code: "MLM", name: "Malam", start_time: "22:00:00", end_time: "06:00:00", is_overnight: true }),
];

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <ShiftsPage />
    </QueryClientProvider>
  );
  return client;
}

const rowOf = (code: string) => screen.getByText(code).closest("tr")!;

beforeEach(() => {
  api.fetchShifts.mockResolvedValue(rows);
  api.saveShift.mockResolvedValue({});
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ShiftsPage", () => {
  it("mengelompokkan shift per awalan kode, Bawaan paling atas", async () => {
    renderPage();
    await screen.findByText("SPG13");
    const heads = screen.getAllByTestId("shift-group").map((el) => el.textContent);
    expect(heads.map((h) => h?.split(" · ")[0])).toEqual(["Bawaan", "CS", "MLM", "PAGI", "SPG"]);
    expect(heads[3]).toContain("2 shift");
    // di dalam grup, urut menurut angka (PAGI2 sebelum PAGI8)
    const codes = screen.getAllByTestId("shift-code").map((el) => el.textContent);
    expect(codes).toEqual(["DEF", "NS", "CS2", "MLM", "PAGI2", "PAGI8", "SPG13"]);
  });

  it("menampilkan kolom ringkas: jam, istirahat, toleransi, dipakai, status", async () => {
    renderPage();
    await screen.findByText("SPG13");
    const spg = rowOf("SPG13");
    expect(within(spg).getByText("11:00–19:00")).toBeTruthy();
    expect(within(spg).getByText("60 mnt")).toBeTruthy();
    expect(within(spg).getByText("10 mnt")).toBeTruthy();
    expect(within(spg).getByText("12 karyawan")).toBeTruthy();
    expect(within(spg).getByText("Aktif")).toBeTruthy();
    expect(within(rowOf("PAGI2")).getByText("Nonaktif")).toBeTruthy();
    expect(within(rowOf("PAGI8")).getByText("Belum dipakai")).toBeTruthy();
    expect(within(rowOf("MLM")).getByLabelText("Lewat tengah malam")).toBeTruthy();
  });

  it("pencarian menyaring kode, nama, dan jam", async () => {
    renderPage();
    await screen.findByText("SPG13");
    const box = screen.getByPlaceholderText(/Cari kode, nama, atau jam/);

    fireEvent.change(box, { target: { value: "pagi" } });
    expect(screen.queryByText("SPG13")).toBeNull();
    expect(screen.getByText("PAGI8")).toBeTruthy();
    expect(screen.getByText("PAGI2")).toBeTruthy();

    fireEvent.change(box, { target: { value: "11:00" } });
    expect(screen.getByText("SPG13")).toBeTruthy();
    expect(screen.queryByText("PAGI8")).toBeNull();

    fireEvent.change(box, { target: { value: "cs2" } });
    expect(screen.getAllByTestId("shift-code").map((el) => el.textContent)).toEqual(["CS2"]);

    fireEvent.change(box, { target: { value: "zzz" } });
    expect(screen.getByText(/Tidak ada shift yang cocok/)).toBeTruthy();
  });

  it("shift yang dipakai: tanpa tombol Hapus/Nonaktifkan, ada petunjuk ganti jadwal dulu", async () => {
    renderPage();
    await screen.findByText("SPG13");
    const spg = rowOf("SPG13");
    expect(within(spg).queryByRole("button", { name: /Hapus|Nonaktifkan/ })).toBeNull();
    expect(within(spg).getByText("Ganti dulu jadwal karyawan yang memakainya")).toBeTruthy();
  });

  it("shift aktif yang belum dipakai bisa dihapus atau dinonaktifkan; yang nonaktif bisa diaktifkan", async () => {
    renderPage();
    await screen.findByText("SPG13");
    const pagi8 = rowOf("PAGI8");
    expect(within(pagi8).getByRole("button", { name: /Hapus/ })).toBeTruthy();
    expect(within(pagi8).getByRole("button", { name: "Nonaktifkan" })).toBeTruthy();
    expect(within(pagi8).queryByText(/Ganti dulu jadwal/)).toBeNull();
    const pagi2 = rowOf("PAGI2");
    expect(within(pagi2).getByRole("button", { name: "Aktifkan" })).toBeTruthy();
    expect(within(pagi2).queryByRole("button", { name: /Hapus|Nonaktifkan/ })).toBeNull();
  });

  it("DEF dan NS tidak bisa dihapus atau dinonaktifkan, walau belum dipakai siapa pun", async () => {
    api.fetchShifts.mockResolvedValue([
      shift({ code: "DEF", name: "Default", is_default: true, employees_using: 0 }),
      shift({ code: "NS", name: "Non-Shift", is_default: true, employees_using: 0 }),
    ]);
    renderPage();
    await screen.findByText("DEF");
    for (const code of ["DEF", "NS"]) {
      const tr = rowOf(code);
      expect(within(tr).queryByRole("button", { name: /Hapus|Nonaktifkan/ })).toBeNull();
      expect(within(tr).getByRole("button", { name: /Edit/ })).toBeTruthy();
    }
  });

  it("Nonaktifkan meminta konfirmasi lalu menyimpan is_active=false dan menyegarkan data", async () => {
    const client = renderPage();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    await screen.findByText("PAGI8");
    fireEvent.click(within(rowOf("PAGI8")).getByRole("button", { name: "Nonaktifkan" }));
    expect(api.saveShift).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: "Ya, nonaktifkan" }));
    await waitFor(() => expect(api.saveShift).toHaveBeenCalledWith({ is_active: false }, "PAGI8"));
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    const keys = invalidate.mock.calls.map((c) => (c[0] as { queryKey: unknown[] }).queryKey);
    expect(keys).toContainEqual(["hris", "shifts"]);
    expect(keys).toContainEqual(["users", "shift-options"]);
    expect(keys).toContainEqual(["hris", "schedules"]);
  });

  it("menampilkan pesan server bila shift tak bisa dinonaktifkan", async () => {
    api.saveShift.mockRejectedValue(new Error("Shift masih dipakai jadwal 3 karyawan"));
    renderPage();
    await screen.findByText("PAGI8");
    fireEvent.click(within(rowOf("PAGI8")).getByRole("button", { name: "Nonaktifkan" }));
    fireEvent.click(await screen.findByRole("button", { name: "Ya, nonaktifkan" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Shift masih dipakai jadwal 3 karyawan"));
  });

  it("hapus memakai pesan server pada toast sukses, dengan cadangan teks bila kosong", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    api.deleteShift.mockResolvedValueOnce({ message: "Shift PAGI8 dihapus" });
    renderPage();
    await screen.findByText("PAGI8");
    fireEvent.click(within(rowOf("PAGI8")).getByRole("button", { name: /Hapus/ }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Shift PAGI8 dihapus"));
    expect(api.deleteShift).toHaveBeenCalledWith("PAGI8", "alasan uji coba");

    api.deleteShift.mockResolvedValueOnce({});
    fireEvent.click(within(rowOf("PAGI8")).getByRole("button", { name: /Hapus/ }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Shift dihapus / dinonaktifkan"));
  });

  it("Aktifkan tidak bisa dikirim dua kali selagi menyimpan", async () => {
    let resolve!: (v: unknown) => void;
    api.saveShift.mockReturnValue(new Promise((r) => (resolve = r)));
    renderPage();
    await screen.findByText("PAGI2");
    const btn = within(rowOf("PAGI2")).getByRole("button", { name: "Aktifkan" }) as HTMLButtonElement;
    fireEvent.click(btn);
    await waitFor(() => expect(btn.disabled).toBe(true));
    fireEvent.click(btn);
    expect(api.saveShift).toHaveBeenCalledTimes(1);
    resolve({});
  });

  it("Aktifkan langsung menyimpan is_active=true", async () => {
    renderPage();
    await screen.findByText("PAGI2");
    fireEvent.click(within(rowOf("PAGI2")).getByRole("button", { name: "Aktifkan" }));
    await waitFor(() => expect(api.saveShift).toHaveBeenCalledWith({ is_active: true }, "PAGI2"));
  });
});
