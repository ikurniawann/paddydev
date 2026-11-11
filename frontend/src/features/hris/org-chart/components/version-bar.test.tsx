import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ChartVersion } from "../types";
import { VersionBar } from "./version-bar";

const api = vi.hoisted(() => ({
  fetchOrgChartVersions: vi.fn(),
  fetchOrgChartValidation: vi.fn(),
  approveOrgChartVersion: vi.fn(),
  deleteOrgChartVersion: vi.fn(),
  importOrgChartFile: vi.fn(),
  downloadOrgChartImportErrors: vi.fn(),
  orgChartTemplateUrl: (v: string) => `/api/hris/org-chart/template?${new URLSearchParams(v ? { version: v } : {})}`,
}));
vi.mock("../api", () => api);
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({ toast }));
afterEach(cleanup);

const draft: ChartVersion = { id: "v1", label: "Draft 6", status: "draft", valid_from: null, approved_at: null, approved_by_name: null, nodes: 150 };
const approved: ChartVersion = {
  id: "v0",
  label: "Draft 5",
  status: "approved",
  valid_from: "2026-11-12",
  approved_at: "2026-11-11T03:00:00Z",
  approved_by_name: "Rina HR",
  nodes: 120,
};
const archived: ChartVersion = { ...approved, id: "v9", label: "Draft 4", status: "archived" };

beforeEach(() => {
  api.fetchOrgChartVersions.mockReset().mockResolvedValue([draft, approved, archived]);
  api.fetchOrgChartValidation.mockReset().mockResolvedValue({ errors: [], warnings: [] });
  api.approveOrgChartVersion.mockReset().mockResolvedValue({ ...draft, status: "approved" });
  api.deleteOrgChartVersion.mockReset().mockResolvedValue({ id: "v1", label: "Draft 6" });
  toast.success.mockReset();
  toast.error.mockReset();
});

function setup(version = "", onChange = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(client, "invalidateQueries");
  render(
    <QueryClientProvider client={client}>
      <VersionBar version={version} onChange={onChange} />
    </QueryClientProvider>
  );
  return { onChange, invalidate };
}

it("bawaan = versi yang disetujui: badge Disetujui, tanggal berlaku, tanpa tombol Setujui", async () => {
  setup();
  expect(await screen.findByText(/berlaku sejak 2026-11-12/)).toBeTruthy();
  expect(screen.getByText(/Disetujui Rina HR/)).toBeTruthy();
  expect((screen.getByLabelText("Versi bagan") as HTMLSelectElement).value).toBe("Draft 5");
  expect(screen.queryByRole("button", { name: "Setujui versi" })).toBeNull();
});

it("versi draft menampilkan tombol Setujui versi; memilih versi memanggil onChange", async () => {
  const { onChange } = setup("Draft 6");
  expect(await screen.findByRole("button", { name: "Setujui versi" })).toBeTruthy();
  expect(screen.getByText("Draft")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Versi bagan"), { target: { value: "Draft 4" } });
  expect(onChange).toHaveBeenCalledWith("Draft 4");
});

it("tanpa versi disetujui, bawaan = impor terbaru (pertama di daftar)", async () => {
  api.fetchOrgChartVersions.mockReset().mockResolvedValue([draft, archived]);
  setup();
  expect(await screen.findByRole("button", { name: "Setujui versi" })).toBeTruthy();
  expect((screen.getByLabelText("Versi bagan") as HTMLSelectElement).value).toBe("Draft 6");
});

it("versi arsip tidak bisa disetujui", async () => {
  setup("Draft 4");
  expect(await screen.findByText("Arsip")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Setujui versi" })).toBeNull();
});

it("error struktur menonaktifkan Setujui", async () => {
  api.fetchOrgChartValidation.mockReset().mockResolvedValue({
    errors: [{ rule: "BR-01", code: "", message: "Bagan harus punya tepat satu puncak" }],
    warnings: [],
  });
  setup("Draft 6");
  fireEvent.click(await screen.findByRole("button", { name: "Setujui versi" }));
  expect(await screen.findByText("Perbaiki 1 kesalahan struktur dulu")).toBeTruthy();
  expect((screen.getByRole("button", { name: "Setujui" }) as HTMLButtonElement).disabled).toBe(true);
  expect(api.fetchOrgChartValidation).toHaveBeenCalledWith("Draft 6");
});

it("tanpa error, Setujui mengirim tanggal berlaku dan alasan lalu menyegarkan data bagan", async () => {
  const { invalidate } = setup("Draft 6");
  fireEvent.click(await screen.findByRole("button", { name: "Setujui versi" }));
  const date = (await screen.findByLabelText("Tanggal berlaku")) as HTMLInputElement;
  expect(date.value).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  fireEvent.change(date, { target: { value: "2026-12-01" } });
  fireEvent.change(screen.getByLabelText("Alasan (opsional)"), { target: { value: "Rapat HR" } });
  const submit = screen.getByRole("button", { name: "Setujui" }) as HTMLButtonElement;
  await waitFor(() => expect(submit.disabled).toBe(false));
  fireEvent.click(submit);
  await waitFor(() =>
    expect(api.approveOrgChartVersion).toHaveBeenCalledWith("v1", { valid_from: "2026-12-01", reason: "Rapat HR" })
  );
  await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Versi Draft 6 disetujui"));
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["hris", "org-chart"] });
});

