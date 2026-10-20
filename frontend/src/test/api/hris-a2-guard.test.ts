/**
 * Guard route HRIS payroll, cuti, lembur, pinjaman, logbook, KPI, kinerja,
 * on/offboarding, ESS, dan laporan setelah migrasi ke apiHandler: tanpa
 * grant menu → 403, tanpa sesi → 401, dan database tidak disentuh.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NextResponse } from "next/server";

const requireIamMenuPrefix = vi.fn();
const requireIamGuard = vi.fn();
const requireApiUser = vi.fn();
const getWorkforceActor = vi.fn();
const getLogbookActor = vi.fn();
const dbTouched = vi.fn();

vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return {
    ...actual,
    requireIamMenuPrefix: (...args: unknown[]) => requireIamMenuPrefix(...args),
    // Aksi menu (approve pinjaman) ikut grant palsu yang sama.
    requireIamAction: (prefixes: unknown) => requireIamMenuPrefix(prefixes),
    requireIamGuard: (...args: unknown[]) => requireIamGuard(...args),
    requireApiUser: (...args: unknown[]) => requireApiUser(...args),
  };
});
vi.mock("@/lib/hris/workforce-auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/hris/workforce-auth")>();
  return { ...actual, getWorkforceActor: () => getWorkforceActor() };
});
vi.mock("@/lib/hris/logbook", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/hris/logbook")>();
  return { ...actual, getLogbookActor: () => getLogbookActor() };
});

const touch = () => {
  dbTouched();
  throw new Error("database must not be reached");
};
vi.mock("@/lib/db", () => ({ query: touch, queryOne: touch, withTransaction: touch, getPool: touch }));
vi.mock("@/lib/pg/create-client", () => ({
  createPgClient: touch,
  createServerPgClient: async () => touch(),
}));

type RouteModule = Record<string, unknown>;
type Handler = (req: never, ctx: never) => Promise<Response>;

const ID = "00000000-0000-0000-0000-000000000001";
const ctx = {
  params: Promise.resolve({ id: ID, employee_id: ID, path: ["leave-attachments", ID, "a.jpg"] }),
};

function req(method: string) {
  const url = `http://x/api?employee_id=${ID}&cycle_id=${ID}&id=${ID}&resource=entry`;
  const request = new Request(url, {
    method,
    ...(method === "GET" || method === "DELETE"
      ? {}
      : { body: JSON.stringify({ action: "create-entry" }), headers: { "Content-Type": "application/json" } }),
  }) as Request & { nextUrl: URL };
  request.nextUrl = new URL(url);
  return request as never;
}

async function call(load: () => Promise<RouteModule>, method: string): Promise<Response> {
  const handler = (await load())[method] as Handler;
  return handler(req(method), ctx as never);
}

const expand = (rows: [string, () => Promise<RouteModule>, string[]][]) =>
  rows.flatMap(([name, load, methods]) => methods.map((m) => [`${m} ${name}`, load, m] as const));

beforeEach(async () => {
  const { ApiError } = await import("@/lib/api/auth");
  dbTouched.mockReset();
  requireIamMenuPrefix.mockReset().mockRejectedValue(ApiError.forbidden());
  requireIamGuard.mockReset().mockResolvedValue({
    error: NextResponse.json({ success: false, error: "Insufficient permissions" }, { status: 403 }),
    user: null,
  });
  requireApiUser.mockReset().mockRejectedValue(ApiError.unauthorized());
  getWorkforceActor.mockReset().mockResolvedValue(null);
  getLogbookActor.mockReset().mockResolvedValue(null);
});

const forbidden: [string, () => Promise<RouteModule>, string[]][] = [
  ["payroll", () => import("@/app/api/hris/payroll/route"), ["GET", "POST"]],
  ["payroll/[id]", () => import("@/app/api/hris/payroll/[id]/route"), ["GET", "PUT", "DELETE"]],
  ["payroll/[id]/calculate", () => import("@/app/api/hris/payroll/[id]/calculate/route"), ["POST"]],
  ["payroll-settings", () => import("@/app/api/hris/payroll-settings/route"), ["GET", "PUT"]],
  ["payslips/notify", () => import("@/app/api/hris/payslips/notify/route"), ["POST"]],
  ["loans/[id]/approve", () => import("@/app/api/hris/loans/[id]/approve/route"), ["POST"]],
  ["leaves/export", () => import("@/app/api/hris/leaves/export/route"), ["GET"]],
  ["kpi/recommendation", () => import("@/app/api/hris/kpi/recommendation/route"), ["GET"]],
  ["kpi/snapshot", () => import("@/app/api/hris/kpi/snapshot/route"), ["POST"]],
  ["kpi/targets", () => import("@/app/api/hris/kpi/targets/route"), ["GET", "POST", "DELETE"]],
  ["kpi/scorecards", () => import("@/app/api/hris/kpi/scorecards/route"), ["PATCH"]],
  ["kpi-config", () => import("@/app/api/hris/kpi-config/route"), ["GET", "PUT"]],
  ["promote", () => import("@/app/api/hris/promote/route"), ["POST"]],
  ["shifts", () => import("@/app/api/hris/shifts/route"), ["POST"]],
  ["shifts/[id]", () => import("@/app/api/hris/shifts/[id]/route"), ["PATCH", "DELETE"]],
  ["reports", () => import("@/app/api/hris/reports/route"), ["GET"]],
];

const unauthenticated: [string, () => Promise<RouteModule>, string[]][] = [
  ["payslips", () => import("@/app/api/hris/payslips/route"), ["GET"]],
  ["payslips/[id]/pdf", () => import("@/app/api/hris/payslips/[id]/pdf/route"), ["GET"]],
  ["loans", () => import("@/app/api/hris/loans/route"), ["GET", "POST"]],
  ["leaves", () => import("@/app/api/hris/leaves/route"), ["GET", "POST"]],
  ["leaves/[id]", () => import("@/app/api/hris/leaves/[id]/route"), ["GET", "PUT", "DELETE"]],
  ["leaves/approve", () => import("@/app/api/hris/leaves/approve/route"), ["POST"]],
  ["leaves/attachment", () => import("@/app/api/hris/leaves/attachment/route"), ["POST"]],
  ["leaves/attachment/[...path]", () => import("@/app/api/hris/leaves/attachment/[...path]/route"), ["GET"]],
  ["leave-balances", () => import("@/app/api/hris/leave-balances/[employee_id]/route"), ["GET", "PUT"]],
  ["logbook", () => import("@/app/api/hris/logbook/route"), ["GET", "POST", "PATCH", "DELETE"]],
  ["me", () => import("@/app/api/hris/me/route"), ["GET"]],
  ["me/beranda", () => import("@/app/api/hris/me/beranda/route"), ["GET"]],
  ["me/team", () => import("@/app/api/hris/me/team/route"), ["GET"]],
  ["notifications", () => import("@/app/api/hris/notifications/route"), ["GET", "POST"]],
  ["onboarding", () => import("@/app/api/hris/onboarding/[employee_id]/route"), ["GET", "POST", "PUT"]],
  ["offboarding", () => import("@/app/api/hris/offboarding/[employee_id]/route"), ["GET", "POST", "PUT"]],
  ["overtime", () => import("@/app/api/hris/overtime/route"), ["GET", "POST"]],
  ["overtime/decide", () => import("@/app/api/hris/overtime/decide/route"), ["POST"]],
  ["performance/cycles", () => import("@/app/api/hris/performance/cycles/route"), ["GET", "POST"]],
  ["performance/reviews", () => import("@/app/api/hris/performance/reviews/route"), ["GET"]],
  ["performance/reviews/[id]", () => import("@/app/api/hris/performance/reviews/[id]/route"), ["GET", "PATCH"]],
  ["performance/realtime", () => import("@/app/api/hris/performance/realtime/route"), ["GET"]],
  ["kpi/rubric", () => import("@/app/api/hris/kpi/rubric/route"), ["POST"]],
  ["kpi/scorecards", () => import("@/app/api/hris/kpi/scorecards/route"), ["GET"]],
  ["shifts", () => import("@/app/api/hris/shifts/route"), ["GET"]],
];

describe("tanpa grant menu → 403 tanpa query", () => {
  it.each(expand(forbidden))("%s", async (_name, load, method) => {
    const res = await call(load, method);
    expect(res.status).toBe(403);
    expect(await res.json()).toMatchObject({ success: false });
    expect(dbTouched).not.toHaveBeenCalled();
  });
});

describe("tanpa sesi → 401 tanpa query", () => {
  it.each(expand(unauthenticated))("%s", async (_name, load, method) => {
    const res = await call(load, method);
    expect(res.status).toBe(401);
    expect(dbTouched).not.toHaveBeenCalled();
  });
});

describe("badge navigasi", () => {
  it("GET tanpa sesi tetap { badges: {} }", async () => {
    const { GET } = await import("@/app/api/hris/nav-badges/route");
    const res = await GET();
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ badges: {} });
    expect(dbTouched).not.toHaveBeenCalled();
  });

  it("POST tanpa record karyawan → 403", async () => {
    const { POST } = await import("@/app/api/hris/nav-badges/route");
    const res = await POST(req("POST"));
    expect(res.status).toBe(403);
    expect(dbTouched).not.toHaveBeenCalled();
  });
});
