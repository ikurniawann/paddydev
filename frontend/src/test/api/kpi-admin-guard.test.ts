// @vitest-environment node
/**
 * Administrasi KPI (target, status scorecard, snapshot, rekomendasi,
 * konfigurasi) butuh menu pengelola Kinerja. Review & Tugas Departemen yang
 * dipegang semua karyawan juga berawalan hris.performance (EPIC-057).
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";

const requireIamMenuPrefix = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/auth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/auth")>()),
  requireIamMenuPrefix,
}));

const ADMIN = ["hris.performance.kpi-scorecard", "hris.performance.kpi-config"];
type Handler = (req: NextRequest) => Promise<Response>;

beforeEach(async () => {
  const { ApiError } = await import("@/lib/api/auth");
  requireIamMenuPrefix.mockReset().mockRejectedValue(ApiError.forbidden("Insufficient permissions"));
});

const cases: [string, () => Promise<Record<string, unknown>>, string][] = [
  ["targets", () => import("@/app/api/hris/kpi/targets/route"), "GET"],
  ["targets", () => import("@/app/api/hris/kpi/targets/route"), "POST"],
  ["targets", () => import("@/app/api/hris/kpi/targets/route"), "DELETE"],
  ["scorecards", () => import("@/app/api/hris/kpi/scorecards/route"), "PATCH"],
  ["snapshot", () => import("@/app/api/hris/kpi/snapshot/route"), "POST"],
  ["recommendation", () => import("@/app/api/hris/kpi/recommendation/route"), "GET"],
  ["kpi-config", () => import("@/app/api/hris/kpi-config/route"), "GET"],
  ["kpi-config", () => import("@/app/api/hris/kpi-config/route"), "PUT"],
];

describe("guard administrasi KPI", () => {
  it.each(cases)("%s %s", async (name, load, method) => {
    const handler = (await load())[method] as Handler;
    const body = method === "GET" || method === "DELETE" ? undefined : "{}";
    const res = await handler(new NextRequest(`http://localhost/api/hris/${name}`, { method, body }));
    expect(res.status).toBe(403);
    expect(requireIamMenuPrefix).toHaveBeenCalledWith(ADMIN);
  });
});
