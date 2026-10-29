import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { addDays } from "@/features/hris/schedules/schedule-range";
import type { ScheduleGrid } from "@/features/hris/schedules/types";
import type { ScheduleAccess } from "@/features/hris/schedule-access/types";
import { EssJadwalTokoPage } from "./jadwal-toko-page";

const accessApi = vi.hoisted(() => ({ fetchScheduleAccess: vi.fn() }));
vi.mock("@/features/hris/schedule-access/api", () => accessApi);
const scheduleApi = vi.hoisted(() => ({ fetchScheduleGrid: vi.fn() }));
vi.mock("@/features/hris/schedules/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/hris/schedules/api")>()),
  ...scheduleApi,
}));
const placementApi = vi.hoisted(() => ({
  searchPlacements: vi.fn(),
  fetchPlacementIds: vi.fn(),
  fetchStoreBranches: vi.fn(),
  bulkPlacement: vi.fn(),
  bulkEndPlacement: vi.fn(),
}));
vi.mock("@/features/hris/placements/api", () => placementApi);
// Master departemen/jabatan khusus HR: halaman pengatur tidak boleh memintanya.
const masters = vi.hoisted(() => ({ departments: vi.fn(() => ({ data: [] })), positions: vi.fn(() => ({ data: [] })) }));
vi.mock("@/features/master-data/departments/queries", () => ({ useDepartmentList: masters.departments }));
vi.mock("@/features/master-data/positions/queries", () => ({ usePositionList: masters.positions }));
const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() } }));
vi.mock("@/components/iam/iam-access-provider", () => ({ useIamAccess: () => ({ hasPrefix: () => false }) }));
vi.mock("@/lib/dates", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/dates")>()),
  todayWib: () => "2026-11-04",
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

const STORE = { id: "s1", code: "KPO", name: "KPO - Paddy Kopo" };

function grid(from: string, to: string): ScheduleGrid {
  const dates: string[] = [];
  for (let d = from; d <= to; d = addDays(d, 1)) dates.push(d);
  return {
    branch: { id: STORE.id, name: STORE.name, code: STORE.code },
    dates: dates.map((date) => ({ date, label: date.slice(5), holiday: null })),
    employees: ["Ana", "Budi"].map((name, i) => ({
      id: `e${i + 1}`,
      nip: String(i + 1),
      name,
      position: "",
      cells: dates.map((date) => ({ date, code: "", source: "none" as const })),
    })),
    non_shift_count: 0,
    shifts: [{ code: "P", name: "Pagi", start_time: "08:00:00", end_time: "16:00:00" }],
  };
}

const placementRow = (id: string, name: string) => ({
  employee_id: id,
  full_name: name,
  nip: id,
  photo_url: null,
  department_name: null,
  position_title: null,
  department_id: null,
  position_id: null,
  schedule_type: "shift",
  has_schedule: false,
  schedule_until: null,
  placements: [
    {
      id: `pl-${id}`,
      employee_id: id,
      branch_id: STORE.id,
      branch_code: STORE.code,
      branch_name: STORE.name,
      is_primary: true,
      start_date: "2026-01-05",
      end_date: null,
      notes: null,
    },
  ],
});

function renderPage(access: ScheduleAccess) {
  accessApi.fetchScheduleAccess.mockResolvedValue(access);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <EssJadwalTokoPage />
    </QueryClientProvider>
  );
  return client;
}

beforeEach(() => {
  scheduleApi.fetchScheduleGrid.mockImplementation(async (p: { from: string; to: string }) => ({ data: grid(p.from, p.to) }));
  placementApi.searchPlacements.mockImplementation(async () => ({
    rows: [placementRow("e1", "Ana"), placementRow("e2", "Budi")],
    total: 2,
    page: 1,
    page_size: 25,
    counts: { all: 2, unplaced: 0, no_schedule: 2, multi: 0 },
  }));
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

it("bukan pengatur → kartu penjelasan", async () => {
  renderPage({ is_delegate: false, can_place: false, branches: [] });
  expect(await screen.findByText("Anda bukan pengatur jadwal toko. Hubungi HR bila seharusnya punya akses.")).toBeTruthy();
  expect(scheduleApi.fetchScheduleGrid).not.toHaveBeenCalled();
});

it("satu toko tanpa hak penempatan: hanya tab Jadwal, grid toko itu", async () => {
  renderPage({ is_delegate: true, can_place: false, branches: [STORE] });
  await screen.findByLabelText("Pilih Ana");
  expect(scheduleApi.fetchScheduleGrid).toHaveBeenCalledWith(expect.objectContaining({ branch_id: "s1" }));
  expect(screen.getByRole("tab", { name: "Jadwal" })).toBeTruthy();
  expect(screen.queryByRole("tab", { name: "Penempatan" })).toBeNull();
});

it("boleh atur penempatan: tab Penempatan tanpa filter departemen, Atur jadwal kembali ke tab Jadwal", async () => {
  renderPage({ is_delegate: true, can_place: true, branches: [STORE] });
  fireEvent.click(await screen.findByRole("tab", { name: "Penempatan" }));
  expect(await screen.findByText("Ana")).toBeTruthy();
  expect(screen.queryByLabelText("Semua departemen")).toBeNull();
  expect(masters.departments).not.toHaveBeenCalled();
  expect(masters.positions).not.toHaveBeenCalled();

  fireEvent.click(await screen.findByLabelText("Pilih Budi"));
  fireEvent.click(screen.getByRole("button", { name: "Atur jadwal" }));
  expect(push).not.toHaveBeenCalled();
  await waitFor(() => expect((screen.getByLabelText("Pilih Budi") as HTMLInputElement).checked).toBe(true));
  expect((screen.getByLabelText("Pilih Ana") as HTMLInputElement).checked).toBe(false);
  expect(screen.getByRole("tab", { name: "Jadwal" }).getAttribute("aria-selected")).toBe("true");
});

it("muat ulang akses yang gagal di latar tidak mengganti halaman yang sedang dipakai", async () => {
  const client = renderPage({ is_delegate: true, can_place: true, branches: [STORE] });
  await screen.findByLabelText("Pilih Ana");
  accessApi.fetchScheduleAccess.mockRejectedValue(new Error("Jaringan terputus"));
  await act(() => client.refetchQueries({ queryKey: ["hris", "schedule-access"] }));
  await waitFor(() => expect(client.getQueryState(["hris", "schedule-access"])?.status).toBe("error"));
  expect(screen.getByRole("tab", { name: "Jadwal" })).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Penempatan" })).toBeTruthy();
  expect(screen.getByLabelText("Pilih Ana")).toBeTruthy();
  expect(screen.queryByText("Jaringan terputus")).toBeNull();
});

it("gagal memuat akses tanpa data → kartu galat", async () => {
  accessApi.fetchScheduleAccess.mockRejectedValue(new Error("Jaringan terputus"));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <EssJadwalTokoPage />
    </QueryClientProvider>
  );
  expect(await screen.findByText("Jaringan terputus")).toBeTruthy();
});
