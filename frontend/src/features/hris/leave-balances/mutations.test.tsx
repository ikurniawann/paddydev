import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api-client";
import { usePreviewLeaveSettings, useResolveCarryReview, useSaveLeaveSettings } from "./mutations";
import { leaveBalanceQueryKeys } from "./query-keys";
import type { LeaveSettings } from "./types";

const api = vi.hoisted(() => ({
  saveLeaveSettings: vi.fn(),
  updateLeaveBalance: vi.fn(),
  previewLeaveSettings: vi.fn(),
  resolveCarryReview: vi.fn(),
}));
vi.mock("./api", () => api);

const old: LeaveSettings = {
  carry_over_enabled: false,
  carry_over_max_days: null,
  carry_over_expiry_month: 3,
  carry_over_expiry_day: 31,
  updated_at: null,
  updated_by_name: null,
  carry_over_ready: true,
};

describe("useSaveLeaveSettings", () => {
  it("menaruh respons PUT di cache sebelum onSuccess pemanggil, agar kartu tidak berkedip nilai lama", async () => {
    const saved: LeaveSettings = { ...old, carry_over_enabled: true, carry_over_max_days: 5 };
    api.saveLeaveSettings.mockResolvedValueOnce({ data: saved, message: "ok" });
    const qc = new QueryClient();
    qc.setQueryData(leaveBalanceQueryKeys.settings(), old);
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useSaveLeaveSettings(), { wrapper });

    let seenInCallback: unknown;
    await act(async () => {
      result.current.mutate(
        { carry_over_enabled: true, reason: "Uji" },
        { onSuccess: () => (seenInCallback = qc.getQueryData(leaveBalanceQueryKeys.settings())) }
      );
    });
    await waitFor(() => expect(seenInCallback).toEqual(saved));
  });

  it("setelah simpan, daftar saldo dan tinjauan dimuat ulang (batas baru menghitung ulang tahun berjalan)", async () => {
    api.saveLeaveSettings.mockResolvedValueOnce({ data: old, message: "ok" });
    const qc = new QueryClient();
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useSaveLeaveSettings(), { wrapper });
    await act(async () => {
      result.current.mutate({ carry_over_max_days: 5, reason: "Kebijakan" });
    });
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: leaveBalanceQueryKeys.all }));
    expect(api.saveLeaveSettings).toHaveBeenCalledWith({ carry_over_max_days: 5, reason: "Kebijakan" });
  });
});

describe("usePreviewLeaveSettings", () => {
  it("memanggil pratinjau tanpa mengubah cache", async () => {
    const impact = { employees_changed: 1, days_increase: 2, days_decrease: 0, needs_review: 0 };
    api.previewLeaveSettings.mockResolvedValueOnce(impact);
    const qc = new QueryClient();
    qc.setQueryData(leaveBalanceQueryKeys.settings(), old);
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => usePreviewLeaveSettings(), { wrapper });
    await act(async () => {
      result.current.mutate({ carry_over_max_days: 3 });
    });
    await waitFor(() => expect(result.current.data).toEqual(impact));
    expect(api.previewLeaveSettings).toHaveBeenCalledWith({ carry_over_max_days: 3 });
    expect(qc.getQueryData(leaveBalanceQueryKeys.settings())).toEqual(old);
  });
});

describe("useResolveCarryReview", () => {
  it("mengirim id dan catatan lalu memuat ulang saldo dan tinjauan", async () => {
    api.resolveCarryReview.mockResolvedValueOnce({ data: {}, message: "ok" });
    const qc = new QueryClient();
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useResolveCarryReview(), { wrapper });
    await act(async () => {
      result.current.mutate({ id: "r1", note: "Dibahas" });
    });
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: leaveBalanceQueryKeys.all }));
    expect(api.resolveCarryReview).toHaveBeenCalledWith("r1", "Dibahas");
    expect(leaveBalanceQueryKeys.reviews(2026).slice(0, 2)).toEqual(leaveBalanceQueryKeys.all);
  });

  it.each([404, 409])("galat %i (tinjauan hilang atau sudah ditutup) memuat ulang daftar", async (status) => {
    api.resolveCarryReview.mockRejectedValueOnce(new ApiError("Tinjauan sudah diselesaikan", status, {}));
    const qc = new QueryClient();
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useResolveCarryReview(), { wrapper });
    await act(async () => {
      result.current.mutate({ id: "r1", note: "Dibahas" });
    });
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: leaveBalanceQueryKeys.all }));
  });

  it("galat lain (mis. 400) tidak memuat ulang", async () => {
    api.resolveCarryReview.mockRejectedValueOnce(new ApiError("Catatan wajib diisi", 400, {}));
    const qc = new QueryClient();
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => useResolveCarryReview(), { wrapper });
    await act(async () => {
      result.current.mutate({ id: "r1", note: "x" });
    });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(invalidate).not.toHaveBeenCalled();
  });
});
