import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { ApiError } from "@/lib/api-client";
import { addDays } from "../schedule-range";
import type { CellsPreview, ScheduleGrid as Grid } from "../types";
import { ScheduleGrid } from "./schedule-grid";

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
  vi.mocked(toast.success).mockReset();
  vi.mocked(toast.error).mockReset();
  vi.mocked(toast.info).mockReset();
});

const shifts = [
  { code: "P", name: "Pagi", start_time: "08:00:00", end_time: "16:00:00", break_minutes: 60 },
  { code: "S", name: "Siang", start_time: "13:00:00", end_time: "21:00:00", break_minutes: 60 },
];

function makeGrid(from: string, rows: { id: string; name: string; codes?: Record<string, string> }[]): Grid {
  const dates = Array.from({ length: 7 }, (_, i) => addDays(from, i));
  return {
    branch: { id: "b1", name: "PVJ", code: "PVJ" },
    dates: dates.map((date) => ({ date, label: date.slice(8), holiday: null })),
    employees: rows.map((r) => ({
      id: r.id,
      nip: `N${r.id}`,
      name: r.name,
      position: "Kasir",
      cells: dates.map((date) => ({ date, code: r.codes?.[date] ?? "", source: r.codes?.[date] ? "day" : "none" })),
    })),
    non_shift_count: 0,
    shifts,
  };
}

const grid = makeGrid("2026-11-02", [
  { id: "e1", name: "Ana", codes: { "2026-11-02": "P", "2026-11-04": "P" } },
  { id: "e2", name: "Budi", codes: { "2026-11-05": "-" } },
]);

const preview = (over: Partial<CellsPreview> = {}): { data: CellsPreview } => ({
  data: {
    summary: { employees: 1, new: 1, changed: 0, same: 0, cleared: 0 },
    errors: [],
    warnings: [],
    fingerprint: "fp1",
    changes: [],
    replaced_days: 0,
    ...over,
  },
});

function renderGrid(props: Partial<React.ComponentProps<typeof ScheduleGrid>> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(qc, "invalidateQueries");
  render(
    <QueryClientProvider client={qc}>
      <ScheduleGrid grid={grid} branchId="b1" today="2026-11-03" {...props} />
    </QueryClientProvider>
  );
  return { invalidate };
}

const previewCall = () => api.saveScheduleCells.mock.calls.find(([p]) => p.step === "preview")?.[0];

