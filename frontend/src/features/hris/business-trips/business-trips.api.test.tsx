import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import { decideBusinessTrip, fetchBusinessTrips } from "./api";
import { useCreateBusinessTrip, useDecideBusinessTrip } from "./mutations";
import { businessTripQueryKeys } from "./query-keys";

const apiGet = vi.fn();
const apiPost = vi.fn();
vi.mock("@/lib/api-client", async (orig) => ({
  ...(await orig<typeof import("@/lib/api-client")>()),
  apiGet: (u: string) => apiGet(u),
  apiPost: (u: string, b: unknown) => apiPost(u, b),
}));

afterEach(() => {
  apiGet.mockReset();
  apiPost.mockReset();
});

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const spy = vi.spyOn(qc, "invalidateQueries");
  const Wrap = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  return { Wrap, spy };
}

describe("business-trips api", () => {
  it("menyusun URL daftar termasuk filter me dan scope approvals", async () => {
    apiGet.mockResolvedValue({ data: [] });
    await fetchBusinessTrips({ employee_id: "me" });
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/business-trips?employee_id=me");
    await fetchBusinessTrips({ scope: "approvals", status: "pending" });
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/business-trips?scope=approvals&status=pending");
  });

  it("decide memanggil /decide dengan action dan rejection_reason", async () => {
    apiPost.mockResolvedValue({ data: {} });
    await decideBusinessTrip("t1", "reject", "Bentrok");
    expect(apiPost).toHaveBeenCalledWith("/api/hris/business-trips/t1/decide", { action: "reject", rejection_reason: "Bentrok" });
  });
});

describe("business-trips mutations", () => {
  it("decide dan create menyegarkan perjalanan dinas dan absensi", async () => {
    apiPost.mockResolvedValue({ data: {} });
    const { Wrap, spy } = wrapper();
    const decide = renderHook(() => useDecideBusinessTrip(), { wrapper: Wrap });
    decide.result.current.mutate({ id: "t1", action: "approve" });
    await waitFor(() => expect(decide.result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: businessTripQueryKeys.all });
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceQueryKeys.all });

    spy.mockClear();
    const create = renderHook(() => useCreateBusinessTrip(), { wrapper: Wrap });
    create.result.current.mutate({ start_date: "2099-03-02", end_date: "2099-03-03", destination: "Bandung", purpose: "Audit stok" });
    await waitFor(() => expect(create.result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: businessTripQueryKeys.all });
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceQueryKeys.all });
  });

  it("keputusan menyegarkan badge menu dan antrean, juga saat server menolak (409 basi)", async () => {
    const { Wrap, spy } = wrapper();
    apiPost.mockResolvedValueOnce({ data: {} });
    const ok = renderHook(() => useDecideBusinessTrip(), { wrapper: Wrap });
    ok.result.current.mutate({ id: "t1", action: "approve" });
    await waitFor(() => expect(ok.result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: ["hris", "nav-badges"] });

    spy.mockClear();
    apiPost.mockRejectedValueOnce(new Error("Pengajuan sudah diproses oleh orang lain"));
    const stale = renderHook(() => useDecideBusinessTrip(), { wrapper: Wrap });
    stale.result.current.mutate({ id: "t2", action: "approve" });
    await waitFor(() => expect(stale.result.current.isError).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: ["hris", "nav-badges"] });
    expect(spy).toHaveBeenCalledWith({ queryKey: businessTripQueryKeys.all });
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceQueryKeys.all });
  });
});
