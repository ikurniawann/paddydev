import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { EmployeeShiftsTab } from "./employee-shifts-tab";

const api = vi.hoisted(() => ({
  applyFixedSchedule: vi.fn(async () => ({ data: { changed: 1, cleared_days: 0, warnings: [] }, message: "Jadwal tetap diterapkan ke 1 karyawan" })),
}));
const state = vi.hoisted(() => ({ history: [] as unknown[], save: vi.fn(async () => ({ message: "ok" })) }));
vi.mock("../api", () => api);
vi.mock("../queries", () => ({
  useActiveShiftOptions: () => ({
    isLoading: false,
    data: [
      { id: "s1", name: "Non-Shift", start_time: "09:00:00", end_time: "18:00:00" },
      { id: "s2", name: "Pagi 4", start_time: "08:00:00", end_time: "12:00:00" },
      { id: "s3", name: "Sore 4", start_time: "17:00:00", end_time: "21:00:00" },
    ],
  }),
  useEmployeeShiftSchedule: () => ({ isLoading: false, data: state.history, refetch: vi.fn() }),
}));
vi.mock("../mutations", () => ({ useSaveEmployeeShiftPattern: () => ({ isPending: false, mutateAsync: state.save }) }));
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

it("pola berjalan dengan dua shift dimuat dan dikirim ulang dengan shift2_id (tidak hilang saat disimpan)", async () => {
  state.history = [
    { id: "r1", day_of_week: 1, shift_id: "s2", shift_name: "Pagi 4", shift2_id: "s3", shift2_name: "Sore 4", effective_from: "2026-01-01", effective_to: null, start_time: "08:00:00", end_time: "12:00:00" },
  ];
  state.save.mockClear();
  render(<EmployeeShiftsTab employeeId="e1" scheduleType="shift" />);
  expect(screen.getByText(/Sen Pagi 4 \+ Sore 4/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Simpan Jadwal" }));
  await waitFor(() => expect(state.save).toHaveBeenCalled());
  const { days } = (state.save.mock.calls[0] as unknown as [{ days: Record<string, unknown>[] }])[0];
  expect(days[0]).toEqual({ day_of_week: 1, shift_id: "s2", shift2_id: "s3" });
  expect(days[1]).toEqual({ day_of_week: 2, shift_id: null });
  state.history = [];
});
