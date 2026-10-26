import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { useSaveLeaveSettings } from "./mutations";
import { leaveBalanceQueryKeys } from "./query-keys";
import type { LeaveSettings } from "./types";

const api = vi.hoisted(() => ({ saveLeaveSettings: vi.fn(), updateLeaveBalance: vi.fn() }));
vi.mock("./api", () => api);

const old: LeaveSettings = {
  carry_over_enabled: false,
  carry_over_max_days: null,
  carry_over_expiry_month: 3,
  carry_over_expiry_day: 31,
  updated_at: null,
  updated_by_name: null,
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
        { carry_over_enabled: true },
        { onSuccess: () => (seenInCallback = qc.getQueryData(leaveBalanceQueryKeys.settings())) }
      );
    });
    await waitFor(() => expect(seenInCallback).toEqual(saved));
  });
});
