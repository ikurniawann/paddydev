import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { addDays } from "@/features/hris/schedules/schedule-range";
import type { ScheduleGrid } from "@/features/hris/schedules/types";
import type { ScheduleAccess } from "@/features/hris/schedule-access/types";
import { ApiError } from "@/lib/api-client";
import { toast } from "sonner";
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

/** `fresh` = staleTime seperti aplikasi (2 menit): akses tidak dimuat ulang hanya karena tab dipasang. */
function renderPage(access: ScheduleAccess, fresh = false) {
  accessApi.fetchScheduleAccess.mockResolvedValue(access);
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, ...(fresh ? { staleTime: 120_000 } : {}) } } });
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

const NOT_DELEGATE = "Anda bukan pengatur jadwal toko. Hubungi HR bila seharusnya punya akses.";
const DELEGATE: ScheduleAccess = { is_delegate: true, can_place: true, branches: [STORE] };
const revoked = () => new ApiError("Insufficient permissions", 403, { error: "Insufficient permissions" });

it("penunjukan dicabut saat halaman terbuka: grid 403 → kartu bukan pengatur, tanpa teks server", async () => {
  scheduleApi.fetchScheduleGrid.mockRejectedValue(revoked());
  renderPage(DELEGATE, true);
  accessApi.fetchScheduleAccess.mockResolvedValue({ is_delegate: false, can_place: false, branches: [] });
  expect(await screen.findByText(NOT_DELEGATE)).toBeTruthy();
  expect(screen.queryByRole("tab", { name: "Jadwal" })).toBeNull();
  expect(screen.queryByText("Insufficient permissions")).toBeNull();
  expect(accessApi.fetchScheduleAccess).toHaveBeenCalledTimes(2);
  expect(scheduleApi.fetchScheduleGrid).toHaveBeenCalledTimes(1);
});

it("akses gagal dimuat ulang sesudah 403 (mis. karyawan dinonaktifkan) → tetap kartu", async () => {
  scheduleApi.fetchScheduleGrid.mockRejectedValue(revoked());
  renderPage(DELEGATE, true);
  accessApi.fetchScheduleAccess.mockRejectedValue(new Error("Jaringan terputus"));
  expect(await screen.findByText(NOT_DELEGATE)).toBeTruthy();
  expect(screen.queryByText("Insufficient permissions")).toBeNull();
});

it("403 wilayah tetap memakai pesan server, bukan kartu", async () => {
  scheduleApi.fetchScheduleGrid.mockRejectedValue(new ApiError("Toko di luar wilayah Anda", 403, null));
  renderPage(DELEGATE, true);
  expect(await screen.findByText("Toko di luar wilayah Anda")).toBeTruthy();
  expect(screen.getByRole("tab", { name: "Jadwal" })).toBeTruthy();
  expect(screen.queryByText(NOT_DELEGATE)).toBeNull();
});

it("tab Penempatan: pencarian 403 → kartu bukan pengatur", async () => {
  renderPage(DELEGATE, true);
  await screen.findByLabelText("Pilih Ana");
  placementApi.searchPlacements.mockRejectedValue(revoked());
  accessApi.fetchScheduleAccess.mockResolvedValue({ is_delegate: false, can_place: false, branches: [] });
  fireEvent.click(screen.getByRole("tab", { name: "Penempatan" }));
  expect(await screen.findByText(NOT_DELEGATE)).toBeTruthy();
  expect(screen.queryByText("Gagal memuat penempatan karyawan")).toBeNull();
});

it("aksi yang ditolak 403 → kartu bukan pengatur, toast tanpa teks server", async () => {
  placementApi.searchPlacements.mockImplementation(async () => ({
    rows: [placementRow("e1", "Ana"), placementRow("e2", "Budi")],
    total: 3,
    page: 1,
    page_size: 25,
    counts: { all: 3, unplaced: 0, no_schedule: 3, multi: 0 },
  }));
  placementApi.fetchPlacementIds.mockRejectedValue(revoked());
  renderPage(DELEGATE, true);
  fireEvent.click(await screen.findByRole("tab", { name: "Penempatan" }));
  fireEvent.click(await screen.findByLabelText("Pilih Ana"));
  accessApi.fetchScheduleAccess.mockResolvedValue({ is_delegate: false, can_place: false, branches: [] });
  fireEvent.click(screen.getByRole("button", { name: /Pilih semua hasil filter/ }));
  expect(await screen.findByText(NOT_DELEGATE)).toBeTruthy();
  expect(toast.error).not.toHaveBeenCalledWith("Insufficient permissions");
});

