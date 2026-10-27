import { NextRequest } from "next/server";
import { describe, expect, it, vi } from "vitest";
import { PUT } from "./route";

vi.mock("@/lib/hris/workforce-route", () => ({ requireWorkforceActor: async () => ({ role: "hrd" }) }));

describe("fallback PUT /api/hris/leave-balances/:employee_id", () => {
  it("menolak dengan 501: saldo cuti dilayani backend Go", async () => {
    const res = await PUT(
      new NextRequest("http://localhost/api/hris/leave-balances/e1?year=2026", {
        method: "PUT",
        body: JSON.stringify({ annual_leave_used: 0 }),
      }),
      { params: Promise.resolve({ employee_id: "e1" }) }
    );
    expect(res.status).toBe(501);
    expect(JSON.stringify(await res.json())).toContain("Ditangani backend Go");
  });
});
