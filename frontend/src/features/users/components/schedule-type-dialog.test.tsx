import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ScheduleTypeDialog } from "./schedule-type-dialog";

const api = vi.hoisted(() => ({
  fetchScheduleType: vi.fn(async () => ({ data: { schedule_type: "shift", future_days: 4 } })),
  saveScheduleType: vi.fn(async () => ({ data: { changed: 1, cleared_days: 4, warnings: [] } })),
}));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));
afterEach(cleanup);

it("meminta konfirmasi sebelum mengganti Shift → Non-Shift", async () => {
  const onDone = vi.fn();
  render(<ScheduleTypeDialog open employeeId="e1" employeeName="Feti" today="2026-10-21" onOpenChange={() => {}} onDone={onDone} />);
  fireEvent.click(await screen.findByRole("radio", { name: "Non-Shift" }));
  expect(await screen.findByText(/4 jadwal per tanggal mulai 21\/10\/2026 akan dihapus/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
  await waitFor(() => expect(api.saveScheduleType).toHaveBeenCalledWith("e1", { schedule_type: "non_shift", effective_from: "2026-10-21" }));
  expect(onDone).toHaveBeenCalled();
});
