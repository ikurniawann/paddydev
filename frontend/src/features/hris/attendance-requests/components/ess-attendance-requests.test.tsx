import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AttendanceGaps, AttendanceRequest } from "../types";
import { EssAttendanceRequests } from "./ess-attendance-requests";

const create = vi.fn();
const cancel = vi.fn();
let gaps: AttendanceGaps = { start: null, end: null, days: [] };
let mine: AttendanceRequest[] = [];
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@/lib/dates", async (orig) => ({ ...(await orig<typeof import("@/lib/dates")>()), todayWib: () => "2026-10-05" }));
vi.mock("../queries", () => ({
  useMyAttendanceGaps: () => ({ data: gaps }),
  useMyAttendanceRequests: () => ({ data: mine }),
}));
vi.mock("../mutations", () => ({
  useCreateAttendanceRequest: () => ({ mutate: create, isPending: false }),
  useCancelAttendanceRequest: () => ({ mutate: cancel, isPending: false }),
}));

function req(over: Partial<AttendanceRequest>): AttendanceRequest {
  return {
    id: "r1", employee_id: "e1", date: "2026-10-03", kind: "correction", clock_in: null, clock_out: "17:00",
    reason: "Lupa absen pulang, HP mati", attachment_path: null, status: "pending", attendance_id: null, decided_by: null,
    decided_by_name: null, decided_at: null, rejection_reason: null, created_at: "2026-10-04T01:00:00Z",
    updated_at: "2026-10-04T01:00:00Z", employee: { id: "e1", full_name: "Budi Santoso", nip: "N01" }, recorded: null, ...over,
  };
}

afterEach(() => {
  cleanup();
  create.mockReset();
  cancel.mockReset();
  gaps = { start: null, end: null, days: [] };
  mine = [];
});

describe("EssAttendanceRequests", () => {
  it("kartu Absen terlewat membuka form bertanggal dengan petunjuk", () => {
    gaps = {
      start: "2026-10-01",
      end: "2026-10-04",
      days: [
        { date: "2026-10-01", kind: "missing", clock_in: null },
        { date: "2026-10-03", kind: "no_clock_out", clock_in: "08:05" },
      ],
    };
    render(<EssAttendanceRequests />);
    expect(screen.getByText("Absen terlewat (2)")).toBeTruthy();
    expect(screen.getByText(/Tidak ada absen/)).toBeTruthy();
    fireEvent.click(screen.getAllByRole("button", { name: "Ajukan koreksi" })[1]);
    expect((screen.getByLabelText("Tanggal") as HTMLInputElement).value).toBe("2026-10-03");
    expect(screen.getByText("Tercatat masuk 08:05 tanpa absen pulang — isi jam pulang yang benar.")).toBeTruthy();
  });

  it("tanpa absen terlewat hanya ada tombol umum, form bertanggal hari ini", () => {
    render(<EssAttendanceRequests />);
    expect(screen.queryByText(/Absen terlewat/)).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Ajukan absen manual / koreksi" }));
    expect((screen.getByLabelText("Tanggal") as HTMLInputElement).value).toBe("2026-10-05");
  });

  it("status pengajuan: alasan ditolak tampil, batal hanya saat menunggu dan lewat konfirmasi", () => {
    mine = [req({ id: "r1" }), req({ id: "r2", status: "rejected", rejection_reason: "Tidak sesuai CCTV" })];
    render(<EssAttendanceRequests />);
    expect(screen.getByText("Menunggu HR")).toBeTruthy();
    expect(screen.getByText("Alasan ditolak: Tidak sesuai CCTV")).toBeTruthy();
    const buttons = screen.getAllByRole("button", { name: "Batalkan" });
    expect(buttons).toHaveLength(1);
    fireEvent.click(buttons[0]);
    expect(cancel).not.toHaveBeenCalled();
    expect(screen.getByText("Batalkan pengajuan absensi?")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Ya, batalkan" }));
    expect(cancel).toHaveBeenCalledWith("r1", expect.anything());
  });

  it("membatalkan konfirmasi ('Kembali') tidak membatalkan pengajuan", () => {
    mine = [req({ id: "r1" })];
    render(<EssAttendanceRequests />);
    fireEvent.click(screen.getByRole("button", { name: "Batalkan" }));
    fireEvent.click(screen.getByRole("button", { name: "Kembali" }));
    expect(cancel).not.toHaveBeenCalled();
  });
});
