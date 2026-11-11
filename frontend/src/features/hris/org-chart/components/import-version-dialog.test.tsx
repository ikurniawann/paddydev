import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ChartImportPreview } from "../types";
import { ImportVersionDialog } from "./import-version-dialog";

const api = vi.hoisted(() => ({ importOrgChartFile: vi.fn(), downloadOrgChartImportErrors: vi.fn() }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast }));
afterEach(cleanup);

const preview = (over: Partial<ChartImportPreview> = {}): { data: ChartImportPreview } => ({
  data: {
    label: "Draft 7",
    base: { label: "Draft 6" },
    summary: { nodes: 150, positions: 149, errors: 0, warnings: 1 },
    errors: [],
    warnings: [{ line: 152, column: "Toko", name: "RTL-900", message: "Toko PPMR tidak ada sebagai toko aktif di HRIS" }],
    diff: { added: ["RTL-900"], removed: ["RTL-056"], changed: [{ code: "RTL-054", title: "Store Leader Senior", fields: ["Jabatan"] }] },
    vacancy: { positions: 149, filled: 128, empty: 10, maybe: 0, planned: 9, frozen: 2 },
    fingerprint: "fp",
    ...over,
  },
});

beforeEach(() => {
  api.importOrgChartFile.mockReset().mockResolvedValue(preview());
  api.downloadOrgChartImportErrors.mockReset().mockResolvedValue(undefined);
  toast.success.mockReset();
  toast.error.mockReset();
});

function setup() {
  const onImported = vi.fn();
  const onOpenChange = vi.fn();
  const client = new QueryClient();
  const invalidate = vi.spyOn(client, "invalidateQueries");
  render(
    <QueryClientProvider client={client}>
      <ImportVersionDialog open onOpenChange={onOpenChange} base="Draft 6" onImported={onImported} />
    </QueryClientProvider>
  );
  return { onImported, onOpenChange, invalidate };
}

const file = new File(["x"], "bagan.xlsx");

function fill(label = "Draft 7") {
  fireEvent.change(screen.getByLabelText("Label versi"), { target: { value: label } });
  fireEvent.change(screen.getByLabelText("File template (.xlsx)"), { target: { files: [file] } });
}

it("Pratinjau butuh label dan file", () => {
  setup();
  const btn = screen.getByRole("button", { name: "Pratinjau" }) as HTMLButtonElement;
  expect(btn.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("Label versi"), { target: { value: "Draft 7" } });
  expect(btn.disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("File template (.xlsx)"), { target: { files: [file] } });
  expect(btn.disabled).toBe(false);
});

it("pratinjau menampilkan ringkasan, selisih, peringatan; simpan setelah dicentang", async () => {
  const { onImported, invalidate } = setup();
  fill();
  fireEvent.click(screen.getByRole("button", { name: "Pratinjau" }));
  expect(await screen.findByText("150 node · 149 posisi")).toBeTruthy();
  expect(api.importOrgChartFile).toHaveBeenCalledWith({ file, label: "Draft 7", base: "Draft 6", step: "preview" });
  expect(screen.getByText("Ditambah 1")).toBeTruthy();
  expect(screen.getByText(/RTL-900/, { selector: "li" })).toBeTruthy();
  expect(screen.getByText("Dihapus 1")).toBeTruthy();
  expect(screen.getByText("Diubah 1")).toBeTruthy();
  expect(screen.getByText(/RTL-054 Store Leader Senior \(Jabatan\)/)).toBeTruthy();
  expect(screen.getByText("Baris 152 · Toko: Toko PPMR tidak ada sebagai toko aktif di HRIS")).toBeTruthy();
  expect(screen.getByText(/Terisi 128/)).toBeTruthy();
  expect(screen.getByText(/Dibekukan 2/)).toBeTruthy();
  const save = screen.getByRole("button", { name: "Simpan sebagai draft" }) as HTMLButtonElement;
  expect(save.disabled).toBe(true);
  fireEvent.click(screen.getByLabelText("Saya sudah memeriksa peringatan"));
  expect(save.disabled).toBe(false);
  api.importOrgChartFile.mockResolvedValueOnce({ data: { version: { label: "Draft 7" } }, message: "Versi Draft 7 disimpan sebagai draft (150 node)" });
  fireEvent.click(save);
  await waitFor(() => expect(onImported).toHaveBeenCalledWith("Draft 7"));
  expect(api.importOrgChartFile).toHaveBeenLastCalledWith({
    file,
    label: "Draft 7",
    base: "Draft 6",
    step: "commit",
    fingerprint: "fp",
    ack_warnings: true,
  });
  expect(toast.success).toHaveBeenCalledWith("Versi Draft 7 disimpan sebagai draft (150 node)");
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["hris", "org-chart"] });
});

it("error memblokir simpan dan bisa diunduh sebagai file", async () => {
  api.importOrgChartFile.mockReset().mockResolvedValue(
    preview({
      summary: { nodes: 151, positions: 150, errors: 1, warnings: 0 },
      warnings: [],
      errors: [{ line: 0, column: "Kode Atasan", name: "", message: "Bagan harus punya tepat satu puncak (ditemukan 2: EXE-001, FIN-900)" }],
    })
  );
  setup();
  fill();
  fireEvent.click(screen.getByRole("button", { name: "Pratinjau" }));
  expect(await screen.findByText("Bagan · Kode Atasan: Bagan harus punya tepat satu puncak (ditemukan 2: EXE-001, FIN-900)")).toBeTruthy();
  expect((screen.getByRole("button", { name: "Simpan sebagai draft" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Unduh file error" }));
  await waitFor(() => expect(api.downloadOrgChartImportErrors).toHaveBeenCalledWith({ file, label: "Draft 7", base: "Draft 6" }));
});

it("mengubah label setelah pratinjau menghapus pratinjau", async () => {
  setup();
  fill();
  fireEvent.click(screen.getByRole("button", { name: "Pratinjau" }));
  await screen.findByText("150 node · 149 posisi");
  fireEvent.change(screen.getByLabelText("Label versi"), { target: { value: "Draft 8" } });
  expect(screen.queryByText("150 node · 149 posisi")).toBeNull();
});

it("pesan server saat pratinjau gagal ditampilkan sebagai toast", async () => {
  api.importOrgChartFile.mockReset().mockRejectedValue(new Error("Label versi sudah dipakai"));
  setup();
  fill("Draft 6");
  fireEvent.click(screen.getByRole("button", { name: "Pratinjau" }));
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Label versi sudah dipakai"));
});
