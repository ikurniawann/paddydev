import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { PlacementSearchRow, StoreBranch } from "../types";
import { PlacementsPage } from "./placements-page";

const api = vi.hoisted(() => ({
  searchPlacements: vi.fn(),
  fetchPlacementIds: vi.fn(),
  fetchStoreBranches: vi.fn(),
  bulkPlacement: vi.fn(),
  bulkEndPlacement: vi.fn(),
}));
const push = vi.fn();
vi.mock("../api", () => api);
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() } }));
vi.mock("@/components/iam/iam-access-provider", () => ({ useIamAccess: () => ({ hasPrefix: () => true }) }));
vi.mock("@/features/master-data/departments/queries", () => ({
  useDepartmentList: () => ({ data: [{ id: "d1", name: "Operations", is_active: true }] }),
}));
vi.mock("@/features/master-data/positions/queries", () => ({
  usePositionList: () => ({ data: [{ id: "p1", title: "Kasir", is_active: true }] }),
}));
vi.mock("./branch-location-dialog", () => ({ BranchLocationDialog: () => null }));
vi.mock("./employee-placements-tab", () => ({ PLACEMENT_MANAGERS: [], EmployeePlacementsTab: () => null }));
// Combobox asli (popover + cmdk) diganti <select> supaya filter bisa dipilih di jsdom.
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

const placement = (id: string, employee: string, branch: StoreBranch, primary: boolean, start = "2026-01-05") => ({
  id,
  employee_id: employee,
  branch_id: branch.id,
  branch_code: branch.code,
  branch_name: branch.name,
  is_primary: primary,
  start_date: start,
  end_date: null,
  notes: null,
});

const row = (over: Partial<PlacementSearchRow>): PlacementSearchRow => ({
  employee_id: "e1",
  full_name: "Ani",
  nip: "1001",
  photo_url: null,
  department_name: "Operations",
  position_title: "Kasir",
  department_id: "d1",
  position_id: "p1",
  schedule_type: "shift",
  has_schedule: true,
  schedule_until: "2026-11-04",
  placements: [],
  ...over,
});

const rows: PlacementSearchRow[] = [
  row({ placements: [placement("pl1", "e1", branches[0], true), placement("pl2", "e1", branches[1], false)] }),
  row({ employee_id: "e2", full_name: "Budi", nip: "1002", has_schedule: false, schedule_until: null }),
  row({ employee_id: "e3", full_name: "Citra", nip: "1003", schedule_type: "non_shift", placements: [placement("pl3", "e3", branches[1], true)] }),
  row({
    employee_id: "e4",
    full_name: "Dodi",
    nip: "1004",
    has_schedule: false,
    schedule_until: null,
    placements: [placement("pl4", "e4", branches[1], true, "2026-03-02")],
  }),
];

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <PlacementsPage />
    </QueryClientProvider>
  );
  return client;
}

const lastFilter = () => api.searchPlacements.mock.calls.at(-1)?.[0];
let total = 60;