it("mode toko punya Shift kosong + hak penempatan: arahkan ke tab Penempatan", async () => {
  scheduleApi.fetchScheduleGrid.mockImplementation(async (p: { from: string; to: string }) => ({
    data: { ...grid(p.from, p.to), employees: [] },
  }));
  renderPage(DELEGATE, true);
  expect(await screen.findByText(/Belum ada karyawan Shift yang ditempatkan di toko ini\./)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Tempatkan karyawan di tab Penempatan" }));
  expect(screen.getByRole("tab", { name: "Penempatan" }).getAttribute("aria-selected")).toBe("true");
});

it("mode toko tanpa hak penempatan: Shift kosong tanpa arahan ke Penempatan", async () => {
  scheduleApi.fetchScheduleGrid.mockImplementation(async (p: { from: string; to: string }) => ({
    data: { ...grid(p.from, p.to), employees: [] },
  }));
  renderPage({ ...DELEGATE, can_place: false });
  expect(await screen.findByText(/Belum ada karyawan Shift yang ditempatkan di toko ini\./)).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Tempatkan karyawan di tab Penempatan" })).toBeNull();
});

it("hak penempatan dicabut saat tab Penempatan terbuka → kembali ke tab Jadwal", async () => {
  const client = renderPage(DELEGATE, true);
  fireEvent.click(await screen.findByRole("tab", { name: "Penempatan" }));
  expect(await screen.findByText("Ana")).toBeTruthy();
  accessApi.fetchScheduleAccess.mockResolvedValue({ ...DELEGATE, can_place: false });
  await act(() => client.invalidateQueries({ queryKey: ["hris", "schedule-access"] }));
  await waitFor(() => expect(screen.queryByRole("tab", { name: "Penempatan" })).toBeNull());
  expect(screen.getByRole("tab", { name: "Jadwal" }).getAttribute("aria-selected")).toBe("true");
  expect(await screen.findByLabelText("Pilih Ana")).toBeTruthy();
});

it("HR Jadwal Kerja (bukan pengatur): mode HR semua toko dengan tab Penempatan (EPIC-073 R2)", async () => {
  placementApi.fetchStoreBranches.mockResolvedValue([STORE, { id: "s2", code: "PVJ", name: "PVJ - Paddy PVJ" }]);
  renderPage({ is_delegate: false, can_place: false, branches: [], hr: true, hr_can_place: true });
  expect(await screen.findByText(/Mode HR/)).toBeTruthy();
  expect(screen.queryByText("Anda bukan pengatur jadwal toko. Hubungi HR bila seharusnya punya akses.")).toBeNull();
  await waitFor(() => expect(placementApi.fetchStoreBranches).toHaveBeenCalled());
  expect(screen.getByRole("tab", { name: "Penempatan" })).toBeTruthy();
});

// Defensif: di server `hr` (update Jadwal Kerja, di bawah hris.kepegawaian) selalu berarti
// hr_can_place juga true; halaman tetap menghormati flag bila suatu saat berbeda.
it("HR tanpa menu penempatan (defensif): hanya tab Jadwal (EPIC-073 R2)", async () => {
  placementApi.fetchStoreBranches.mockResolvedValue([STORE]);
  renderPage({ is_delegate: false, can_place: false, branches: [], hr: true, hr_can_place: false });
  expect(await screen.findByText(/Mode HR/)).toBeTruthy();
  expect(screen.queryByRole("tab", { name: "Penempatan" })).toBeNull();
});

it("HR yang juga pengatur memakai mode HR (EPIC-073 R2)", async () => {
  placementApi.fetchStoreBranches.mockResolvedValue([STORE]);
  renderPage({ is_delegate: true, can_place: false, branches: [STORE], hr: true, hr_can_place: true });
  expect(await screen.findByText(/Mode HR/)).toBeTruthy();
});

it("mode HR tanpa judul HR ganda; subjudul menyebut semua toko (review EPIC-073)", async () => {
  placementApi.fetchStoreBranches.mockResolvedValue([STORE]);
  renderPage({ is_delegate: false, can_place: false, branches: [], hr: true, hr_can_place: true });
  await screen.findByText(/Mode HR/);
  expect(screen.queryByRole("heading", { name: "Jadwal Kerja" })).toBeNull();
  expect(screen.getByText(/Atur jadwal kerja karyawan di semua toko/)).toBeTruthy();
  fireEvent.click(screen.getByRole("tab", { name: "Penempatan" }));
  await screen.findByText("Ana");
  expect(screen.queryByRole("heading", { name: "Penempatan Karyawan" })).toBeNull();
});

it("mode HR: cabang tanpa karyawan Shift → tombol ke tab Penempatan, tidak keluar halaman (review EPIC-073)", async () => {
  placementApi.fetchStoreBranches.mockResolvedValue([STORE]);
  scheduleApi.fetchScheduleGrid.mockImplementation(async (p: { from: string; to: string }) => ({
    data: { ...grid(p.from, p.to), employees: [] },
  }));
  renderPage({ is_delegate: false, can_place: false, branches: [], hr: true, hr_can_place: true });
  await screen.findByText(/Mode HR/);
  fireEvent.change(await screen.findByLabelText("Pilih cabang"), { target: { value: "s1" } });
  fireEvent.click(await screen.findByRole("button", { name: "Atur penempatan karyawan" }));
  expect(push).not.toHaveBeenCalled();
  await waitFor(() => expect(screen.getByRole("tab", { name: "Penempatan" }).getAttribute("aria-selected")).toBe("true"));
});

