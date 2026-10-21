import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import {
  attendanceRequestAttachmentUrl,
  cancelAttendanceRequest,
  createAttendanceRequest,
  decideAttendanceRequest,
  fetchAttendanceRequests,
  fetchMyAttendanceGaps,
  fetchMyAttendanceRequests,
} from "./api";
import { useCancelAttendanceRequest, useCreateAttendanceRequest, useDecideAttendanceRequest } from "./mutations";
import { attendanceRequestQueryKeys } from "./query-keys";

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

describe("attendance-requests api", () => {
  it("menyusun URL antrean HR, milik sendiri, dan absen terlewat", async () => {
    apiGet.mockResolvedValue({ data: [] });
    await fetchAttendanceRequests({ status: "pending" });
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/attendance-requests?status=pending");
    await fetchAttendanceRequests({ status: "approved", employee_id: "e1" });
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/attendance-requests?status=approved&employee_id=e1");
    await fetchAttendanceRequests();
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/attendance-requests");
    await fetchMyAttendanceRequests();
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/me/attendance-requests");
    apiGet.mockResolvedValue({ data: { start: "2026-10-01", end: "2026-10-04", days: [] } });
    expect(await fetchMyAttendanceGaps()).toEqual({ start: "2026-10-01", end: "2026-10-04", days: [] });
    expect(apiGet).toHaveBeenLastCalledWith("/api/hris/me/attendance-gaps");
  });

  it("mengirim pengajuan, keputusan, dan pembatalan ke endpoint Go", async () => {
    apiPost.mockResolvedValue({ data: {} });
    const payload = { date: "2026-10-03", clock_in: null, clock_out: "17:00", reason: "Lupa absen pulang" };
    await createAttendanceRequest(payload);
    expect(apiPost).toHaveBeenLastCalledWith("/api/hris/attendance-requests", payload);
    await decideAttendanceRequest("r1", "reject", "Tidak sesuai CCTV");
    expect(apiPost).toHaveBeenLastCalledWith("/api/hris/attendance-requests/r1/decide", {
      action: "reject",
      rejection_reason: "Tidak sesuai CCTV",
    });
    await cancelAttendanceRequest("r1");
    expect(apiPost).toHaveBeenLastCalledWith("/api/hris/attendance-requests/r1/cancel", {});
    expect(attendanceRequestAttachmentUrl("attendance-requests/e1/a.png")).toBe(
      "/api/hris/attendance-requests/attachment/attendance-requests/e1/a.png"
    );
  });
});

describe("attendance-requests mutations", () => {
  it("keputusan menyegarkan pengajuan, absensi, dan badge menu, juga saat gagal (antrean basi)", async () => {
    apiPost.mockRejectedValue(new Error("Pengajuan sudah diproses oleh orang lain"));
    const { Wrap, spy } = wrapper();
    const { result } = renderHook(() => useDecideAttendanceRequest(), { wrapper: Wrap });
    result.current.mutate({ id: "r1", action: "approve" });
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceRequestQueryKeys.all });
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceQueryKeys.all });
    expect(spy).toHaveBeenCalledWith({ queryKey: ["hris", "nav-badges"] });
  });

  it("keputusan juga menyegarkan periode absensi dan payroll (persetujuan menulis absensi)", async () => {
    apiPost.mockResolvedValue({ data: {}, message: "ok" });
    const { Wrap, spy } = wrapper();
    const { result } = renderHook(() => useDecideAttendanceRequest(), { wrapper: Wrap });
    result.current.mutate({ id: "r1", action: "approve" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceQueryKeys.periodsAll() });
    expect(spy).toHaveBeenCalledWith({ queryKey: ["hris", "payroll"] });
  });

  it("pengajuan baru dan pembatalan tidak menyegarkan periode absensi maupun payroll", async () => {
    apiPost.mockResolvedValue({ data: {} });
    const { Wrap, spy } = wrapper();
    const create = renderHook(() => useCreateAttendanceRequest(), { wrapper: Wrap });
    create.result.current.mutate({ date: "2026-10-03", clock_in: null, clock_out: "17:00", reason: "Lupa absen pulang" });
    await waitFor(() => expect(create.result.current.isSuccess).toBe(true));
    const cancel = renderHook(() => useCancelAttendanceRequest(), { wrapper: Wrap });
    cancel.result.current.mutate("r1");
    await waitFor(() => expect(cancel.result.current.isSuccess).toBe(true));
    expect(spy).not.toHaveBeenCalledWith({ queryKey: attendanceQueryKeys.periodsAll() });
    expect(spy).not.toHaveBeenCalledWith({ queryKey: ["hris", "payroll"] });
  });

  it("pembatalan menyegarkan juga saat gagal (sudah diputus HR)", async () => {
    apiPost.mockRejectedValue(new Error("Pengajuan sudah disetujui"));
    const { Wrap, spy } = wrapper();
    const { result } = renderHook(() => useCancelAttendanceRequest(), { wrapper: Wrap });
    result.current.mutate("r1");
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceRequestQueryKeys.all });
  });

  it("pengajuan baru menyegarkan daftar & absen terlewat", async () => {
    apiPost.mockResolvedValue({ data: {} });
    const { Wrap, spy } = wrapper();
    const { result } = renderHook(() => useCreateAttendanceRequest(), { wrapper: Wrap });
    result.current.mutate({ date: "2026-10-03", clock_in: null, clock_out: "17:00", reason: "Lupa absen pulang" });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(spy).toHaveBeenCalledWith({ queryKey: attendanceRequestQueryKeys.all });
  });
});