beforeEach(() => {
  total = 60;
  api.fetchStoreBranches.mockResolvedValue(branches);
  api.searchPlacements.mockImplementation(async (f: { page?: number; page_size?: number }) => ({
    rows,
    total,
    page: f.page ?? 1,
    page_size: f.page_size ?? 25,
    counts: { all: 60, unplaced: 7, no_schedule: 5, multi: 3 },
  }));
  api.fetchPlacementIds.mockResolvedValue({ ids: Array.from({ length: 30 }, (_, i) => `id${i}`), total: 30, truncated: false });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("PlacementsPage", () => {
  it("menampilkan toko utama, toko lain, tanggal mulai, dan status jadwal", async () => {
    renderPage();
    const ani = (await screen.findByText("Ani")).closest("tr")!;
    expect(within(ani).getByText("PVJ - Paddy PVJ")).toBeTruthy();
    expect(within(ani).getByText("+1 toko lain")).toBeTruthy();
    expect(within(ani).getByText("5 Jan 2026")).toBeTruthy();
    expect(within(ani).getByText("1001 · Kasir")).toBeTruthy();
    expect(within(ani).getByText("Shift · s/d 4 Nov")).toBeTruthy();
    expect(within(screen.getByText("Budi").closest("tr")!).getByText("Belum ditempatkan")).toBeTruthy();
    expect(within(screen.getByText("Citra").closest("tr")!).getByText("Non-Shift")).toBeTruthy();
    expect(within(screen.getByText("Dodi").closest("tr")!).getByText("Belum ada jadwal")).toBeTruthy();
    expect(screen.getByText("1–25 dari 60")).toBeTruthy();
  });

  it("chip Belum ditempatkan mengirim status=unplaced dan kembali ke halaman 1", async () => {
    renderPage();
    await screen.findByText("Ani");
    fireEvent.click(screen.getByRole("button", { name: "Halaman berikutnya" }));
    await waitFor(() => expect(lastFilter()).toMatchObject({ page: 2 }));
    fireEvent.click(screen.getByRole("button", { name: "Belum ditempatkan · 7" }));
    await waitFor(() => expect(lastFilter()).toMatchObject({ status: "unplaced", page: 1 }));
  });

  it("filter departemen dan jabatan ikut terkirim", async () => {
    renderPage();
    await screen.findByText("Ani");
    fireEvent.change(screen.getByLabelText("Semua departemen"), { target: { value: "d1" } });
    await waitFor(() => expect(lastFilter()).toMatchObject({ department_id: "d1", page: 1 }));
    fireEvent.change(screen.getByLabelText("Semua jabatan"), { target: { value: "p1" } });
    await waitFor(() => expect(lastFilter()).toMatchObject({ department_id: "d1", position_id: "p1" }));
  });

  it("pencarian dikirim setelah jeda ketik", async () => {
    renderPage();
    await screen.findByText("Ani");
    fireEvent.change(screen.getByPlaceholderText("Nama atau NIP"), { target: { value: "budi" } });
    expect(lastFilter()).not.toMatchObject({ q: "budi" });
    await waitFor(() => expect(lastFilter()).toMatchObject({ q: "budi", page: 1 }));
  });

  it("tombol halaman berikutnya dan jumlah per halaman", async () => {
    renderPage();
    await screen.findByText("Ani");
    fireEvent.click(screen.getByRole("button", { name: "Halaman berikutnya" }));
    await waitFor(() => expect(lastFilter()).toMatchObject({ page: 2, page_size: 25 }));
    expect(await screen.findByText("26–50 dari 60")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Baris per halaman"), { target: { value: "50" } });
    await waitFor(() => expect(lastFilter()).toMatchObject({ page: 1, page_size: 50 }));
  });

  it("mencentang 2 baris menampilkan bilah aksi dengan semua tombol", async () => {
    renderPage();
    await screen.findByText("Ani");
    expect(screen.queryByText(/karyawan dipilih/)).toBeNull();
    fireEvent.click(screen.getByLabelText("Pilih Ani"));
    fireEvent.click(screen.getByLabelText("Pilih Budi"));
    expect(screen.getByText("2 karyawan dipilih")).toBeTruthy();
    for (const name of ["Pindahkan ke toko", "Tambah toko", "Akhiri penempatan", "Atur jadwal"]) {
      expect(screen.getByRole("button", { name })).toBeTruthy();
    }
  });

  it("pilihan bertahan saat pindah halaman", async () => {
    renderPage();
    await screen.findByText("Ani");
    fireEvent.click(screen.getByLabelText("Pilih Ani"));
    fireEvent.click(screen.getByRole("button", { name: "Halaman berikutnya" }));
    await screen.findByText("26–50 dari 60");
    expect(screen.getByText("1 karyawan dipilih")).toBeTruthy();
  });

  it("Pilih semua hasil filter memanggil ids dengan filter aktif", async () => {
    renderPage();
    await screen.findByText("Ani");
    fireEvent.click(screen.getByRole("button", { name: "Multi-toko · 3" }));
    await waitFor(() => expect(lastFilter()).toMatchObject({ status: "multi" }));
    fireEvent.click(screen.getByLabelText("Pilih Ani"));
    fireEvent.click(screen.getByRole("button", { name: "Pilih semua hasil filter (60)" }));
    await waitFor(() => expect(screen.getByText("30 karyawan dipilih")).toBeTruthy());
    const filter = api.fetchPlacementIds.mock.calls[0][0];
    expect(filter).toMatchObject({ status: "multi" });
    expect(filter).not.toHaveProperty("page");
  });

  it("Atur jadwal membuka Jadwal Kerja pada toko utama terbanyak", async () => {
    renderPage();
    await screen.findByText("Ani");
    fireEvent.click(screen.getByLabelText("Pilih Ani"));
    fireEvent.click(screen.getByLabelText("Pilih Citra"));
    fireEvent.click(screen.getByLabelText("Pilih Dodi"));
    fireEvent.click(screen.getByRole("button", { name: "Atur jadwal" }));
    expect(push).toHaveBeenCalledWith("/dashboard/hris/schedules?branch_id=b2&employees=e1,e3,e4");
  });

  it("Atur jadwal sesudah Pilih semua hasil filter memakai toko utama dari baris yang termuat", async () => {
    api.fetchPlacementIds.mockResolvedValue({ ids: ["e1", "e2", "e3", "e4"], total: 4, truncated: false });
    renderPage();
    await screen.findByText("Ani");
    fireEvent.click(screen.getByLabelText("Pilih Ani"));
    fireEvent.click(screen.getByRole("button", { name: "Pilih semua hasil filter (60)" }));
    await screen.findByText("4 karyawan dipilih");
    fireEvent.click(screen.getByRole("button", { name: "Atur jadwal" }));
    expect(push).toHaveBeenCalledWith("/dashboard/hris/schedules?branch_id=b2&employees=e1,e2,e3,e4");
  });

  it("Atur jadwal tanpa toko yang diketahui dan tanpa filter toko tidak mengirim branch_id", async () => {
    api.fetchPlacementIds.mockResolvedValue({ ids: ["x1", "x2"], total: 2, truncated: false });
    renderPage();
    await screen.findByText("Ani");
    fireEvent.click(screen.getByLabelText("Pilih Budi"));
    fireEvent.click(screen.getByRole("button", { name: "Pilih semua hasil filter (60)" }));
    await screen.findByText("2 karyawan dipilih");
    fireEvent.click(screen.getByRole("button", { name: "Atur jadwal" }));
    expect(push).toHaveBeenCalledWith("/dashboard/hris/schedules?employees=x1,x2");
  });

  it("halaman mundur ke halaman terakhir bila hasil menyusut", async () => {
    const client = renderPage();
    await screen.findByText("Ani");
    fireEvent.click(screen.getByRole("button", { name: "Halaman berikutnya" }));
    await screen.findByText("26–50 dari 60");
    fireEvent.click(screen.getByRole("button", { name: "Halaman berikutnya" }));
    await screen.findByText("51–60 dari 60");
    total = 40;
    await act(() => client.invalidateQueries());
    await waitFor(() => expect(lastFilter()).toMatchObject({ page: 2 }));
    expect(await screen.findByText("26–40 dari 40")).toBeTruthy();
    expect(screen.queryByText(/51–40/)).toBeNull();
  });

  it("centang penempatan berakhir mengirim include_ended dan menampilkannya abu-abu", async () => {
    renderPage();
    await screen.findByText("Ani");
    api.searchPlacements.mockImplementation(async (f: { page?: number; page_size?: number; include_ended?: boolean }) => ({
      rows: f.include_ended
        ? [row({ placements: [placement("pl1", "e1", branches[0], true), { ...placement("pl9", "e1", branches[1], false, "2025-01-01"), end_date: "2025-12-31" }] })]
        : rows,
      total: 60,
      page: f.page ?? 1,
      page_size: f.page_size ?? 25,
      counts: { all: 60, unplaced: 7, no_schedule: 5, multi: 3 },
    }));
    fireEvent.click(screen.getByLabelText("Tampilkan penempatan yang sudah berakhir"));
    await waitFor(() => expect(lastFilter()).toMatchObject({ include_ended: true, page: 1 }));
    const ended = await screen.findByText("KPO - Paddy Kopo · berakhir 31 Des 2025");
    expect(ended.className).toContain("text-gray-400");
    expect(within(screen.getByText("Ani").closest("tr")!).queryByText("+1 toko lain")).toBeNull();
  });

  it("Pindahkan ke toko membuka dialog untuk karyawan terpilih", async () => {
    renderPage();
    await screen.findByText("Ani");
    fireEvent.click(screen.getByLabelText("Pilih Ani"));
    fireEvent.click(screen.getByLabelText("Pilih Budi"));
    fireEvent.click(screen.getByRole("button", { name: "Pindahkan ke toko" }));
    expect(await screen.findByText("Pindahkan 2 karyawan ke toko")).toBeTruthy();
  });
});
