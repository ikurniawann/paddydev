import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ChartGap } from "./chart-gap";

const api = vi.hoisted(() => ({
  fetchOrgChartGap: vi.fn(),
  fetchOrgChartValidation: vi.fn(),
  orgChartGapXlsxUrl: (version: string) =>
    version ? `/api/hris/org-chart/gap?version=${encodeURIComponent(version)}&format=xlsx` : "/api/hris/org-chart/gap?format=xlsx",
}));
vi.mock("../api", () => api);
afterEach(cleanup);

beforeEach(() => {
  api.fetchOrgChartGap.mockReset().mockResolvedValue({
    version: { label: "Draft 6" },
    summary: { employees: 318, matched: 210, ambiguous: 40, outside: 68, manager_diff: 25, alias_review: 30, empty_nodes: 50, stores_outside: 8 },
    matches: [],
    empty_nodes: [],
    titles_outside: [{ title: "Warehouse Staff", employees: 12 }],
    stores_outside: ["BXST"],
    chart_stores_missing: [],
  });
  api.fetchOrgChartValidation.mockReset().mockResolvedValue({
    errors: [],
    warnings: [
      { rule: "BR-04", code: "HRG-004", message: "Level L7 sama dengan atasan HRG-003 (L7)" },
      { rule: "BR-04", code: "HRG-010", message: "Level L7 sama dengan atasan HRG-009 (L7)" },
      { rule: "BR-04", code: "MKT-004", message: "Level L5 sama dengan atasan MKT-003 (L5)" },
    ],
  });
});

it("menampilkan ringkasan selisih, tautan Excel, dan peringatan validasi", async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChartGap version="" />
    </QueryClientProvider>
  );
  expect(await screen.findByText("318")).toBeTruthy();
  expect(screen.getByText("210")).toBeTruthy();
  expect(screen.getByRole("link", { name: /Unduh Excel/ }).getAttribute("href")).toBe("/api/hris/org-chart/gap?format=xlsx");
  expect(await screen.findByText("Level L7 sama dengan atasan HRG-003 (L7)")).toBeTruthy();
  expect(screen.getAllByText(/HRG-010|MKT-004/).length).toBeGreaterThanOrEqual(2);
  expect(screen.getByText("Warehouse Staff")).toBeTruthy();
  expect(screen.getByText("BXST")).toBeTruthy();
});

it("gagal memuat laporan ditampilkan sebagai error", async () => {
  api.fetchOrgChartGap.mockReset().mockRejectedValue(new Error("boom"));
  api.fetchOrgChartValidation.mockReset().mockRejectedValue(new Error("boom"));
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ChartGap version="" />
    </QueryClientProvider>
  );
  const alerts = await screen.findAllByRole("alert");
  const text = alerts.map((a) => a.textContent).join(" ");
  expect(text).toContain("Laporan selisih tidak bisa dimuat");
  expect(text).toContain("Validasi bagan tidak bisa dimuat");
});

it("versi yang dipilih dipakai untuk laporan dan Excel", async () => {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ChartGap version="Draft 7" />
    </QueryClientProvider>
  );
  await screen.findByText("318");
  expect(api.fetchOrgChartGap).toHaveBeenCalledWith("Draft 7");
  expect(api.fetchOrgChartValidation).toHaveBeenCalledWith("Draft 7");
  expect(screen.getByRole("link", { name: /Unduh Excel/ }).getAttribute("href")).toContain("version=Draft%207");
});