it("tanggal kosong tidak bisa dikirim", async () => {
  setup("Draft 6");
  fireEvent.click(await screen.findByRole("button", { name: "Setujui versi" }));
  fireEvent.change(await screen.findByLabelText("Tanggal berlaku"), { target: { value: "" } });
  expect((screen.getByRole("button", { name: "Setujui" }) as HTMLButtonElement).disabled).toBe(true);
});

it("pesan error server ditampilkan sebagai toast", async () => {
  api.approveOrgChartVersion.mockReset().mockRejectedValue(new Error("Versi ini sudah tidak berstatus draft"));
  setup("Draft 6");
  fireEvent.click(await screen.findByRole("button", { name: "Setujui versi" }));
  const submit = (await screen.findByRole("button", { name: "Setujui" })) as HTMLButtonElement;
  await waitFor(() => expect(submit.disabled).toBe(false));
  fireEvent.click(submit);
  await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Versi ini sudah tidak berstatus draft"));
});

it("tanpa hak approve, pesan 403 server tampil dalam bahasa Indonesia", async () => {
  api.approveOrgChartVersion.mockReset().mockRejectedValue(new Error("Insufficient permissions"));
  setup("Draft 6");
  fireEvent.click(await screen.findByRole("button", { name: "Setujui versi" }));
  const submit = (await screen.findByRole("button", { name: "Setujui" })) as HTMLButtonElement;
  await waitFor(() => expect(submit.disabled).toBe(false));
  fireEvent.click(submit);
  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith("Anda tidak punya hak menyetujui versi bagan. Minta admin menambahkan aksi Approve pada menu Struktur Org.")
  );
});

it("unduh template mengikuti versi yang dilihat; tombol Impor versi tersedia", async () => {
  setup("Draft 6");
  const link = await screen.findByRole("link", { name: /Unduh template/ });
  expect(link.getAttribute("href")).toContain("/api/hris/org-chart/template");
  expect(link.getAttribute("href")).toContain("version=Draft+6");
  expect(screen.getByRole("button", { name: "Impor versi" })).toBeTruthy();
});

it("hapus draft: konfirmasi, panggil API, kembali ke versi bawaan", async () => {
  const { onChange, invalidate } = setup("Draft 6");
  fireEvent.click(await screen.findByRole("button", { name: "Hapus draft" }));
  expect(await screen.findByText("Hapus Draft 6?")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Hapus" }));
  await waitFor(() => expect(api.deleteOrgChartVersion).toHaveBeenCalledWith("v1"));
  await waitFor(() => expect(onChange).toHaveBeenCalledWith(""));
  expect(toast.success).toHaveBeenCalledWith("Versi Draft 6 dihapus");
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["hris", "org-chart"] });
});

it("versi disetujui tidak punya tombol Hapus draft", async () => {
  setup();
  await screen.findByText(/berlaku sejak 2026-11-12/);
  expect(screen.queryByRole("button", { name: "Hapus draft" })).toBeNull();
});
