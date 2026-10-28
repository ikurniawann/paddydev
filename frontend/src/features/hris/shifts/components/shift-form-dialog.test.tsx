import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ShiftRow } from "../types";
import { ShiftFormDialog } from "./shift-form-dialog";

const api = vi.hoisted(() => ({ fetchShifts: vi.fn(), saveShift: vi.fn(), deleteShift: vi.fn() }));
const toast = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast }));

const existing: ShiftRow = {
  id: "s1",
  code: "SPG13",
  name: "SPG13 11:00-19:00",
  start_time: "11:00:00",
  end_time: "19:00:00",
  break_minutes: 60,
  late_tolerance_minutes: 10,
  is_overnight: false,
  is_active: true,
  sort_order: 0,
  employees_using: 12,
};

function renderDialog(shift: ShiftRow | null) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <ShiftFormDialog open shift={shift} onOpenChange={onOpenChange} />
    </QueryClientProvider>
  );
  return { client, onOpenChange };
}

beforeEach(() => {
  api.saveShift.mockResolvedValue({});
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ShiftFormDialog", () => {
  it("menampilkan ringkasan masuk, batas terlambat, dan pulang", () => {
    renderDialog(existing);
    expect(screen.getByText("Masuk 11:00 · terlambat setelah 11:10 · pulang 19:00")).toBeTruthy();
  });

  it("ringkasan ikut berubah saat jam atau toleransi diubah", () => {
    renderDialog(existing);
    fireEvent.change(screen.getByLabelText("Jam mulai"), { target: { value: "07:30" } });
    fireEvent.change(screen.getByLabelText("Toleransi terlambat (menit)"), { target: { value: "15" } });
    expect(screen.getByText("Masuk 07:30 · terlambat setelah 07:45 · pulang 19:00")).toBeTruthy();
  });

  it("peringatan pemakai tampil saat edit shift yang dipakai", () => {
    renderDialog(existing);
    expect(screen.getByText(/Dipakai 12 karyawan/)).toBeTruthy();
  });

  it("tanpa peringatan untuk shift baru atau yang belum dipakai", () => {
    renderDialog(null);
    expect(screen.queryByText(/Dipakai \d+ karyawan/)).toBeNull();
    cleanup();
    renderDialog({ ...existing, employees_using: 0 });
    expect(screen.queryByText(/Dipakai \d+ karyawan/)).toBeNull();
  });

  it("menyarankan nama dari kode + jam bila nama kosong, dan memakainya saat simpan", async () => {
    renderDialog(null);
    fireEvent.change(screen.getByLabelText(/Kode/), { target: { value: "pagi8" } });
    fireEvent.change(screen.getByLabelText("Jam mulai"), { target: { value: "07:00" } });
    fireEvent.change(screen.getByLabelText("Jam selesai"), { target: { value: "15:00" } });
    const name = screen.getByLabelText("Nama shift") as HTMLInputElement;
    expect(name.value).toBe("");
    expect(name.placeholder).toContain("PAGI8 07:00–15:00");
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(api.saveShift).toHaveBeenCalled());
    expect(api.saveShift.mock.calls[0][0]).toMatchObject({ code: "PAGI8", name: "PAGI8 07:00–15:00" });
    expect(api.saveShift.mock.calls[0][1]).toBeUndefined();
  });

  it("nama yang diisi sendiri tidak ditimpa saran", async () => {
    renderDialog(null);
    fireEvent.change(screen.getByLabelText(/Kode/), { target: { value: "PAGI8" } });
    fireEvent.change(screen.getByLabelText("Nama shift"), { target: { value: "Pagi toko" } });
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(api.saveShift).toHaveBeenCalled());
    expect(api.saveShift.mock.calls[0][0]).toMatchObject({ name: "Pagi toko" });
  });

  it("menyimpan menyegarkan daftar shift, pilihan shift karyawan, dan jadwal", async () => {
    const { client, onOpenChange } = renderDialog(existing);
    const invalidate = vi.spyOn(client, "invalidateQueries");
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(api.saveShift).toHaveBeenCalledWith(expect.objectContaining({ code: "SPG13" }), "s1");
    const keys = invalidate.mock.calls.map((c) => (c[0] as { queryKey: unknown[] }).queryKey);
    expect(keys).toContainEqual(["hris", "shifts"]);
    expect(keys).toContainEqual(["users", "shift-options"]);
    expect(keys).toContainEqual(["hris", "schedules"]);
  });

  it("menampilkan pesan server saat gagal menyimpan", async () => {
    api.saveShift.mockRejectedValue(new Error("Kode sudah dipakai"));
    renderDialog(existing);
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Kode sudah dipakai"));
  });
});
