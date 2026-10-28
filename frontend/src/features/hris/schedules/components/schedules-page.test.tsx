import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { addDays } from "../schedule-range";
import type { ScheduleGrid } from "../types";
import { SchedulesPage } from "./schedules-page";

const usersApi = vi.hoisted(() => ({ applyFixedSchedule: vi.fn() }));
vi.mock("@/features/users/api", () => usersApi);
const api = vi.hoisted(() => ({
  fetchScheduleGrid: vi.fn(),
  saveScheduleCells: vi.fn(),
  importScheduleFile: vi.fn(),
  downloadScheduleErrors: vi.fn(),
}));
vi.mock("../api", async (importOriginal) => ({ ...(await importOriginal<typeof import("../api")>()), ...api }));
// Rabu 4 Nov 2026: minggu tampil mulai Senin 2 Nov (sudah lewat dua hari).
vi.mock("@/lib/dates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/dates")>()),
  todayWib: () => "2026-11-04",
}));
vi.mock("@/features/hris/placements/queries", () => ({
  useStoreBranches: () => ({ data: [{ id: "b1", name: "PVJ - Paddy PVJ", code: "PVJ" }] }),
}));
vi.mock("@/components/ui/combobox", () => ({
  Combobox: ({ options, value, onChange, placeholder }: {
    options: { value: string; label: string }[];
    value?: string;
    onChange: (v: string) => void;
    placeholder?: string;
  }) => (
    <select aria-label={placeholder} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{placeholder}</option>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  ),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));
afterEach(cleanup);
beforeEach(() => {
  api.fetchScheduleGrid.mockReset();
  api.importScheduleFile.mockReset();
  api.fetchScheduleGrid.mockImplementation(async (p: { from: string; to: string }) => ({ data: grid(p.from, p.to) }));
});

function grid(from: string, to: string): ScheduleGrid {
  const dates: string[] = [];
  for (let d = from; d <= to; d = addDays(d, 1)) dates.push(d);
  return {
    branch: { id: "b1", name: "PVJ", code: "PVJ" },
    dates: dates.map((date) => ({ date, label: date.slice(5), holiday: null })),
    employees: [
      { id: "e1", nip: "1", name: "Ana", position: "", cells: dates.map((date) => ({ date, code: "", source: "none" as const })) },
      { id: "e2", nip: "2", name: "Budi", position: "", cells: dates.map((date) => ({ date, code: "", source: "none" as const })) },
    ],
    non_shift_count: 0,
    shifts: [{ code: "P", name: "Pagi", start_time: "08:00:00", end_time: "16:00:00" }],
  };
}

function renderPage(props: React.ComponentProps<typeof SchedulesPage>) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  render(
    <QueryClientProvider client={qc}>
      <SchedulesPage {...props} />
    </QueryClientProvider>
  );
  return { invalidate };
}

/** Kunci yang dimuat ulang setelah jadwal berubah: grid, Penempatan (status jadwal), profil, Shift Tim ESS. */
const SCHEDULE_DEPENDENTS = ['["hris","schedules"]', '["hris","placements"]', '["users","shift-schedule"]', '["hris","ess","team"]'];
const invalidatedKeys = (invalidate: ReturnType<typeof renderPage>["invalidate"]) =>
  invalidate.mock.calls.map(([f]) => JSON.stringify(f?.queryKey));

const checkbox = (name: string) => screen.getByLabelText(`Pilih ${name}`) as HTMLInputElement;

