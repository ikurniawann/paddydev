import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ChartVacancy } from "./chart-vacancy";

const api = vi.hoisted(() => ({
  fetchOrgChartVacancy: vi.fn(),
  orgChartVacancyXlsxUrl: (version: string, f: { division: string; region: string; state: string }) =>
    `/api/hris/org-chart/vacancy?${new URLSearchParams({ ...(version ? { version } : {}), ...(f.division ? { division: f.division } : {}), ...(f.region ? { region: f.region } : {}), ...(f.state ? { state: f.state } : {}), format: "xlsx" })}`,
}));
vi.mock("../api", () => api);
afterEach(cleanup);

const response = {
  version: { id: "v1", label: "Draft 6", status: "draft", valid_from: null, approved_at: null, approved_by_name: null },
  summary: { positions: 149, filled: 20, empty: 120, maybe: 0, planned: 9, frozen: 0 },
  groups: [
    { division: "RTL", division_name: "Retail Operational", region: "Bandung", positions: 10, filled: 2, empty: 5, maybe: 2, planned: 1, frozen: 0 },
    { division: "FIN", division_name: "Finance", region: "", positions: 4, filled: 1, empty: 3, maybe: 0, planned: 0, frozen: 0 },
  ],
  rows: [
    { code: "RTL-010", title: "Shopkeeper", division_code: "RTL", division_name: "Retail Operational", region: "Bandung", store_codes: ["PVJ1"], parent_code: "RTL-009", parent_title: "Store Leader", state: "maybe", holders: 0, maybe_holders: ["Sinta"] },
    { code: "FIN-002", title: "Finance Staff", division_code: "FIN", division_name: "Finance", region: "", store_codes: [], parent_code: "FIN-001", parent_title: "Finance Manager", state: "empty", holders: 0, maybe_holders: [] },
    { code: "RTL-020", title: "Patcher", division_code: "RTL", division_name: "Retail Operational", region: "Bandung", store_codes: [], parent_code: "", parent_title: "", state: "planned", holders: 0, maybe_holders: [] },
  ],
};

beforeEach(() => {
  api.fetchOrgChartVacancy.mockReset().mockResolvedValue(response);
});

function setup(version = "") {
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ChartVacancy version={version} />
    </QueryClientProvider>
  );
}

it("menampilkan ringkasan, kelompok, dan posisi Mungkin terisi beserta nama", async () => {
  setup();
  const tiles = await screen.findByLabelText("Ringkasan posisi");
  expect(within(tiles).getByText("Kosong").parentElement?.textContent).toContain("120");
  expect(within(tiles).getByText("Posisi").parentElement?.textContent).toContain("149");
  expect(within(tiles).getByText("Mungkin terisi").parentElement?.textContent).toContain("0");
  const rows = screen.getByRole("table", { name: "Daftar posisi belum terisi" });
  const maybe = within(rows).getByText("RTL-010").closest("tr") as HTMLElement;
  expect(maybe.textContent).toContain("Shopkeeper");
  expect(maybe.textContent).toContain("Mungkin terisi");
  expect(maybe.textContent).toContain("Sinta");
  expect((within(rows).getByText("FIN-002").closest("tr") as HTMLElement).textContent).toContain("Kosong");
  expect((within(rows).getByText("RTL-020").closest("tr") as HTMLElement).textContent).toContain("Rencana");
  const groups = screen.getByRole("table", { name: "Ringkasan per divisi dan region" });
  expect(within(groups).getByText("Bandung")).toBeTruthy();
  expect(within(groups).getByText("Tanpa region")).toBeTruthy();
});

it("tautan Excel mengikuti versi dan filter", async () => {
  setup("Draft 6");
  await screen.findByLabelText("Ringkasan posisi");
  const link = screen.getByRole("link", { name: /Unduh Excel/ });
  expect(link.getAttribute("href")).toContain("/api/hris/org-chart/vacancy");
  expect(link.getAttribute("href")).toContain("format=xlsx");
  expect(link.getAttribute("href")).toContain("version=Draft+6");
  fireEvent.change(screen.getByLabelText("Keadaan"), { target: { value: "maybe" } });
  await waitFor(() =>
    expect(api.fetchOrgChartVacancy).toHaveBeenLastCalledWith("Draft 6", { division: "", region: "", state: "maybe" })
  );
  expect(screen.getByRole("link", { name: /Unduh Excel/ }).getAttribute("href")).toContain("state=maybe");
});

it("filter divisi dan region dikirim ke API", async () => {
  setup();
  await screen.findByLabelText("Ringkasan posisi");
  expect(api.fetchOrgChartVacancy).toHaveBeenCalledWith("", { division: "", region: "", state: "" });
  fireEvent.change(screen.getByLabelText("Divisi"), { target: { value: "RTL" } });
  await waitFor(() =>
    expect(api.fetchOrgChartVacancy).toHaveBeenLastCalledWith("", { division: "RTL", region: "", state: "" })
  );
  fireEvent.change(screen.getByLabelText("Region"), { target: { value: "Bandung" } });
  await waitFor(() =>
    expect(api.fetchOrgChartVacancy).toHaveBeenLastCalledWith("", { division: "RTL", region: "Bandung", state: "" })
  );
});

it("semua posisi terisi ditampilkan sebagai pesan, bukan tabel kosong", async () => {
  api.fetchOrgChartVacancy.mockReset().mockResolvedValue({ ...response, rows: [] });
  setup();
  expect(await screen.findByText("Semua posisi untuk filter ini sudah terisi")).toBeTruthy();
});

it("gagal memuat ditampilkan sebagai error", async () => {
  api.fetchOrgChartVacancy.mockReset().mockRejectedValue(new Error("Insufficient permissions"));
  setup();
  expect((await screen.findByRole("alert")).textContent).toContain("Posisi kosong tidak bisa dimuat");
});
