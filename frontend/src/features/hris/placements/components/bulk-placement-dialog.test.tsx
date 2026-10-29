import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BulkPlacementResponse, StoreBranch } from "../types";
import { BulkPlacementDialog } from "./bulk-placement-dialog";

const api = vi.hoisted(() => ({
  fetchStoreBranches: vi.fn(),
  bulkPlacement: vi.fn(),
  bulkEndPlacement: vi.fn(),
}));
const toast = vi.hoisted(() => ({ error: vi.fn(), success: vi.fn() }));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast }));
vi.mock("@/lib/dates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/dates")>()),
  todayWib: () => "2026-11-04",
}));
vi.mock("@/components/ui/combobox", () => ({
  Combobox: ({
    options,
    value,
    onChange,
    placeholder,
  }: {
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

const branches: StoreBranch[] = [
  { id: "b1", code: "PVJ", name: "PVJ - Paddy PVJ", company_id: "c1", company_name: "Paddy", lat: null, lng: null, geofence_radius_m: null },
  { id: "b2", code: "KPO", name: "KPO - Paddy Kopo", company_id: "c1", company_name: "Paddy", lat: null, lng: null, geofence_radius_m: null },
];

const moveResult = (step: "preview" | "commit"): BulkPlacementResponse => ({
  data: {
    step,
    created: 0,
    moved: 1,
    skipped: 1,
    results: [
      {
        employee_id: "e1",
        name: "Ani",
        nip: "1001",
        outcome: "moved",
        reason: null,
        ended: [{ branch_name: "KPO - Paddy Kopo", end_date: "2026-10-31" }],
      },
      { employee_id: "e2", name: "Budi", nip: "1002", outcome: "skipped", reason: "Sudah di toko ini", ended: [] },
    ],
  },
  message: step === "commit" ? "1 dipindahkan, 1 dilewati" : undefined,
});

let client: QueryClient;
function renderDialog(props: Partial<React.ComponentProps<typeof BulkPlacementDialog>> = {}) {
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onDone = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <BulkPlacementDialog open mode="move" employeeIds={["e1", "e2"]} onOpenChange={() => {}} onDone={onDone} {...props} />
    </QueryClientProvider>
  );
  return { onDone };
}

beforeEach(() => {
  api.fetchStoreBranches.mockResolvedValue(branches);
  api.bulkPlacement.mockImplementation(async (p: { step: "preview" | "commit" }) => moveResult(p.step));
  api.bulkEndPlacement.mockImplementation(async (p: { step: "preview" | "commit" }) => ({
    data: {
      step: p.step,
      ended: 1,
      skipped: 0,
      results: [
        { employee_id: "e1", name: "Ani", nip: "1001", outcome: "ended", reason: null, ended: [{ branch_name: "PVJ - Paddy PVJ", end_date: "2026-11-30" }] },
      ],
    },
    message: p.step === "commit" ? "1 diakhiri" : undefined,
  }));
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

async function chooseStore(label: string, name: string) {
  const select = await screen.findByLabelText(label);
  await waitFor(() => expect(within(select).getByText(name)).toBeTruthy());
  fireEvent.change(select, { target: { value: branches.find((b) => b.name === name)!.id } });
}

describe("BulkPlacementDialog", () => {
  it("Pindahkan: pratinjau dulu, lalu simpan dengan step commit", async () => {
    const { onDone } = renderDialog();
    expect(screen.getByText("Pindahkan 2 karyawan ke toko")).toBeTruthy();
    await chooseStore("Pilih toko", "PVJ - Paddy PVJ");
    fireEvent.change(screen.getByLabelText("Tanggal pindah"), { target: { value: "2026-11-01" } });
    fireEvent.click(screen.getByRole("button", { name: "Lihat pratinjau" }));

    await waitFor(() => expect(api.bulkPlacement).toHaveBeenCalledTimes(1));
    expect(api.bulkPlacement.mock.calls[0][0]).toMatchObject({
      mode: "move",
      step: "preview",
      branch_id: "b1",
      employee_ids: ["e1", "e2"],
      start_date: "2026-11-01",
    });

    const ani = (await screen.findByText("Ani")).closest("tr")!;
    expect(within(ani).getByText("Dipindah")).toBeTruthy();
    expect(within(ani).getByText("KPO - Paddy Kopo diakhiri 31 Okt 2026")).toBeTruthy();
    const budi = screen.getByText("Budi").closest("tr")!;
    expect(within(budi).getByText("Dilewati")).toBeTruthy();
    expect(within(budi).getByText("Sudah di toko ini")).toBeTruthy();
    expect(screen.getByText("1 dipindah · 1 dilewati")).toBeTruthy();

    const invalidate = vi.spyOn(client, "invalidateQueries");
    expect(invalidate).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));

    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
    expect(api.bulkPlacement.mock.calls[1][0]).toMatchObject({ mode: "move", step: "commit", branch_id: "b1" });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["hris", "placements"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["hris", "schedules"] });
    expect(toast.success).toHaveBeenCalledWith("1 dipindahkan, 1 dilewati");
  });

  it("pratinjau wajib memilih toko", async () => {
    renderDialog();
    await screen.findByLabelText("Pilih toko");
    fireEvent.click(screen.getByRole("button", { name: "Lihat pratinjau" }));
    expect(toast.error).toHaveBeenCalledWith("Pilih toko tujuan dulu");
    expect(api.bulkPlacement).not.toHaveBeenCalled();
  });

  it("Tambah toko mengirim mode add dan pilihan toko utama", async () => {
    renderDialog({ mode: "add" });
    expect(screen.getByText("Tambah toko untuk 2 karyawan")).toBeTruthy();
    await chooseStore("Pilih toko", "KPO - Paddy Kopo");
    fireEvent.click(screen.getByLabelText(/Jadikan toko utama/));
    fireEvent.click(screen.getByRole("button", { name: "Lihat pratinjau" }));
    await waitFor(() => expect(api.bulkPlacement).toHaveBeenCalledTimes(1));
    expect(api.bulkPlacement.mock.calls[0][0]).toMatchObject({ mode: "add", step: "preview", branch_id: "b2", is_primary: true });
  });

  it("Akhiri memakai bulk-end dengan tanggal akhir", async () => {
    const { onDone } = renderDialog({ mode: "end", employeeIds: ["e1"] });
    expect(screen.getByText("Akhiri penempatan 1 karyawan")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Tanggal terakhir bekerja"), { target: { value: "2026-11-30" } });
    fireEvent.click(screen.getByRole("button", { name: "Lihat pratinjau" }));
    await waitFor(() => expect(api.bulkEndPlacement).toHaveBeenCalledTimes(1));
    expect(api.bulkEndPlacement.mock.calls[0][0]).toEqual({
      employee_ids: ["e1"],
      end_date: "2026-11-30",
      branch_id: null,
      step: "preview",
    });
    expect(await screen.findByText("Diakhiri")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
    expect(api.bulkEndPlacement.mock.calls[1][0]).toMatchObject({ step: "commit" });
  });

  it("mengubah isian setelah pratinjau meminta pratinjau ulang", async () => {
    renderDialog();
    await chooseStore("Pilih toko", "PVJ - Paddy PVJ");
    fireEvent.click(screen.getByRole("button", { name: "Lihat pratinjau" }));
    await screen.findByText("Ani");
    fireEvent.click(screen.getByRole("button", { name: "Ubah isian" }));
    expect(screen.queryByRole("button", { name: "Simpan" })).toBeNull();
    expect(screen.getByRole("button", { name: "Lihat pratinjau" })).toBeTruthy();
  });

  it.each([
    ["move", "Tanggal pindah"],
    ["add", "Tanggal mulai"],
    ["end", "Tanggal terakhir bekerja"],
  ] as const)("%s: tanggal sebelum hari ini diberi peringatan tanpa menghalangi simpan", async (mode, label) => {
    const warning = "Tanggal sebelum hari ini — riwayat penempatan dan pembagian biaya gaji ikut berubah.";
    const { onDone } = renderDialog({ mode });
    expect(screen.queryByText(warning)).toBeNull(); // bawaan = hari ini
    if (mode !== "end") await chooseStore("Pilih toko", "PVJ - Paddy PVJ");
    fireEvent.change(screen.getByLabelText(label), { target: { value: "2026-11-03" } });
    expect(screen.getByText(warning)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Lihat pratinjau" }));
    await screen.findByText("Ani");
    expect(screen.getByText(warning)).toBeTruthy(); // tetap terlihat saat memutuskan simpan
    fireEvent.click(screen.getByRole("button", { name: "Simpan" }));
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
  });

  it("tanggal hari ini atau nanti tanpa peringatan", () => {
    renderDialog({ mode: "move" });
    fireEvent.change(screen.getByLabelText("Tanggal pindah"), { target: { value: "2026-11-05" } });
    expect(screen.queryByText(/Tanggal sebelum hari ini/)).toBeNull();
  });

  it.each([
    ["move", "Tanggal pindah", "2026-11-04"],
    ["add", "Tanggal mulai", "2026-11-04"],
    ["end", "Tanggal terakhir bekerja", "2026-11-03"],
  ] as const)("mode toko %s: tanggal paling awal %s, tanpa peringatan tanggal lampau", (mode, label, min) => {
    renderDialog({ mode, branches: [{ id: "b1", name: "PVJ - Paddy PVJ" }] });
    const input = screen.getByLabelText(label) as HTMLInputElement;
    expect(input.min).toBe(min);
    fireEvent.change(input, { target: { value: "2026-11-03" } });
    expect(screen.queryByText(/Tanggal sebelum hari ini/)).toBeNull();
  });

  it("mode toko: tanggal sebelum batas ditolak sebelum dikirim", async () => {
    renderDialog({ mode: "end", branches: [{ id: "b1", name: "PVJ - Paddy PVJ" }] });
    fireEvent.change(screen.getByLabelText("Tanggal terakhir bekerja"), { target: { value: "2026-11-02" } });
    fireEvent.click(screen.getByRole("button", { name: "Lihat pratinjau" }));
    expect(toast.error).toHaveBeenCalledWith("Tanggal akhir paling awal kemarin");
    expect(api.bulkEndPlacement).not.toHaveBeenCalled();
  });

  it("mode HR: tanpa batas tanggal", () => {
    renderDialog({ mode: "move" });
    expect((screen.getByLabelText("Tanggal pindah") as HTMLInputElement).min).toBe("");
  });

  it("lebih dari 500 karyawan tidak bisa diproses", async () => {
    renderDialog({ employeeIds: Array.from({ length: 501 }, (_, i) => `e${i}`) });
    expect(screen.getByText("Maksimal 500 karyawan per aksi")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Lihat pratinjau" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
