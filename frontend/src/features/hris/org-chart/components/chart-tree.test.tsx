import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ChartNode } from "../types";
import { ChartTree } from "./chart-tree";

const api = vi.hoisted(() => ({ fetchOrgChartTree: vi.fn() }));
vi.mock("../api", () => api);
afterEach(cleanup);

const node = (over: Partial<ChartNode>): ChartNode => ({
  code: "X",
  parent_code: null,
  title: "X",
  raw_title: "X",
  is_position: true,
  job_level: "L7",
  level_confidence: "keyword",
  employment_type: "PERMANENT_OR_CONTRACT",
  vendor: null,
  division_code: "EXE",
  division_name: "Executive",
  region: null,
  store_codes: [],
  status: "active",
  needs_confirm: false,
  flags: [],
  holders: [],
  ...over,
});

const nodes = [
  node({ code: "EXE-001", title: "CEO", job_level: "L0", holders: [{ id: "e1", name: "Bu Direktur", nip: "1" }] }),
  node({ code: "EXE-002", parent_code: "EXE-001", title: "General Manager", job_level: "L1" }),
  node({ code: "RTL-001", parent_code: "EXE-002", title: "Retail Operational Manager", job_level: "L2", division_code: "RTL", division_name: "Retail Operational" }),
  node({ code: "RTL-053", parent_code: "RTL-001", title: "Store Supervisor", job_level: "L4", division_code: "RTL", division_name: "Retail Operational", region: "Makassar" }),
  node({
    code: "RTL-054",
    parent_code: "RTL-053",
    title: "Store Leader",
    job_level: "L5",
    division_code: "RTL",
    division_name: "Retail Operational",
    region: "Makassar",
    store_codes: ["PPMR", "PLMR"],
    status: "planned",
    needs_confirm: true,
  }),
  node({ code: "SUP-001", parent_code: "EXE-001", title: "Support", is_position: false, job_level: null, division_code: "SUP", division_name: "Support" }),
];

beforeEach(() => {
  api.fetchOrgChartTree.mockReset().mockResolvedValue({ version: { label: "Draft 6" }, nodes });
});

function setup() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChartTree version="" />
    </QueryClientProvider>
  );
}

it("menampilkan pohon bagan dengan badge status, perlu konfirmasi, toko, dan pemegang", async () => {
  setup();
  expect(await screen.findByText("CEO")).toBeTruthy();
  expect(screen.getByRole("list", { name: "Bagan organisasi" })).toBeTruthy();
  expect(screen.queryByRole("tree")).toBeNull();
  expect(document.querySelector("[aria-selected]")).toBeNull();
  expect(screen.getByText("Draft 6 · 5 posisi")).toBeTruthy();
  expect(screen.getByText("Bu Direktur")).toBeTruthy();
  expect(screen.getByText("PPMR · PLMR")).toBeTruthy();
  const leader = screen.getByText("Store Leader").closest("li") as HTMLElement;
  expect(leader.textContent).toContain("Rencana");
  expect(leader.textContent).toContain("Perlu konfirmasi");
  const legend = screen.getByLabelText("Legenda bagan");
  for (const label of ["Aktif", "Rencana", "Perlu konfirmasi"]) expect(legend.textContent).toContain(label);
});

it("node yang punya bawahan bisa dilipat dan dibuka", async () => {
  setup();
  await screen.findByText("CEO");
  const toggle = screen.getByRole("button", { name: "Lipat CEO" });
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  expect(screen.queryByText("General Manager")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Buka CEO" }));
  expect(screen.getByText("General Manager")).toBeTruthy();
});

it("filter divisi memanggil API dengan division", async () => {
  setup();
  await screen.findByText("CEO");
  fireEvent.change(screen.getByLabelText("Divisi"), { target: { value: "RTL" } });
  await waitFor(() => expect(api.fetchOrgChartTree).toHaveBeenLastCalledWith({ division: "RTL", region: "" }, ""));
});

it("versi yang dipilih dikirim ke API", async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChartTree version="Draft 7" />
    </QueryClientProvider>
  );
  await screen.findByText("CEO");
  expect(api.fetchOrgChartTree).toHaveBeenCalledWith({ division: "", region: "" }, "Draft 7");
});

it("gagal memuat ditampilkan sebagai error, bukan bagan kosong", async () => {
  api.fetchOrgChartTree.mockReset().mockRejectedValue(new Error("Insufficient permissions"));
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ChartTree version="" />
    </QueryClientProvider>
  );
  expect((await screen.findByRole("alert")).textContent).toContain("Bagan tidak bisa dimuat");
  expect(screen.queryByText("Tidak ada posisi untuk filter ini")).toBeNull();
});
