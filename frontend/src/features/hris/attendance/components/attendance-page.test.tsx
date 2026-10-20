import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AttendancePage } from "./attendance-page";

let role: string | null = "hrd";
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ user: role ? { id: "u1", email: "a@b.c", role } : null, loading: false, signOut: vi.fn() }),
}));
vi.mock("@/features/hris/business-trips", () => ({ BusinessTripsTab: () => <div>tab-dinas</div> }));
vi.mock("./attendance-monitoring-tab", () => ({ AttendanceMonitoringTab: () => <div>monitoring</div> }));
vi.mock("./attendance-recap-tab", () => ({ AttendanceRecapTab: () => <div>rekap</div> }));
vi.mock("./attendance-periods-tab", () => ({ AttendancePeriodsTab: () => <div>periode</div> }));

afterEach(cleanup);

describe("AttendancePage: tab Perjalanan Dinas", () => {
  it.each(["super_admin", "admin", "hrd"])("ditawarkan untuk %s", (r) => {
    role = r;
    render(<AttendancePage />);
    expect(screen.getByRole("tab", { name: "Perjalanan Dinas" })).toBeTruthy();
  });

  it.each(["hiring_manager", "employee", null])("tidak ditawarkan untuk %s", (r) => {
    role = r;
    render(<AttendancePage />);
    expect(screen.queryByRole("tab", { name: "Perjalanan Dinas" })).toBeNull();
    expect(screen.getByRole("tab", { name: "Rekap" })).toBeTruthy();
  });
});
