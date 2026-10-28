import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { useSaveEmployeeShiftPattern } from "./mutations";

const api = vi.hoisted(() => ({ saveEmployeeShiftPattern: vi.fn() }));
vi.mock("./api", () => api);

describe("useSaveEmployeeShiftPattern", () => {
  it("menyimpan pola mingguan lalu menyegarkan jadwal karyawan, grid jadwal, dan penempatan", async () => {
    api.saveEmployeeShiftPattern.mockResolvedValue({});
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
    const invalidate = vi.spyOn(client, "invalidateQueries");
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useSaveEmployeeShiftPattern("emp-1"), { wrapper });

    await act(async () => {
      await result.current.mutateAsync({} as never);
    });

    await waitFor(() => expect(invalidate).toHaveBeenCalled());
    const keys = invalidate.mock.calls.map((c) => (c[0] as { queryKey: unknown[] }).queryKey);
    expect(keys).toContainEqual(["users", "shift-schedule", "emp-1"]);
    expect(keys).toContainEqual(["hris", "schedules"]);
    expect(keys).toContainEqual(["hris", "placements"]);
  });
});