describe("ScheduleGrid", () => {
  it("tanggal lewat dan kotak tidak ditempatkan tidak bisa diklik", () => {
    renderGrid();
    expect(screen.queryByRole("button", { name: "Ana 02/11" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Budi 05/11" })).toBeNull();
    expect(screen.getByRole("button", { name: "Ana 03/11" })).toBeTruthy();
  });

  it("klik kotak membuka pemilih shift; pilih kode → preview 1 sel, tanpa masalah langsung simpan", async () => {
    api.saveScheduleCells
      .mockResolvedValueOnce(preview())
      .mockResolvedValueOnce({ data: preview().data, message: "Jadwal 1 karyawan disimpan" });
    const { invalidate } = renderGrid();
    fireEvent.click(screen.getByRole("button", { name: "Ana 03/11" }));
    fireEvent.click(await screen.findByRole("button", { name: /^S · Siang/ }));
    await waitFor(() => expect(api.saveScheduleCells).toHaveBeenCalledTimes(2));
    expect(previewCall()).toEqual({
      branch_id: "b1",
      step: "preview",
      cells: [{ employee_id: "e1", date: "2026-11-03", code: "S" }],
    });
    expect(api.saveScheduleCells.mock.calls[1][0]).toMatchObject({
      step: "commit",
      fingerprint: "fp1",
      ack_warnings: false,
      cells: [{ employee_id: "e1", date: "2026-11-03", code: "S" }],
    });
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Jadwal 1 karyawan disimpan"));
    const keys = invalidate.mock.calls.map(([f]) => JSON.stringify(f?.queryKey));
    expect(keys).toEqual(
      expect.arrayContaining(['["hris","schedules"]', '["hris","placements"]', '["users","shift-schedule"]', '["hris","ess","team"]'])
    );
  });

  it("pratinjau tanpa perubahan → tidak disimpan, toast Tidak ada perubahan", async () => {
    api.saveScheduleCells.mockResolvedValueOnce(
      preview({ summary: { employees: 1, new: 0, changed: 0, same: 1, cleared: 0 } })
    );
    renderGrid();
    fireEvent.click(screen.getByRole("button", { name: "Ana 03/11" }));
    fireEvent.click(await screen.findByRole("button", { name: /^S · Siang/ }));
    await waitFor(() => expect(toast.info).toHaveBeenCalledWith("Tidak ada perubahan"));
    expect(api.saveScheduleCells).toHaveBeenCalledTimes(1);
    expect(previewCall()).toBeTruthy();
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("dengan peringatan → dialog konfirmasi; simpan setelah peringatan dicentang", async () => {
    api.saveScheduleCells
      .mockResolvedValueOnce(
        preview({ warnings: [{ line: 0, column: "Rab 04/11", name: "Ana", message: "Ana sedang cuti pada 04/11" }] })
      )
      .mockResolvedValueOnce({ data: preview().data, message: "Jadwal 1 karyawan disimpan" });
    renderGrid();
    fireEvent.click(screen.getByRole("button", { name: "Ana 06/11" }));
    fireEvent.click(await screen.findByRole("button", { name: /^P · Pagi/ }));
    expect(await screen.findByText("Ana sedang cuti pada 04/11")).toBeTruthy();
    const save = screen.getByRole("button", { name: "Simpan jadwal" }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.click(screen.getByLabelText("Saya sudah memeriksa peringatan"));
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() =>
      expect(api.saveScheduleCells).toHaveBeenLastCalledWith(expect.objectContaining({ step: "commit", ack_warnings: true }))
    );
  });

  it("menimpa jadwal (replaced_days > 0) → alasan wajib", async () => {
    api.saveScheduleCells
      .mockResolvedValueOnce(
        preview({
          replaced_days: 1,
          summary: { employees: 1, new: 0, changed: 1, same: 0, cleared: 0 },
          changes: [{ nip: "Ne1", name: "Ana", date: "2026-11-04", before: "P", after: "L" }],
        })
      )
      .mockResolvedValueOnce({ data: preview().data, message: "Jadwal 1 karyawan disimpan" });
    renderGrid();
    fireEvent.click(screen.getByRole("button", { name: "Ana 04/11" }));
    fireEvent.click(await screen.findByRole("button", { name: /^L · Libur/ }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("1 jadwal yang sudah ada akan diganti atau dihapus")).toBeTruthy();
    expect(within(dialog).getByText("Ana · 04/11 · P → L")).toBeTruthy();
    const save = screen.getByRole("button", { name: "Simpan jadwal" }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Alasan mengganti jadwal"), { target: { value: "Tukar shift" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    await waitFor(() =>
      expect(api.saveScheduleCells).toHaveBeenLastCalledWith(expect.objectContaining({ step: "commit", reason: "Tukar shift" }))
    );
  });

  it("error tanggal (nama kosong) tampil sebagai daftar umum dan simpan dikunci", async () => {
    api.saveScheduleCells.mockResolvedValueOnce(
      preview({ errors: [{ line: 0, column: "Sel 03/11", name: "", message: "Periode sudah ditutup" }] })
    );
    renderGrid();
    fireEvent.click(screen.getByRole("button", { name: "Ana 03/11" }));
    fireEvent.click(await screen.findByRole("button", { name: /^P · Pagi/ }));
    expect(await screen.findByText("Sel 03/11: Periode sudah ditutup")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Simpan jadwal" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("shift-klik memilih rentang dalam satu baris, Ctrl-klik menambah; Isi dan Kosongkan mengirim sel yang benar", async () => {
    api.saveScheduleCells.mockResolvedValue(preview());
    renderGrid();
    fireEvent.click(screen.getByRole("button", { name: "Ana 04/11" }), { ctrlKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Ana 06/11" }), { shiftKey: true });
    expect(screen.getByText("3 kotak dipilih")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Budi 07/11" }), { metaKey: true });
    expect(screen.getByText("4 kotak dipilih")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Isi kotak terpilih" }));
    fireEvent.click(await screen.findByRole("button", { name: /^P · Pagi/ }));
    await waitFor(() => expect(previewCall()).toBeTruthy());
    expect(previewCall().cells).toEqual([
      { employee_id: "e1", date: "2026-11-04", code: "P" },
      { employee_id: "e1", date: "2026-11-05", code: "P" },
      { employee_id: "e1", date: "2026-11-06", code: "P" },
      { employee_id: "e2", date: "2026-11-07", code: "P" },
    ]);
    // Setelah tersimpan pilihan dikosongkan.
    await waitFor(() => expect(screen.queryByText("4 kotak dipilih")).toBeNull());

    api.saveScheduleCells.mockClear();
    fireEvent.click(screen.getByRole("button", { name: "Budi 03/11" }), { ctrlKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Budi 04/11" }), { ctrlKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Kosongkan" }));
    await waitFor(() => expect(previewCall()).toBeTruthy());
    expect(previewCall().cells).toEqual([
      { employee_id: "e2", date: "2026-11-03", code: "" },
      { employee_id: "e2", date: "2026-11-04", code: "" },
    ]);
  });

  it("jadwal diubah orang lain (409) → pesan + muat ulang grid", async () => {
    api.saveScheduleCells
      .mockResolvedValueOnce(preview())
      .mockRejectedValueOnce(new ApiError("Jadwal sudah diubah orang lain sejak preview — muat ulang jadwal", 409, {}));
    const { invalidate } = renderGrid();
    fireEvent.click(screen.getByRole("button", { name: "Ana 03/11" }));
    fireEvent.click(await screen.findByRole("button", { name: /^P · Pagi/ }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Jadwal sudah diubah orang lain sejak preview — muat ulang jadwal")
    );
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["hris", "schedules"] });
  });

  it("centang awal dari tautan Penempatan; Salin minggu lalu memuat 7 hari sebelumnya", async () => {
    api.fetchScheduleGrid.mockResolvedValueOnce({
      data: makeGrid("2026-10-26", [{ id: "e1", name: "Ana", codes: { "2026-10-29": "S" } }]),
    });
    api.saveScheduleCells.mockResolvedValueOnce(preview());
    renderGrid({ initialChecked: ["e2", "x9"] });
    expect((screen.getByLabelText("Pilih Budi") as HTMLInputElement).checked).toBe(true);
    expect((screen.getByLabelText("Pilih Ana") as HTMLInputElement).checked).toBe(false);
    expect(screen.getByText("1 karyawan dicentang")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Salin minggu lalu" }));
    await waitFor(() =>
      expect(api.fetchScheduleGrid).toHaveBeenCalledWith({ branch_id: "b1", from: "2026-10-26", to: "2026-11-01" })
    );
    await waitFor(() => expect(previewCall()).toBeTruthy());
    expect(previewCall().cells).toEqual([{ employee_id: "e1", date: "2026-11-05", code: "S" }]);
    // Salin minggu lalu selalu dikonfirmasi dulu.
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("button", { name: "Simpan jadwal" })).toBeTruthy();
  });
  describe("sentuhan (HP)", () => {
    const original = window.matchMedia;
    afterEach(() => {
      window.matchMedia = original;
    });
    const stubMedia = (matches: boolean) => {
      window.matchMedia = ((query: string) => ({
        matches,
        media: query,
        addEventListener: () => {},
        removeEventListener: () => {},
      })) as unknown as typeof window.matchMedia;
    };

    it("Pilih beberapa kotak: ketuk dua kotak memilihnya tanpa membuka pemilih; matikan → pilihan hilang", async () => {
      renderGrid();
      const toggle = screen.getByRole("button", { name: "Pilih beberapa kotak" });
      expect(toggle.getAttribute("aria-pressed")).toBe("false");
      fireEvent.click(toggle);
      expect(toggle.getAttribute("aria-pressed")).toBe("true");

      fireEvent.click(screen.getByRole("button", { name: "Ana 03/11" }));
      fireEvent.click(screen.getByRole("button", { name: "Ana 04/11" }));
      expect(screen.getByText("2 kotak dipilih")).toBeTruthy();
      // Pemilih dirender lewat portal: biarkan efek tertunda selesai sebelum menegaskan ketiadaannya.
      await act(async () => {});
      expect(screen.queryByRole("button", { name: /^S · Siang/ })).toBeNull();
      expect(screen.queryByRole("dialog")).toBeNull();

      // Ketuk lagi pada kotak terpilih membatalkannya.
      fireEvent.click(screen.getByRole("button", { name: "Ana 04/11" }));
      expect(screen.getByText("1 kotak dipilih")).toBeTruthy();
      fireEvent.click(screen.getByRole("button", { name: "Ana 04/11" }));
      expect(screen.getByText("2 kotak dipilih")).toBeTruthy();

      fireEvent.click(toggle);
      expect(toggle.getAttribute("aria-pressed")).toBe("false");
      expect(screen.queryByText("2 kotak dipilih")).toBeNull();
      expect(screen.getByRole("button", { name: "Ana 03/11" }).getAttribute("aria-pressed")).toBe("false");
    });

    it("mode pilih banyak: Isi kotak terpilih mengirim kedua sel", async () => {
      api.saveScheduleCells.mockResolvedValue(preview());
      renderGrid();
      fireEvent.click(screen.getByRole("button", { name: "Pilih beberapa kotak" }));
      fireEvent.click(screen.getByRole("button", { name: "Ana 03/11" }));
      fireEvent.click(screen.getByRole("button", { name: "Budi 03/11" }));
      fireEvent.click(screen.getByRole("button", { name: "Isi kotak terpilih" }));
      fireEvent.click(await screen.findByRole("button", { name: /^S · Siang/ }));
      await waitFor(() => expect(previewCall()).toBeTruthy());
      expect(previewCall().cells).toEqual([
        { employee_id: "e1", date: "2026-11-03", code: "S" },
        { employee_id: "e2", date: "2026-11-03", code: "S" },
      ]);
    });

    it("layar sempit: legenda tertutup sampai tombol Kode shift (N) diketuk", async () => {
      stubMedia(false);
      renderGrid();
      const toggle = await screen.findByRole("button", { name: /^Kode shift \(2\)/ });
      await waitFor(() => expect(toggle.getAttribute("aria-expanded")).toBe("false"));
      expect(screen.queryByText(/Pagi 08:00–16:00/)).toBeNull();
      fireEvent.click(toggle);
      expect(toggle.getAttribute("aria-expanded")).toBe("true");
      expect(screen.getByText(/Pagi 08:00–16:00/)).toBeTruthy();
      fireEvent.click(toggle);
      expect(screen.queryByText(/Pagi 08:00–16:00/)).toBeNull();
    });

    it("Safari lama (hanya addListener/removeListener): grid tetap tampil dan legenda mengikuti matches", async () => {
      const added: unknown[] = [];
      const removed: unknown[] = [];
      window.matchMedia = ((query: string) => ({
        matches: false,
        media: query,
        addListener: (fn: unknown) => added.push(fn),
        removeListener: (fn: unknown) => removed.push(fn),
      })) as unknown as typeof window.matchMedia;
      renderGrid();
      expect(screen.getByRole("button", { name: "Ana 03/11" })).toBeTruthy();
      const toggle = screen.getByRole("button", { name: /^Kode shift \(2\)/ });
      await waitFor(() => expect(toggle.getAttribute("aria-expanded")).toBe("false"));
      expect(screen.queryByText(/Pagi 08:00–16:00/)).toBeNull();
      expect(added.length).toBeGreaterThan(0);
      cleanup();
      expect(removed.length).toBe(added.length);
    });

    it("layar lebar: legenda terbuka", async () => {
      stubMedia(true);
      renderGrid();
      const toggle = screen.getByRole("button", { name: /^Kode shift \(2\)/ });
      await waitFor(() => expect(toggle.getAttribute("aria-expanded")).toBe("true"));
      expect(screen.getByText(/Pagi 08:00–16:00/)).toBeTruthy();
    });

    it("petunjuk: teks Ctrl/Shift hanya di layar lebar, petunjuk ketuk hanya di HP", () => {
      renderGrid();
      const desktop = screen.getByText(/Tahan Ctrl \(Mac: Cmd\)/);
      expect(desktop.className).toContain("hidden");
      expect(desktop.className).toContain("sm:inline");
      const touch = screen.getByText("Ketuk Pilih beberapa kotak untuk mengisi banyak hari sekaligus.");
      expect(touch.className).toContain("sm:hidden");
    });

    it("kolom nama: min-w-32 di HP, sm:min-w-56 di layar lebar; nama lengkap di title", () => {
      renderGrid();
      const th = screen.getByText("Karyawan").closest("th")!;
      expect(th.className).toContain("min-w-32");
      expect(th.className).toContain("sm:min-w-56");
      const td = screen.getByText("Ana", { selector: "span.font-medium" }).closest("td")!;
      expect(td.className).toContain("min-w-32");
      expect(td.className).toContain("sm:min-w-56");
      expect(screen.getByText("Ana", { selector: "span.font-medium" }).getAttribute("title")).toBe("Ana");
    });
  });
});
