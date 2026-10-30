import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { DailyRosterData, RosterEmployee } from "@/features/hris/attendance/types";
import { EssAbsensiTimPage } from "./absensi-tim-page";

const api = vi.hoisted(() => ({ fetchTeamAttendance: vi.fn(), fetchDailyRoster: vi.fn() }));
vi.mock("@/features/hris/attendance/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/hris/attendance/api")>()),
  ...api,
}));
vi.mock("@/components/ui/select", () => ({
  Select: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectValue: () => null,
  SelectContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SelectItem: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

const member = (over: Partial<RosterEmployee>): RosterEmployee => ({
  employee_id: "e1", full_name: "Koordinator K", nip: "N1", photo_url: null, department_name: null, job_title: null,
  shift: null, status: "belum_absen", is_overdue: false, attendance: null, leave: null, ...over,
});

function roster(employees: RosterEmployee[]): DailyRosterData {
  return {
    date: "2026-10-30", day_of_week: 5, is_today: true,
    summary: { hadir: 1, terlambat: 0, belum_absen: 1, absen: 0, cuti: 0, libur_nasional: 0, libur: 0, tanpa_jadwal: 0, scheduled: 2 },
    shifts: [], holidays: [], employees,
  } as DailyRosterData;
}

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><EssAbsensiTimPage /></QueryClientProvider>);
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("EssAbsensiTimPage (EPIC-074)", () => {
  it("menampilkan anggota langsung & tidak langsung; anggota tidak langsung menyebut atasannya", async () => {
    api.fetchTeamAttendance.mockResolvedValue(roster([
      member({ direct: true, reporting_to_name: "Atasan A" }),
      member({ employee_id: "e2", full_name: "Staf S", status: "hadir", direct: false, reporting_to_name: "Koordinator K" }),
    ]));
    renderPage();
    expect(await screen.findByText("Staf S")).toBeTruthy();
    expect(screen.getByText("Koordinator K", { selector: "p,span,div" })).toBeTruthy();
    expect(screen.getByText("Atasan: Koordinator K")).toBeTruthy();
    expect(screen.queryByText("Atasan: Atasan A")).toBeNull();
    expect(api.fetchDailyRoster).not.toHaveBeenCalled();
    expect(screen.getByRole("heading", { name: "Absensi Tim" })).toBeTruthy();
  });

  it("tanpa anggota tim → penjelasan kosong", async () => {
    api.fetchTeamAttendance.mockResolvedValue(roster([]));
    renderPage();
    expect(await screen.findByText("Belum ada anggota tim.")).toBeTruthy();
    // EPIC-074 sisa: tanpa tim, ringkasan & filter tidak ditampilkan (bukan deretan angka nol).
    expect(screen.queryByText(/Terjadwal/)).toBeNull();
    expect(screen.queryByPlaceholderText("Nama / NIP…")).toBeNull();
  });
});
