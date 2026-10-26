import { beforeEach, describe, expect, it, vi } from "vitest";

const queryOne = vi.fn();
vi.mock("@/lib/db", () => ({ query: vi.fn(), queryOne: (...args: unknown[]) => queryOne(...args) }));
// EPIC-067 R11: badge mengikuti aksi "approve" pada daun Absensi (Go CanApprove(LeafAbsensi)).
const approves: Record<string, string[]> = {};
const iamCalls: [string, readonly string[], string][] = [];
vi.mock("@/lib/iam/has-menu", async (orig) => ({
  ...(await orig<typeof import("@/lib/iam/has-menu")>()),
  userHasIamAction: async (userId: string, _role: string, prefixes: readonly string[], action: string) => {
    iamCalls.push([userId, prefixes, action]);
    return action === "approve" && prefixes.some((p) => (approves[userId] ?? []).includes(p));
  },
}));

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
  for (const k of Object.keys(approves)) delete approves[k];
  approves["u-hrd"] = ["hris.workforce.attendance"];
  iamCalls.length = 0;
  queryOne.mockReset();
  queryOne.mockImplementation(async (sql: string) =>
    sql.includes("hris.attendance_requests") ? { total: "3" } : { total: "0" }
  );
});

describe("buildNavBadges — pengajuan absensi (EPIC-065, paritas Go)", () => {
  it("HRD mendapat jumlah pengajuan absensi yang menunggu di menu Absensi", async () => {
    const badges = await buildNavBadges(actor({ userId: "u-hrd", role: "hrd", isHr: true, isHrd: true }));
    expect(ATTENDANCE_REQUESTS_HREF).toBe("/dashboard/hris/attendance");
    expect(badges[ATTENDANCE_REQUESTS_HREF]).toBe(3);
    const sql = String(queryOne.mock.calls.map((c) => c[0]).find((s: string) => s.includes("hris.attendance_requests")));
    expect(sql).toContain("status = 'pending'");
  });

  it("hiring_manager (HR tanpa HRD) dan karyawan tidak mendapat badge itu", async () => {
    expect(await buildNavBadges(actor({ role: "hiring_manager", isHr: true }))).not.toHaveProperty(ATTENDANCE_REQUESTS_HREF);
    expect(await buildNavBadges(actor({ employeeId: "e1" }))).not.toHaveProperty(ATTENDANCE_REQUESTS_HREF);
  });

  it("badge mengikuti approve pada daun Absensi, bukan role (EPIC-067 R11)", async () => {
    approves["u-hris"] = ["hris.workforce.attendance"];
    const badges = await buildNavBadges(actor({ userId: "u-hris", role: "admin_hris", isHr: true }));
    expect(badges[ATTENDANCE_REQUESTS_HREF]).toBe(3);
    expect(iamCalls).toContainEqual(["u-hris", ["hris.workforce.attendance"], "approve"]);

    approves["u-hrd"] = ["hris.workforce.leaves", "hris.kepegawaian.overtime"];
    expect(await buildNavBadges(actor({ userId: "u-hrd", role: "hrd", isHr: true, isHrd: true }))).not.toHaveProperty(
      ATTENDANCE_REQUESTS_HREF
    );
  });
});
