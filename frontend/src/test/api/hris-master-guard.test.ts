/**
 * Route master/brands/feedback/dashboard rekrutmen dulu tanpa cek izin di
 * handler (audit S1). Tanpa grant menu modul: 403 dan database tidak disentuh.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NextResponse } from "next/server";

const requireIamGuard = vi.fn();
const requireIamMenuPrefix = vi.fn();
const requireIamAction = vi.fn();
const dbTouched = vi.fn();

vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return {
    ...actual,
    requireIamGuard: (...args: unknown[]) => requireIamGuard(...args),
    requireIamMenuPrefix: (...args: unknown[]) => requireIamMenuPrefix(...args),
    // Hapus HRIS dijaga aksi delete menu daun.
    requireIamAction: (...args: unknown[]) => requireIamAction(...args),
  };
});

const touch = () => {
  dbTouched();
  throw new Error("database must not be reached");
};
vi.mock("@/lib/pg/create-client", () => ({
  createPgClient: touch,
  createServerPgClient: async () => touch(),
}));

const forbidden = () => ({
  error: NextResponse.json({ success: false, error: "Insufficient permissions" }, { status: 403 }),
  user: null,
});
const jsonReq = (body: unknown = {}) =>
  new Request("http://x/api", { method: "POST", body: JSON.stringify(body) }) as never;
const idParams = { params: Promise.resolve({ id: "00000000-0000-0000-0000-000000000001" }) };

beforeEach(async () => {
  const { ApiError } = await import("@/lib/api/auth");
  dbTouched.mockReset();
  requireIamGuard.mockReset().mockResolvedValue(forbidden());
  requireIamMenuPrefix.mockReset().mockRejectedValue(ApiError.forbidden());
  requireIamAction.mockReset().mockRejectedValue(ApiError.forbidden());
});

describe("tanpa grant → 403 tanpa query", () => {
  it.each([
    ["master departments POST", async () => (await import("@/app/api/master/departments/route")).POST(jsonReq())],
    ["master departments PUT", async () => (await import("@/app/api/master/departments/[id]/route")).PUT(jsonReq(), idParams)],
    ["master positions DELETE", async () => (await import("@/app/api/master/positions/[id]/route")).DELETE(jsonReq(), idParams)],
    ["master employment-statuses POST", async () => (await import("@/app/api/master/employment-statuses/route")).POST(jsonReq())],
    ["brands POST", async () => (await import("@/app/api/brands/route")).POST(jsonReq())],
    ["brands PATCH", async () => (await import("@/app/api/brands/[id]/route")).PATCH(jsonReq(), idParams)],
    ["sections POST", async () => (await import("@/app/api/sections/route")).POST(jsonReq())],
    ["positions POST", async () => (await import("@/app/api/positions/route")).POST(jsonReq())],
    ["analytics overview", async () => (await import("@/app/api/analytics/overview/route")).GET(jsonReq())],
    ["dashboard attention", async () => (await import("@/app/api/dashboard/attention/route")).GET(jsonReq())],
    ["candidates GET", async () => (await import("@/app/api/candidates/route")).GET(jsonReq())],
    ["cv-upload DELETE", async () => (await import("@/app/api/candidates/[id]/cv-upload/route")).DELETE(jsonReq(), idParams)],
    ["cv-extract POST", async () => (await import("@/app/api/candidates/cv-extract/route")).POST(jsonReq())],
    ["promote POST", async () => (await import("@/app/api/hris/promote/route")).POST(jsonReq())],
    ["feedback categories POST", async () => (await import("@/app/api/hris/feedback-categories/route")).POST(jsonReq())],
    ["feedback cycles DELETE", async () => (await import("@/app/api/hris/feedback-cycles/[id]/route")).DELETE(jsonReq(), idParams)],
  ])("%s", async (_name, run) => {
    const res = (await run()) as Response;
    expect(res.status).toBe(403);
    expect(dbTouched).not.toHaveBeenCalled();
  });
});
