import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AttendancePage } from "./attendance-page";

let role: string | null = "hrd";
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: role ? { id: "u1", email: "a@b.c", role } : null, loading: false, signOut: vi.fn() }),
}));
let access: { data?: { attendance: boolean; leaves: boolean; overtime: boolean } } = {};
vi.mock("@/features/hris/approve-access", () => ({ useHrApproveAccess: () => access }));
const grant = (attendance: boolean) => {
  access = { data: { attendance, leaves: false, overtime: false } };
};
vi.mock("@/features/hris/business-trips", () => ({ BusinessTripsTab: () => <div>tab-dinas</div> }));
vi.mock("@/features/hris/attendance-requests", () => ({ AttendanceRequestsTab: () => <div>tab-pengajuan</div> }));
vi.mock("./attendance-gap-summary-card", () => ({ AttendanceGapSummaryCard: () => <div>kartu-ringkasan</div> }));
vi.mock("./attendance-monitoring-tab", () => ({ AttendanceMonitoringTab: () => <div>monitoring</div> }));
vi.mock("./attendance-recap-tab", () => ({ AttendanceRecapTab: () => <div>rekap</div> }));
vi.mock("./attendance-periods-tab", () => ({ AttendancePeriodsTab: () => <div>periode</div> }));

afterEach(() => {
  cleanup();
  role = "hrd";
  access = {};
});

describe("AttendancePage: tab HR mengikuti akses approve (EPIC-067 R14)", () => {
  it("approve absensi = true: Pengajuan Absensi dan Perjalanan Dinas tampil walau role bukan HR", () => {
    role = "pos";
    grant(true);
    render(<AttendancePage />);
    expect(screen.getByRole("tab", { name: "Pengajuan Absensi" })).toBeTruthy();
    expect(screen.getByRole("tab", { name: "Perjalanan Dinas" })).toBeTruthy();
  });

  it("approve absensi = false: kedua tab tidak ada walau role hrd", () => {
    role = "hrd";
    grant(false);
    render(<AttendancePage />);
    expect(screen.queryByRole("tab", { name: "Pengajuan Absensi" })).toBeNull();
    expect(screen.queryByRole("tab", { name: "Perjalanan Dinas" })).toBeNull();
    expect(screen.getByRole("tab", { name: "Rekap" })).toBeTruthy();
  });

  it("selama memuat atau gagal (data kosong) tab HR tidak tampil (gagal tutup, tanpa berkedip)", () => {
    role = "hrd";
    access = {};
    render(<AttendancePage />);
    expect(screen.queryByRole("tab", { name: "Pengajuan Absensi" })).toBeNull();
    expect(screen.queryByRole("tab", { name: "Perjalanan Dinas" })).toBeNull();
  });
});

describe("AttendancePage: kartu ringkasan absen terlewat (EPIC-067)", () => {
  it("dirender di atas tab", () => {
    grant(true);
    render(<AttendancePage />);
    const card = screen.getByText("kartu-ringkasan");
    const tabs = screen.getByRole("tablist");
    expect(card.compareDocumentPosition(tabs) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

describe("AttendancePage: tab di HP (permintaan 30-10)", () => {
  it("di layar kecil daftar tab bisa digeser, bukan dipaksa 5 kolom sama lebar; grid baru dari sm", () => {
    grant(true);
    render(<AttendancePage />);
    const list = screen.getByRole("tablist");
    const classes = list.className.split(/\s+/);
    expect(classes).not.toContain("grid");
    expect(classes).toContain("overflow-x-auto");
    expect(classes).toContain("sm:grid");
    expect(classes).toContain("sm:grid-cols-5");
  });
});

