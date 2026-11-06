import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ScheduledResignBanner } from "./scheduled-resign-banner";

const api = vi.hoisted(() => ({ fetchScheduledResign: vi.fn(), cancelScheduledResign: vi.fn() }));
vi.mock("../../api", async (importOriginal) => ({ ...(await importOriginal<typeof import("../../api")>()), ...api }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function renderBanner(props: { canCancel: boolean; onChanged?: () => void }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <ScheduledResignBanner employeeId="e1" canCancel={props.canCancel} onChanged={props.onChanged ?? (() => {})} />
    </QueryClientProvider>
  );
}

const schedule = {
  id: "s1",
  effective_date: "2026-11-20",
  resign_type: "resign",
  reason: "Pindah kerja",
  created_by_name: "HR Admin",
  created_at: "2026-11-06T03:00:00Z",
};

beforeEach(() => {
  api.fetchScheduledResign.mockReset();
  api.cancelScheduledResign.mockReset();
});
afterEach(cleanup);

describe("ScheduledResignBanner (EPIC-095)", () => {
  it("menampilkan jadwal dan membatalkannya", async () => {
    api.fetchScheduledResign.mockResolvedValue(schedule);
    api.cancelScheduledResign.mockResolvedValue({ message: "Jadwal resign dibatalkan" });
    const onChanged = vi.fn();
    renderBanner({ canCancel: true, onChanged });
    expect(await screen.findByText("Resign terjadwal: Resign (mengundurkan diri) efektif 20-11-2026 (hari terakhir kerja)")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Batalkan jadwal" }));
    fireEvent.click(await screen.findByRole("button", { name: "Ya, batalkan" }));
    await waitFor(() => expect(api.cancelScheduledResign).toHaveBeenCalledWith("e1", null));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
  });

  it("tanpa hak: tanpa tombol batal", async () => {
    api.fetchScheduledResign.mockResolvedValue(schedule);
    renderBanner({ canCancel: false });
    await screen.findByText(/Resign terjadwal:/);
    expect(screen.queryByRole("button", { name: "Batalkan jadwal" })).toBeNull();
  });

  it("tanpa jadwal: tidak merender apa pun", async () => {
    api.fetchScheduledResign.mockResolvedValue(null);
    const { container } = renderBanner({ canCancel: true });
    await waitFor(() => expect(api.fetchScheduledResign).toHaveBeenCalled());
    expect(container.innerHTML).toBe("");
  });
});
