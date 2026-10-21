import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AttendancePeriod, AttendancePeriodStats } from "../types";
import { ClosePeriodDialog } from "./attendance-period-dialogs";

let detailStats: AttendancePeriodStats | undefined;
vi.mock("../queries", () => ({
  useAttendancePeriod: () => ({ data: detailStats ? { stats: detailStats } : undefined }),
}));
vi.mock("../mutations", () => ({
  useCloseAttendancePeriod: () => ({ mutate: vi.fn(), isPending: false }),
  useReopenAttendancePeriod: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdateAttendancePeriodClosing: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const period = {
  id: "p1",
  name: "Oktober 2026",
  label: "1–31 Okt 2026",
  closing_date: "2026-11-03",
} as AttendancePeriod;

const base: AttendancePeriodStats = {
  employees: 10,
  rows: 200,
  without_clock_out: 0,
  unvalidated: 0,
  adjusted: 0,
  late: 0,
};

afterEach(() => {
  cleanup();
  detailStats = undefined;
});

describe("ClosePeriodDialog", () => {
  it("memperingatkan pengajuan absensi yang belum diputus", () => {
    detailStats = { ...base, pending_requests: 3 };
    render(<ClosePeriodDialog period={period} open onOpenChange={() => {}} />);
    expect(screen.getByText("Pengajuan absensi menunggu")).toBeTruthy();
    expect(
      screen.getByText(/Ada 3 pengajuan absensi karyawan yang belum diputus.*tab Pengajuan Absensi.*tidak bisa disetujui/)
    ).toBeTruthy();
  });

  it("tanpa pengajuan menunggu tidak ada peringatan", () => {
    detailStats = { ...base, pending_requests: 0 };
    render(<ClosePeriodDialog period={period} open onOpenChange={() => {}} />);
    expect(screen.getByText("Pengajuan absensi menunggu")).toBeTruthy();
    expect(screen.queryByText(/pengajuan absensi karyawan yang belum diputus/)).toBeNull();
  });
});
