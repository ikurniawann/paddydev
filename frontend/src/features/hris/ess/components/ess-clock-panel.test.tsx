import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EssClockPanel } from "./ess-clock-panel";

const toastError = vi.fn();
const toastSuccess = vi.fn();

vi.mock("sonner", () => ({
  toast: { error: (...a: unknown[]) => toastError(...a), success: (...a: unknown[]) => toastSuccess(...a) },
}));
vi.mock("@/components/hris/CameraCapture", () => ({
  CameraCapture: ({ open, onConfirm }: { open: boolean; onConfirm: (photo: string) => void }) =>
    open ? <button onClick={() => onConfirm("data:image/jpeg;base64,SELFIE")}>Ambil foto</button> : null,
}));
let todayData: Record<string, unknown> | null = null;
vi.mock("../queries", () => ({
  useTodayAttendance: () => ({ data: todayData, isLoading: false }),
}));
vi.mock("@/features/hris/placements/queries", () => ({
  useClockPlacements: () => ({
    data: {
      placements: [{ branch_id: "b1", branch_name: "PVJ", is_primary: true, lat: null, lng: null }],
      default_branch_id: "b1",
    },
    isLoading: false,
  }),
}));

const note422 = {
  success: false,
  error: "Anda di luar area PVJ (±334 m, batas 250 m). Isi keterangan minimal 5 karakter untuk tetap absen.",
  details: { code: "geofence_note_required", distance_m: 334, radius_m: 250, branch_name: "PVJ" },
};

function json(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <EssClockPanel />
    </QueryClientProvider>
  );
}

const getCurrentPosition = vi.fn();
const gps = { latitude: -6.88, longitude: 107.6, accuracy: 12 };

beforeEach(() => {
  toastError.mockClear();
  toastSuccess.mockClear();
  todayData = null;
  getCurrentPosition.mockReset();
  getCurrentPosition.mockImplementation((ok: (p: unknown) => void) => ok({ coords: gps }));
  vi.stubGlobal("navigator", { ...navigator, geolocation: { getCurrentPosition } });
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("EssClockPanel di luar area", () => {
  it("422 -> popup -> kirim ulang payload yang sama + notes", async () => {
    const fetchFn = vi
      .fn()
      .mockResolvedValueOnce(json(422, note422))
      .mockResolvedValueOnce(
        json(200, { message: "ok", data: { is_late: false, clock_in_geofence: "invalid", clock_in_distance_m: 334 } })
      );
    vi.stubGlobal("fetch", fetchFn);
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /Clock In/ }));
    fireEvent.click(screen.getByRole("button", { name: "Ambil foto" }));

    // Popup memakai radius_m dari server (250), bukan 100.
    expect(await screen.findByText(/batas 250 m/)).toBeTruthy();
    expect(toastError).not.toHaveBeenCalled();
    expect(fetchFn).toHaveBeenCalledTimes(1);

    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "  Ambil stok di gudang " } });
    fireEvent.click(screen.getByRole("button", { name: "Tetap absen" }));

    await waitFor(() => expect(fetchFn).toHaveBeenCalledTimes(2));
    const first = JSON.parse(fetchFn.mock.calls[0][1].body);
    const second = JSON.parse(fetchFn.mock.calls[1][1].body);
    expect(first.notes).toBeUndefined();
    expect(second).toEqual({ ...first, notes: "Ambil stok di gudang" });
    expect(second.photo).toBe("data:image/jpeg;base64,SELFIE");
    // Lokasi dan toko sama dengan permintaan pertama; GPS tidak diambil ulang.
    expect(first.clock_in_location).toEqual(gps);
    expect(first.branch_id).toBe("b1");
    expect(second.clock_in_location).toEqual(first.clock_in_location);
    expect(second.branch_id).toBe(first.branch_id);
    expect(getCurrentPosition).toHaveBeenCalledTimes(1);

    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    expect(toastSuccess.mock.calls[0][1].description).toContain("Di luar area · 334 m");
    expect(screen.queryByRole("button", { name: "Tetap absen" })).toBeNull();
  });

  it("clock-out di luar area mengirim ulang dengan attendance_id yang sama + notes", async () => {
    todayData = { id: "att-1", clock_in: "2026-10-20T01:00:00Z", clock_out: null, is_late: false, late_minutes: 0 };
    const fetchFn = vi
      .fn()
      .mockResolvedValueOnce(json(422, note422))
      .mockResolvedValueOnce(json(200, { data: { clock_out_geofence: "invalid", clock_out_distance_m: 334 } }));
    vi.stubGlobal("fetch", fetchFn);
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /Clock Out/ }));
    fireEvent.click(screen.getByRole("button", { name: "Ambil foto" }));
    await screen.findByText(/batas 250 m/);
    fireEvent.change(screen.getByLabelText("Keterangan"), { target: { value: "Antar barang" } });
    fireEvent.click(screen.getByRole("button", { name: "Tetap absen" }));

    await waitFor(() => expect(fetchFn).toHaveBeenCalledTimes(2));
    const first = JSON.parse(fetchFn.mock.calls[0][1].body);
    const second = JSON.parse(fetchFn.mock.calls[1][1].body);
    expect(first).toMatchObject({ action: "clock-out", attendance_id: "att-1", clock_out_location: gps });
    expect(second).toEqual({ ...first, notes: "Antar barang" });
    expect(getCurrentPosition).toHaveBeenCalledTimes(1);
  });

  it("Batal menutup popup tanpa mengirim ulang", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json(422, note422));
    vi.stubGlobal("fetch", fetchFn);
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /Clock In/ }));
    fireEvent.click(screen.getByRole("button", { name: "Ambil foto" }));
    await screen.findByRole("button", { name: "Tetap absen" });
    fireEvent.click(screen.getByRole("button", { name: "Batal" }));

    await waitFor(() => expect(screen.queryByRole("button", { name: "Tetap absen" })).toBeNull());
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });

  it("error lain tetap tampil sebagai toast, tanpa popup", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json(500, { error: "Server error" })));
    renderPanel();

    fireEvent.click(screen.getByRole("button", { name: /Clock In/ }));
    fireEvent.click(screen.getByRole("button", { name: "Ambil foto" }));

    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Tetap absen" })).toBeNull();
  });
});

describe("EssClockPanel badge geofence hari ini", () => {
  it("menampilkan badge Masuk dan Pulang dari absensi hari ini", () => {
    todayData = {
      id: "att-1",
      clock_in: "2026-10-20T01:00:00Z",
      clock_out: "2026-10-20T09:00:00Z",
      is_late: false,
      late_minutes: 0,
      clock_in_geofence: "invalid",
      clock_in_distance_m: 334,
      clock_out_geofence: "valid",
      clock_out_distance_m: 20,
    };
    renderPanel();
    expect(screen.getByText("Masuk: Di luar area · 334 m")).toBeTruthy();
    expect(screen.getByText("Pulang: Dalam area · 20 m")).toBeTruthy();
  });
});
