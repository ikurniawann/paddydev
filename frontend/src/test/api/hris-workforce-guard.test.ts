/**
 * Guard route HRIS kepegawaian, absensi, kontrak, libur, pengumuman, task
 * departemen, gaji, 360 feedback, dan lowongan setelah migrasi ke apiHandler:
 * tanpa sesi → 401, tanpa grant menu → 403, dan database tidak disentuh.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NextResponse } from "next/server";

const requireIamMenuPrefix = vi.fn();
const requireIamGuard = vi.fn();
const requireApiUser = vi.fn();
const getWorkforceActor = vi.fn();
const dbTouched = vi.fn();

vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return {
    ...actual,
    requireIamMenuPrefix: (...args: unknown[]) => requireIamMenuPrefix(...args),
    // Hapus HRIS dijaga aksi delete menu daun (requireIamAction): ikut grant palsu yang sama.
    requireIamAction: (prefixes: unknown) => requireIamMenuPrefix(prefixes),
    requireIamGuard: (...args: unknown[]) => requireIamGuard(...args),
    requireApiUser: (...args: unknown[]) => requireApiUser(...args),
  };
});

vi.mock("@/lib/hris/workforce-auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/hris/workforce-auth")>();
  return { ...actual, getWorkforceActor: () => getWorkforceActor() };
});

const touch = () => {
  dbTouched();
  throw new Error("database must not be reached");
};
vi.mock("@/lib/db", () => ({
  query: touch,
  queryOne: touch,
  withTransaction: touch,
  getPool: touch,
}));
vi.mock("@/lib/pg/create-client", () => ({
  createPgClient: touch,
  createServerPgClient: async () => touch(),
}));

type Handler = (req: never, ctx: never) => Promise<Response>;
type RouteModule = Record<string, unknown>;

const ID = "00000000-0000-0000-0000-000000000001";
const ctx = { params: Promise.resolve({ id: ID, doc_id: ID, path: ["announcements", "a.jpg"] }) };

function req(url = "http://x/api?employee_id=" + ID, method = "GET") {
  const request = new Request(url, {
    method,
    ...(method === "GET" ? {} : { body: "{}", headers: { "Content-Type": "application/json" } }),
  }) as Request & { nextUrl: URL };
  request.nextUrl = new URL(url);
  return request as never;
}

async function call(load: () => Promise<RouteModule>, method: string): Promise<Response> {
  const handler = (await load())[method] as Handler;
  return handler(req(undefined, method), ctx as never);
}

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
});

const forbidden: [string, () => Promise<RouteModule>, string[]][] = [
  ["announcements", () => import("@/app/api/hris/announcements/route"), ["GET", "POST"]],
  ["announcements/[id]", () => import("@/app/api/hris/announcements/[id]/route"), ["PATCH", "DELETE"]],
  ["announcements/[id]/read", () => import("@/app/api/hris/announcements/[id]/read/route"), ["POST"]],
  ["announcements/cover", () => import("@/app/api/hris/announcements/cover/route"), ["POST"]],
  ["attendance/export", () => import("@/app/api/hris/attendance/export/route"), ["GET"]],
  ["attendance/[id]", () => import("@/app/api/hris/attendance/[id]/route"), ["DELETE"]],
  ["contracts", () => import("@/app/api/hris/contracts/route"), ["GET"]],
  ["contracts/expiring", () => import("@/app/api/hris/contracts/expiring/route"), ["GET"]],
  ["contracts/[id]", () => import("@/app/api/hris/contracts/[id]/route"), ["PATCH", "DELETE"]],
  ["contracts/[id]/document", () => import("@/app/api/hris/contracts/[id]/document/route"), ["GET"]],
  [
    "contracts/[id]/signed-document",
    () => import("@/app/api/hris/contracts/[id]/signed-document/route"),
    ["POST", "GET", "DELETE"],
  ],
  ["employee-salary", () => import("@/app/api/hris/employee-salary/route"), ["GET", "POST"]],
  ["employee-salary/[id]", () => import("@/app/api/hris/employee-salary/[id]/route"), ["GET", "PUT", "DELETE"]],
  ["employees", () => import("@/app/api/hris/employees/route"), ["POST"]],
  ["employees/[id]/contracts", () => import("@/app/api/hris/employees/[id]/contracts/route"), ["GET", "POST"]],
  [
    "employees/[id]/recruitment-documents",
    () => import("@/app/api/hris/employees/[id]/recruitment-documents/route"),
    ["GET"],
  ],
  ["employees/documents", () => import("@/app/api/hris/employees/documents/route"), ["POST"]],
  ["employment-history", () => import("@/app/api/hris/employment-history/route"), ["POST"]],
  ["feedback-approvals", () => import("@/app/api/hris/feedback-approvals/route"), ["GET", "POST"]],
  ["feedback-approvals/approve", () => import("@/app/api/hris/feedback-approvals/approve/route"), ["POST"]],
  ["feedback-approvals/reject", () => import("@/app/api/hris/feedback-approvals/reject/route"), ["POST"]],
  ["feedback-assignments", () => import("@/app/api/hris/feedback-assignments/route"), ["GET", "POST"]],
  [
    "feedback-assignments/[id]",
    () => import("@/app/api/hris/feedback-assignments/[id]/route"),
    ["GET", "PUT", "DELETE"],
  ],
  ["feedback-categories", () => import("@/app/api/hris/feedback-categories/route"), ["GET", "POST"]],
  ["feedback-cycles", () => import("@/app/api/hris/feedback-cycles/route"), ["GET", "POST"]],
  ["feedback-cycles/[id]", () => import("@/app/api/hris/feedback-cycles/[id]/route"), ["GET", "PUT", "DELETE"]],
  ["feedback-responses", () => import("@/app/api/hris/feedback-responses/route"), ["GET", "POST"]],
  [
    "feedback-responses/[id]",
    () => import("@/app/api/hris/feedback-responses/[id]/route"),
    ["GET", "PUT", "DELETE", "POST", "PATCH"],
  ],
  ["feedback-summaries", () => import("@/app/api/hris/feedback-summaries/route"), ["GET", "POST"]],
  ["holidays", () => import("@/app/api/hris/holidays/route"), ["POST"]],
  ["holidays/[id]", () => import("@/app/api/hris/holidays/[id]/route"), ["PATCH", "DELETE"]],
  ["holidays/import", () => import("@/app/api/hris/holidays/import/route"), ["GET", "POST"]],
  ["job-openings", () => import("@/app/api/hris/job-openings/route"), ["GET", "POST"]],
  ["job-openings/[id]", () => import("@/app/api/hris/job-openings/[id]/route"), ["PATCH", "DELETE"]],
];

const unauthenticated: [string, () => Promise<RouteModule>, string[]][] = [
  ["announcements/[id]", () => import("@/app/api/hris/announcements/[id]/route"), ["GET"]],
  ["announcements/feed", () => import("@/app/api/hris/announcements/feed/route"), ["GET"]],
  ["announcements/cover/[...path]", () => import("@/app/api/hris/announcements/cover/[...path]/route"), ["GET"]],
  ["attendance", () => import("@/app/api/hris/attendance/route"), ["GET", "POST"]],
  ["attendance/[id]", () => import("@/app/api/hris/attendance/[id]/route"), ["GET", "PUT"]],
  ["attendance/stats", () => import("@/app/api/hris/attendance/stats/route"), ["GET"]],
  ["attendance/schedule", () => import("@/app/api/hris/attendance/schedule/route"), ["GET"]],
  ["attendance/daily-roster", () => import("@/app/api/hris/attendance/daily-roster/route"), ["GET"]],
  ["departments", () => import("@/app/api/hris/departments/route"), ["GET"]],
  ["dept-tasks", () => import("@/app/api/hris/dept-tasks/route"), ["GET", "POST"]],
  ["dept-tasks/[id]", () => import("@/app/api/hris/dept-tasks/[id]/route"), ["PATCH"]],
  ["dept-tasks/occurrences/[id]", () => import("@/app/api/hris/dept-tasks/occurrences/[id]/route"), ["PATCH"]],
  ["employees/[id]/lifecycle", () => import("@/app/api/hris/employees/[id]/lifecycle/route"), ["GET"]],
  ["employees/[id]/shifts", () => import("@/app/api/hris/employees/[id]/shifts/route"), ["GET", "PUT"]],
  ["employees/documents", () => import("@/app/api/hris/employees/documents/route"), ["GET"]],
  ["employment-history", () => import("@/app/api/hris/employment-history/route"), ["GET"]],
  ["holidays", () => import("@/app/api/hris/holidays/route"), ["GET"]],
];

describe("tanpa grant menu → 403 tanpa query", () => {
  it.each(forbidden.flatMap(([name, load, methods]) => methods.map((m) => [`${m} ${name}`, load, m] as const)))(
    "%s",
    async (_name, load, method) => {
      const res = await call(load, method);
      expect(res.status).toBe(403);
      expect(dbTouched).not.toHaveBeenCalled();
    }
  );
});

describe("tanpa sesi → 401 tanpa query", () => {
  it.each(
    unauthenticated.flatMap(([name, load, methods]) => methods.map((m) => [`${m} ${name}`, load, m] as const))
  )("%s", async (_name, load, method) => {
    const res = await call(load, method);
    expect(res.status).toBe(401);
    expect(dbTouched).not.toHaveBeenCalled();
  });
});

describe("badge pengumuman", () => {
  it("tanpa sesi tetap { unread: 0 } tanpa query", async () => {
    const { GET } = await import("@/app/api/hris/announcements/unread-count/route");
    const res = await GET();
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ unread: 0 });
    expect(dbTouched).not.toHaveBeenCalled();
  });
});
