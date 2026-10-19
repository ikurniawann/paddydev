import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BulkScheduleTypeDialog } from "./bulk-schedule-type-dialog";

const api = vi.hoisted(() => ({
  bulkScheduleType: vi.fn(),
}));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));
afterEach(cleanup);

it("cek dampak dulu (dry run) lalu simpan", async () => {
  api.bulkScheduleType
    .mockResolvedValueOnce({ data: { changed: 3, cleared_days: 5, warnings: [] } })
    .mockResolvedValueOnce({ data: { changed: 3, cleared_days: 5, warnings: [] }, message: "Tipe jadwal 3 karyawan disimpan" });
  const onDone = vi.fn();
  render(<BulkScheduleTypeDialog open ids={["a", "b", "c"]} today="2026-10-21" onOpenChange={() => {}} onDone={onDone} />);
  fireEvent.click(screen.getByRole("radio", { name: "Non-Shift" }));
  fireEvent.click(screen.getByRole("button", { name: "Periksa dampak" }));
  await waitFor(() =>
    expect(api.bulkScheduleType).toHaveBeenCalledWith({
      employee_ids: ["a", "b", "c"], schedule_type: "non_shift", effective_from: "2026-10-21", dry_run: true,
    })
  );
  expect(await screen.findByText(/3 karyawan berubah tipe/)).toBeTruthy();
  expect(screen.getByText(/5 jadwal per tanggal mulai 21\/10\/2026 akan dihapus/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
  await waitFor(() => expect(api.bulkScheduleType).toHaveBeenLastCalledWith(expect.objectContaining({ dry_run: false })));
  expect(onDone).toHaveBeenCalled();
});
