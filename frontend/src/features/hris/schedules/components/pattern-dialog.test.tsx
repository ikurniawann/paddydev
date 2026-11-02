import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { addDays } from "../schedule-range";
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
  { code: "P", name: "Pagi", start_time: "08:00:00", end_time: "16:00:00", break_minutes: 60 },
  { code: "S", name: "Siang", start_time: "13:00:00", end_time: "21:00:00" },
  { code: "A", name: "Awal", start_time: "08:00:00", end_time: "12:00:00", break_minutes: 0 },
  { code: "B", name: "Sore", start_time: "17:00:00", end_time: "21:00:00", break_minutes: 30 },
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

it("lebih dari 6000 kotak ditolak sebelum Terapkan", () => {
  renderDialog({ employees: Array.from({ length: 250 }, (_, i) => ({ id: `e${i}`, name: `K${i}` })) });
  DAYS.forEach((d) => choose(d, "L"));
  const apply = screen.getByRole("button", { name: "Terapkan" }) as HTMLButtonElement;
  expect(
    screen.getByText("250 karyawan × 28 hari = 7000 kotak, maksimal 6000 sekali simpan. Perkecil rentang tanggal atau jumlah karyawan.")
  ).toBeTruthy();
  expect(apply.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("Sampai tanggal"), { target: { value: "2026-11-25" } }); // 24 hari = 6000
  expect(screen.queryByText(/kotak, maksimal 6000/)).toBeNull();
  expect(apply.disabled).toBe(false);
  expect(api.fetchScheduleGrid).not.toHaveBeenCalled();
});

it("7 hari terisi shift: peringatan belum ada libur", () => {
  renderDialog();
  DAYS.forEach((d) => choose(d, "P"));
  expect(screen.getByText("Belum ada hari libur — sistem shift wajib libur 1 hari setiap minggu")).toBeTruthy();
  expect(screen.getByText("Total ±49 jam kerja/minggu")).toBeTruthy();
  expect((screen.getByRole("button", { name: "Terapkan" }) as HTMLButtonElement).disabled).toBe(false); // tidak memblokir
});

it("6 hari kerja + libur: tanpa peringatan, total 42 jam bersih", () => {
  renderDialog();
  DAYS.forEach((d, i) => choose(d, i < 6 ? "P" : "L"));
  expect(screen.queryByText(/Belum ada hari libur/)).toBeNull();
  expect(screen.getByText("Total ±42 jam kerja/minggu")).toBeTruthy();
});

it("total belum tampil sebelum semua hari dipilih", () => {
  renderDialog();
  choose("Senin", "P");
  expect(screen.queryByText(/jam kerja\/minggu/)).toBeNull();
});

it("total disembunyikan bila ada hari dikosongkan", () => {
  renderDialog();
  DAYS.forEach((d, i) => choose(d, i < 6 ? "P" : "__clear"));
  expect(screen.queryByText(/jam kerja\/minggu/)).toBeNull();
});

it("shift kedua opsional per hari: kode KODE1+KODE2 dikirim ke server", async () => {
  api.fetchScheduleGrid.mockResolvedValueOnce(rangeGrid("2026-11-02", "2026-11-29", ["e1", "e2"]));
  api.saveScheduleCells.mockResolvedValueOnce({
    data: { summary: { employees: 2, new: 56, changed: 0, same: 0, cleared: 0 }, errors: [], warnings: [], fingerprint: "fp2", changes: [], replaced_days: 0 },
  });
  renderDialog();
  DAYS.forEach((d, i) => choose(d, i === 0 ? "A" : "L"));
  // Libur tidak bisa diberi shift kedua; Senin (A 08–12) hanya menawarkan shift yang mulai ≥ 12:00.
  expect((screen.getByLabelText("Selasa shift kedua") as HTMLSelectElement).disabled).toBe(true);
  const options = Array.from((screen.getByLabelText("Senin shift kedua") as HTMLSelectElement).options).map((o) => o.value);
  expect(options).toEqual(["", "S", "B"]);
  choose("Senin shift kedua", "B");
  fireEvent.click(screen.getByRole("button", { name: "Terapkan" }));
  await waitFor(() => expect(api.saveScheduleCells).toHaveBeenCalledTimes(1));
  const sent = api.saveScheduleCells.mock.calls[0][0];
  expect(sent.cells).toContainEqual({ employee_id: "e1", date: "2026-11-02", code: "A+B" });
  expect(sent.cells).toContainEqual({ employee_id: "e1", date: "2026-11-09", code: "A+B" });
  expect(sent.cells).toContainEqual({ employee_id: "e1", date: "2026-11-03", code: "L" });
});

it("total jam per minggu menjumlahkan jam bersih kedua shift", () => {
  renderDialog();
  // Senin A(4 jam) + B(4 jam − 30 menit istirahat = 3,5 jam) = 7,5 jam; Selasa P = 7 jam; sisanya libur.
  DAYS.forEach((d, i) => choose(d, i === 0 ? "A" : i === 1 ? "P" : "L"));
  choose("Senin shift kedua", "B");
  expect(screen.getByText("Total ±15 jam kerja/minggu")).toBeTruthy();
});

it("mengganti hari menjadi libur membuang shift kedua", () => {
  renderDialog();
  choose("Senin", "A");
  choose("Senin shift kedua", "B");
  choose("Senin", "L");
  expect((screen.getByLabelText("Senin shift kedua") as HTMLSelectElement).value).toBe("");
});
