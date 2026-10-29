import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useRevokeDelegate, useSaveDelegate } from "./mutations";

const api = vi.hoisted(() => ({
  fetchScheduleDelegates: vi.fn(),
  fetchDelegateSuggest: vi.fn(),
  saveScheduleDelegate: vi.fn(),
  revokeScheduleDelegate: vi.fn(),
}));
vi.mock("./api", () => api);

function setup() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const spy = vi.spyOn(client, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return { spy, wrapper };
}
const invalidatedKeys = (spy: ReturnType<typeof setup>["spy"]) => spy.mock.calls.map((c) => c[0]?.queryKey);

beforeEach(() => Object.values(api).forEach((fn) => fn.mockReset()));

describe("mutasi pengatur jadwal toko (EPIC-071)", () => {
  it("useSaveDelegate menginvalidasi daftar pengatur dan akses jadwal", async () => {
    api.saveScheduleDelegate.mockResolvedValue({});
    const { spy, wrapper } = setup();
    const { result } = renderHook(() => useSaveDelegate(), { wrapper });
    await act(() => result.current.mutateAsync({ employeeId: "e1", payload: { branch_ids: ["b1"], can_place: false } }));
    expect(api.saveScheduleDelegate).toHaveBeenCalledWith("e1", { branch_ids: ["b1"], can_place: false });
    await waitFor(() => expect(invalidatedKeys(spy)).toHaveLength(2));
    expect(invalidatedKeys(spy)).toEqual(
      expect.arrayContaining([["hris", "schedule-delegates"], ["hris", "schedule-access"]])
    );
  });

  it("useRevokeDelegate menginvalidasi daftar pengatur dan akses jadwal", async () => {
    api.revokeScheduleDelegate.mockResolvedValue({ revoked: true });
    const { spy, wrapper } = setup();
    const { result } = renderHook(() => useRevokeDelegate(), { wrapper });
    await act(() => result.current.mutateAsync("e1"));
    expect(api.revokeScheduleDelegate).toHaveBeenCalledWith("e1");
    await waitFor(() => expect(invalidatedKeys(spy)).toHaveLength(2));
    expect(invalidatedKeys(spy)).toEqual(
      expect.arrayContaining([["hris", "schedule-delegates"], ["hris", "schedule-access"]])
    );
  });
});
