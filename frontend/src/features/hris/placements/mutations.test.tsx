import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { useCreatePlacement, useDeletePlacement, useUpdatePlacement } from "./mutations";

const api = vi.hoisted(() => ({
  createPlacement: vi.fn(),
  updatePlacement: vi.fn(),
  deletePlacement: vi.fn(),
  bulkPlacement: vi.fn(),
  bulkEndPlacement: vi.fn(),
  saveBranchLocation: vi.fn(),
  saveBranchMinWorkHours: vi.fn(),
}));
vi.mock("./api", () => api);
// Alasan hapus diisi langsung (dialog global tidak dipasang di uji hook).
vi.mock("@/lib/hris/delete-reason", () => ({
  withDeleteReason: (_title: string, fn: (id: string, reason: string) => Promise<unknown>) => (id: string) =>
    fn(id, "Salah input"),
}));

const payload = { branch_id: "b1", is_primary: true, start_date: "2026-11-01" };

function setup<T>(hook: () => T) {
  const qc = new QueryClient();
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const { result } = renderHook(hook, { wrapper });
  const keys = () => invalidate.mock.calls.map(([f]) => JSON.stringify(f?.queryKey));
  return { result, keys };
}

// Jadwal Kerja hanya menampilkan karyawan yang ditempatkan di cabang, jadi
// mengubah penempatan lewat Kelola ikut memuat ulang grid jadwal.
const WANT = ['["hris","placements"]', '["hris","schedules"]'];

describe("mutasi penempatan tunggal (Kelola)", () => {
  it("tambah memuat ulang penempatan dan jadwal", async () => {
    api.createPlacement.mockResolvedValueOnce({ data: {} });
    const { result, keys } = setup(() => useCreatePlacement("e1"));
    await act(async () => {
      result.current.mutate(payload as never);
    });
    await waitFor(() => expect(keys()).toEqual(expect.arrayContaining(WANT)));
  });

  it("ubah memuat ulang penempatan dan jadwal", async () => {
    api.updatePlacement.mockResolvedValueOnce({ data: {} });
    const { result, keys } = setup(() => useUpdatePlacement());
    await act(async () => {
      result.current.mutate({ id: "p1", payload: payload as never });
    });
    await waitFor(() => expect(keys()).toEqual(expect.arrayContaining(WANT)));
  });

  it("hapus memuat ulang penempatan dan jadwal", async () => {
    api.deletePlacement.mockResolvedValueOnce({ message: "ok" });
    const { result, keys } = setup(() => useDeletePlacement());
    await act(async () => {
      result.current.mutate("p1");
    });
    await waitFor(() => expect(keys()).toEqual(expect.arrayContaining(WANT)));
    expect(api.deletePlacement).toHaveBeenCalledWith("p1", "Salah input");
  });
});
