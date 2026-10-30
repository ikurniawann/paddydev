import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DailyRosterData } from "@/features/hris/attendance/types";
import { TeamAttendanceCard } from "./team-attendance-card";

let data: Partial<DailyRosterData> | undefined;
vi.mock("@/features/hris/attendance/queries", () => ({ useTeamAttendance: () => ({ data }) }));

afterEach(() => {
  cleanup();
  data = undefined;
});

const summary = (over: Record<string, number>) =>
  ({ hadir: 0, terlambat: 0, belum_absen: 0, absen: 0, cuti: 0, libur_nasional: 0, libur: 0, tanpa_jadwal: 0, scheduled: 0, ...over }) as DailyRosterData["summary"];

describe("TeamAttendanceCard (EPIC-074 R7)", () => {
  it("atasan: ringkasan hari ini dan tautan ke Absensi Tim", () => {
    data = { summary: summary({ hadir: 3, terlambat: 1, belum_absen: 2 }), employees: new Array(6).fill(null) };
    render(<TeamAttendanceCard />);
    const link = screen.getByRole("link", { name: /Absensi Tim/ });
    expect(link.getAttribute("href")).toBe("/dashboard/me/absensi-tim");
    expect(screen.getByText("4 hadir · 1 terlambat · 2 belum absen")).toBeTruthy();
  });

  it("tanpa tim atau belum dimuat: tidak dirender", () => {
    data = { summary: summary({}), employees: [] };
    const { container, rerender } = render(<TeamAttendanceCard />);
    expect(container.innerHTML).toBe("");
    data = undefined;
    rerender(<TeamAttendanceCard />);
    expect(container.innerHTML).toBe("");
  });
});
