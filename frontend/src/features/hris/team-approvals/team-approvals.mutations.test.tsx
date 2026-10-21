import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

const apiPost = vi.fn();
vi.mock("@/lib/api-client", () => ({ apiGet: vi.fn(), apiPost: (...a: unknown[]) => apiPost(...a) }));

import { useDecideTeamRequest } from "./mutations";

function setup() {
  const qc = new QueryClient();
  const spy = vi.spyOn(qc, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { spy, ...renderHook(() => useDecideTeamRequest(), { wrapper }) };
}

afterEach(() => apiPost.mockReset());

describe("useDecideTeamRequest", () => {
  it("cuti memakai /leaves/approve dan menyegarkan antrean + badge", async () => {
    apiPost.mockResolvedValue({});
    const { result, spy } = setup();
    act(() => result.current.mutate({ kind: "leave", id: "l1", action: "reject", rejectionReason: "Penuh" }));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(apiPost).toHaveBeenCalledWith("/api/hris/leaves/approve", {
      leave_id: "l1",
      action: "reject",
      rejection_reason: "Penuh",
    });
    const keys = spy.mock.calls.map((c) => c[0]?.queryKey);
    expect(keys).toContainEqual(["hris", "team-approvals"]);
    expect(keys).toContainEqual(["hris", "nav-badges"]);
  });

  it("lembur memakai /overtime/decide; antrean tetap disegarkan saat server menolak (409 basi)", async () => {
    apiPost.mockRejectedValue(new Error("Pengajuan sudah diproses oleh orang lain"));
    const { result, spy } = setup();
    act(() => result.current.mutate({ kind: "overtime", id: "o1", action: "approve" }));
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(apiPost).toHaveBeenCalledWith("/api/hris/overtime/decide", {
      overtime_id: "o1",
      action: "approve",
      rejection_reason: undefined,
    });
    const keys = spy.mock.calls.map((c) => c[0]?.queryKey);
    expect(keys).toContainEqual(["hris", "team-approvals"]);
    expect(keys).toContainEqual(["hris", "nav-badges"]);
  });
});
