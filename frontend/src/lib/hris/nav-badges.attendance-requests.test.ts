import { beforeEach, describe, expect, it, vi } from "vitest";

const queryOne = vi.fn();
vi.mock("@/lib/db", () => ({ query: vi.fn(), queryOne: (...args: unknown[]) => queryOne(...args) }));

import { ATTENDANCE_REQUESTS_HREF, buildNavBadges } from "./nav-badges";
import type { WorkforceActor } from "./workforce-auth";

const actor = (over: Partial<WorkforceActor>): WorkforceActor => ({
  userId: "u1",
  role: "employee",
  employeeId: null,
  isHr: false,
  isHrd: false,
  fullName: "Akun Uji",
  ...over,
});

beforeEach(() => {
  queryOne.mockReset();
  queryOne.mockImplementation(async (sql: string) =>
    sql.includes("hris.attendance_requests") ? { total: "3" } : { total: "0" }
  );
});

describe("buildNavBadges — pengajuan absensi (EPIC-065, paritas Go)", () => {
  it("HRD mendapat jumlah pengajuan absensi yang menunggu di menu Absensi", async () => {
    const badges = await buildNavBadges(actor({ role: "hrd", isHr: true, isHrd: true }));
    expect(ATTENDANCE_REQUESTS_HREF).toBe("/dashboard/hris/attendance");
    expect(badges[ATTENDANCE_REQUESTS_HREF]).toBe(3);
    const sql = String(queryOne.mock.calls.map((c) => c[0]).find((s: string) => s.includes("hris.attendance_requests")));
    expect(sql).toContain("status = 'pending'");
  });

  it("hiring_manager (HR tanpa HRD) dan karyawan tidak mendapat badge itu", async () => {
    expect(await buildNavBadges(actor({ role: "hiring_manager", isHr: true }))).not.toHaveProperty(ATTENDANCE_REQUESTS_HREF);
    expect(await buildNavBadges(actor({ employeeId: "e1" }))).not.toHaveProperty(ATTENDANCE_REQUESTS_HREF);
  });
});
