import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { useDeleteShift, useSaveShift } from "./mutations";

const api = vi.hoisted(() => ({ saveShift: vi.fn(), deleteShift: vi.fn() }));
vi.mock("./api", () => api);
vi.mock("@/lib/hris/delete-reason", () => ({
  withDeleteReason: (_title: string, fn: (id: string, reason: string) => Promise<unknown>) => (id: string) =>
    fn(id, "Tidak dipakai"),
}));

function setup<T>(hook: () => T) {
  const qc = new QueryClient();
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  const { result } = renderHook(hook, { wrapper });
  const keys = () => invalidate.mock.calls.map(([f]) => JSON.stringify(f?.queryKey));
  return { result, keys };
}

// Jam shift tampil di grid jadwal dan ringkasan Shift Tim ESS.
const WANT = ['["hris","shifts"]', '["hris","schedules"]', '["hris","ess","team"]'];

describe("mutasi shift", () => {
  it("simpan memuat ulang shift, jadwal, dan Shift Tim ESS", async () => {
    api.saveShift.mockResolvedValueOnce({ data: {} });
    const { result, keys } = setup(() => useSaveShift());
    await act(async () => {
      result.current.mutate({ payload: {} as never });
    });
    await waitFor(() => expect(keys()).toEqual(expect.arrayContaining(WANT)));
  });

  it("hapus memuat ulang shift, jadwal, dan Shift Tim ESS", async () => {
    api.deleteShift.mockResolvedValueOnce({ message: "ok" });
    const { result, keys } = setup(() => useDeleteShift());
    await act(async () => {
      result.current.mutate("s1");
    });
    await waitFor(() => expect(keys()).toEqual(expect.arrayContaining(WANT)));
  });
});
