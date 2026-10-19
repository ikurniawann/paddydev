import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { EmployeeShiftsTab } from "./employee-shifts-tab";

const api = vi.hoisted(() => ({
  applyFixedSchedule: vi.fn(async () => ({ data: { changed: 1, cleared_days: 0, warnings: [] }, message: "Jadwal tetap diterapkan ke 1 karyawan" })),
}));
vi.mock("../api", () => api);
vi.mock("../queries", () => ({
  useActiveShiftOptions: () => ({ isLoading: false, data: [{ id: "s1", name: "Non-Shift", start_time: "09:00:00", end_time: "18:00:00" }] }),
  useEmployeeShiftSchedule: () => ({ isLoading: false, data: [], refetch: vi.fn() }),
}));
vi.mock("../mutations", () => ({ useSaveEmployeeShiftPattern: () => ({ isPending: false, mutateAsync: vi.fn() }) }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
afterEach(cleanup);

it("mengunci pengaturan shift untuk karyawan Non-Shift", async () => {
  render(<EmployeeShiftsTab employeeId="e1" scheduleType="non_shift" />);
  expect(screen.getByText("Jadwal tetap (Non-Shift)")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Simpan Jadwal" })).toBeNull();
  expect(screen.getByText(/Ubah tipe jadwal ke Shift untuk mengatur shift/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Terapkan jadwal tetap" }));
  await waitFor(() => expect(api.applyFixedSchedule).toHaveBeenCalledWith(expect.objectContaining({ employee_ids: ["e1"] })));
});

it("karyawan Shift tetap bisa mengatur pola", () => {
  render(<EmployeeShiftsTab employeeId="e1" scheduleType="shift" />);
  expect(screen.getByRole("button", { name: "Simpan Jadwal" })).toBeTruthy();
});