it("template dan unggah mulai hari ini bila minggu tampil sudah dimulai", async () => {
  api.importScheduleFile.mockResolvedValueOnce({
    data: { summary: { employees: 0, new: 0, changed: 0, same: 0 }, errors: [], warnings: [], changes: [] },
  });
  renderPage({ initialBranchId: "b1" });
  await screen.findByLabelText("Pilih Ana");
  expect(api.fetchScheduleGrid).toHaveBeenCalledWith({ branch_id: "b1", from: "2026-11-02", to: "2026-11-15" });

  const href = screen.getByRole("link", { name: "Unduh template" }).getAttribute("href") ?? "";
  const q = new URLSearchParams(href.split("?")[1]);
  expect([q.get("branch_id"), q.get("from"), q.get("to")]).toEqual(["b1", "2026-11-04", "2026-11-15"]);

  fireEvent.click(screen.getByRole("button", { name: "Unggah jadwal" }));
  expect(await screen.findByText(/Rentang jadwal: 4 Nov – 15 Nov 2026/)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("File jadwal (.xlsx)"), { target: { files: [new File(["x"], "j.xlsx")] } });
  fireEvent.click(screen.getByRole("button", { name: "Periksa file" }));
  await waitFor(() =>
    expect(api.importScheduleFile).toHaveBeenCalledWith(
      expect.objectContaining({ branch_id: "b1", from: "2026-11-04", to: "2026-11-15", step: "preview" })
    )
  );
});

it("centang tetap saat pindah minggu dan centang awal tidak dipasang ulang", async () => {
  renderPage({ initialBranchId: "b1", initialEmployeeIds: ["e2"] });
  await screen.findByLabelText("Pilih Ana");
  expect(checkbox("Budi").checked).toBe(true);
  fireEvent.click(checkbox("Budi"));
  fireEvent.click(checkbox("Ana"));

  fireEvent.click(screen.getByRole("button", { name: "Minggu berikutnya" }));
  await waitFor(() =>
    expect(api.fetchScheduleGrid).toHaveBeenCalledWith({ branch_id: "b1", from: "2026-11-09", to: "2026-11-22" })
  );
  expect(await screen.findByText("11-22")).toBeTruthy();
  expect(checkbox("Ana").checked).toBe(true);
  expect(checkbox("Budi").checked).toBe(false);
});

it("unggah tersimpan memuat ulang grid, Penempatan, profil, dan Shift Tim", async () => {
  api.importScheduleFile
    .mockResolvedValueOnce({
      data: { summary: { employees: 1, new: 7, changed: 0, same: 0 }, errors: [], warnings: [], changes: [], fingerprint: "fp1" },
    })
    .mockResolvedValueOnce({ data: { summary: { employees: 1, new: 7, changed: 0, same: 0 } }, message: "Jadwal 1 karyawan disimpan" });
  const { invalidate } = renderPage({ initialBranchId: "b1" });
  await screen.findByLabelText("Pilih Ana");
  fireEvent.click(screen.getByRole("button", { name: "Unggah jadwal" }));
  fireEvent.change(await screen.findByLabelText("File jadwal (.xlsx)"), { target: { files: [new File(["x"], "j.xlsx")] } });
  fireEvent.click(screen.getByRole("button", { name: "Periksa file" }));
  const save = (await screen.findByRole("button", { name: "Simpan jadwal" })) as HTMLButtonElement;
  await waitFor(() => expect(save.disabled).toBe(false));
  fireEvent.click(save);
  await waitFor(() => expect(api.importScheduleFile).toHaveBeenLastCalledWith(expect.objectContaining({ step: "commit" })));
  await waitFor(() => expect(invalidatedKeys(invalidate)).toEqual(expect.arrayContaining(SCHEDULE_DEPENDENTS)));
});

it("jadwal tetap diterapkan memuat ulang grid, Penempatan, profil, dan Shift Tim", async () => {
  api.fetchScheduleGrid.mockImplementation(async (p: { from: string; to: string }) => ({
    data: { ...grid(p.from, p.to), non_shift_count: 2 },
  }));
  usersApi.applyFixedSchedule.mockResolvedValueOnce({ message: "Jadwal tetap diterapkan" });
  const { invalidate } = renderPage({ initialBranchId: "b1" });
  fireEvent.click(await screen.findByRole("button", { name: "Terapkan jadwal tetap (Non-Shift)" }));
  fireEvent.click(await screen.findByRole("button", { name: "Terapkan" }));
  await waitFor(() => expect(usersApi.applyFixedSchedule).toHaveBeenCalled());
  await waitFor(() => expect(invalidatedKeys(invalidate)).toEqual(expect.arrayContaining(SCHEDULE_DEPENDENTS)));
});
