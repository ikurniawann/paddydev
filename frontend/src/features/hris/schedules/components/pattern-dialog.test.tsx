import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { addDays } from "../schedule-cells";
import type { ScheduleGrid } from "../types";
import { PatternDialog } from "./pattern-dialog";

const api = vi.hoisted(() => ({
  saveScheduleCells: vi.fn(),
  fetchScheduleGrid: vi.fn(),
}));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));
afterEach(cleanup);
beforeEach(() => {
  api.saveScheduleCells.mockReset();
  api.fetchScheduleGrid.mockReset();
});

const shifts = [
  { code: "P", name: "Pagi", start_time: "08:00:00", end_time: "16:00:00" },
  { code: "S", name: "Siang", start_time: "13:00:00", end_time: "21:00:00" },
];
const DAYS = ["Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"];

function rangeGrid(from: string, to: string, ids: string[]): { data: ScheduleGrid } {
  const dates: string[] = [];
  for (let d = from; d <= to; d = addDays(d, 1)) dates.push(d);
  return {
    data: {
      branch: { id: "b1", name: "PVJ", code: "PVJ" },
      dates: dates.map((date) => ({ date, label: date, holiday: null })),
      employees: ids.map((id) => ({ id, nip: id, name: id, position: "", cells: dates.map((date) => ({ date, code: "", source: "none" as const })) })),
      non_shift_count: 0,
      shifts,
    },
  };
}

function renderDialog(over: Partial<React.ComponentProps<typeof PatternDialog>> = {}) {
  const onSaved = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <PatternDialog
        open
        onOpenChange={() => {}}
        branchId="b1"
        employees={[
          { id: "e1", name: "Ana" },
          { id: "e2", name: "Budi" },
        ]}
        shifts={shifts}
        defaultFrom="2026-11-02"
        today="2026-11-02"
        onSaved={onSaved}
        {...over}
      />
    </QueryClientProvider>
  );
  return { onSaved };
}

function choose(day: string, value: string) {
  fireEvent.change(screen.getByLabelText(day), { target: { value } });
}

it("7 pilihan hari + rentang default s/d +27 hari, maks 62 hari", () => {
  renderDialog();
  for (const d of DAYS) expect(screen.getByLabelText(d)).toBeTruthy();
  expect((screen.getByLabelText("Dari tanggal") as HTMLInputElement).value).toBe("2026-11-02");
  expect((screen.getByLabelText("Sampai tanggal") as HTMLInputElement).value).toBe("2026-11-29");
  const apply = screen.getByRole("button", { name: "Terapkan" }) as HTMLButtonElement;
  expect(apply.disabled).toBe(true); // hari belum dipilih semua
  DAYS.forEach((d, i) => choose(d, i < 5 ? "P" : "L"));
  expect(apply.disabled).toBe(false);
  fireEvent.change(screen.getByLabelText("Sampai tanggal"), { target: { value: "2027-01-03" } });
  expect(screen.getByText("Rentang maksimal 62 hari")).toBeTruthy();
  expect(apply.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("Sampai tanggal"), { target: { value: "2027-01-02" } });
  expect(apply.disabled).toBe(false);
  fireEvent.change(screen.getByLabelText("Dari tanggal"), { target: { value: "2026-11-01" } });
  expect(screen.getByText("Tanggal mulai tidak boleh sebelum hari ini")).toBeTruthy();
});

it("Terapkan → pratinjau → konfirmasi → simpan", async () => {
  api.fetchScheduleGrid.mockResolvedValueOnce(rangeGrid("2026-11-02", "2026-11-29", ["e1", "e2"]));
  api.saveScheduleCells
    .mockResolvedValueOnce({
      data: {
        summary: { employees: 2, new: 56, changed: 0, same: 0, cleared: 0 },
        errors: [],
        warnings: [],
        fingerprint: "fpp",
        changes: [],
        replaced_days: 0,
      },
    })
    .mockResolvedValueOnce({ data: { summary: {} }, message: "Jadwal 2 karyawan disimpan" });
  const { onSaved } = renderDialog();
  DAYS.forEach((d, i) => choose(d, i < 5 ? "P" : "L"));
  fireEvent.click(screen.getByRole("button", { name: "Terapkan" }));

  await waitFor(() =>
    expect(api.fetchScheduleGrid).toHaveBeenCalledWith({ branch_id: "b1", from: "2026-11-02", to: "2026-11-29" })
  );
  await waitFor(() => expect(api.saveScheduleCells).toHaveBeenCalledTimes(1));
  const sent = api.saveScheduleCells.mock.calls[0][0];
  expect(sent).toMatchObject({ branch_id: "b1", step: "preview" });
  expect(sent.cells).toHaveLength(56);
  expect(sent.cells).toContainEqual({ employee_id: "e2", date: "2026-11-08", code: "L" });

  // Konfirmasi selalu tampil untuk pola, walau tanpa peringatan.
  expect(await screen.findByText("2 karyawan · 56 baru · 0 berubah · 0 sama")).toBeTruthy();
  expect(api.saveScheduleCells).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Simpan jadwal" }));
  await waitFor(() =>
    expect(api.saveScheduleCells).toHaveBeenLastCalledWith(expect.objectContaining({ step: "commit", fingerprint: "fpp" }))
  );
  await waitFor(() => expect(onSaved).toHaveBeenCalled());
});

it("lebih dari 300 karyawan ditolak sebelum dikirim", () => {
  renderDialog({ employees: Array.from({ length: 301 }, (_, i) => ({ id: `e${i}`, name: `K${i}` })) });
  expect(screen.getByText("Maksimal 300 karyawan per simpan")).toBeTruthy();
  DAYS.forEach((d) => choose(d, "L"));
  expect((screen.getByRole("button", { name: "Terapkan" }) as HTMLButtonElement).disabled).toBe(true);
});
