/**
 * Guard record karyawan (audit S1): GET/PUT/DELETE /api/hris/employees/[id]
 * dan GET /api/hris/employees sebelumnya tanpa cek izin, dan PUT menyebar
 * body mentah ke UPDATE (user_id, is_access_app ikut bisa diubah).
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

const requireIamMenuPrefix = vi.fn();
const requireIamAction = vi.fn();
const requireApiUser = vi.fn();
const loadGrantedMenuCodesForUser = vi.fn();
const queryOne = vi.fn();
const updateCalls: Record<string, unknown>[] = [];
const selectCalls: string[] = [];

vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return {
    ...actual,
    requireIamMenuPrefix: (...args: unknown[]) => requireIamMenuPrefix(...args),
    requireIamAction: (...args: unknown[]) => requireIamAction(...args),
    requireApiUser: (...args: unknown[]) => requireApiUser(...args),
  };
});

vi.mock("@/lib/iam/has-menu", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/iam/has-menu")>();
  return {
    ...actual,
    loadGrantedMenuCodesForUser: (...args: unknown[]) => loadGrantedMenuCodesForUser(...args),
  };
});

vi.mock("@/lib/db", () => ({ queryOne: (...args: unknown[]) => queryOne(...args) }));

/** Builder PostgREST-like minimal: setiap metode chain, hasil di single()/await. */
function fakeQuery(table: string) {
  const row = { id: "emp-1", full_name: "Budi", reporting_to: null, employment_status: "permanent" };
  const builder: Record<string, unknown> = {};
  const chain = () => builder;
  Object.assign(builder, {
    select: (cols: string) => {
      selectCalls.push(`${table}:${cols}`);
      return builder;
    },
    update: (payload: Record<string, unknown>) => {
      updateCalls.push(payload);
      return builder;
    },
    insert: chain,
    eq: chain,
    neq: chain,
    or: chain,
    order: chain,
    range: chain,
    single: async () => ({ data: row, error: null }),
    then: (resolve: (v: unknown) => unknown) => resolve({ data: [row], error: null, count: 1 }),
  });
  return builder;
}

vi.mock("@/lib/pg/create-client", () => ({
  createPgClient: () => ({ from: (table: string) => fakeQuery(table) }),
}));

const detailRoute = () => import("@/app/api/hris/employees/[id]/route");
const listRoute = () => import("@/app/api/hris/employees/route");
const params = { params: Promise.resolve({ id: "emp-1" }) };
const req = (url: string, init?: RequestInit) => {
  const r = new Request(url, init) as Request & { nextUrl: URL };
  r.nextUrl = new URL(url);
  return r as never;
};

beforeEach(() => {
  updateCalls.length = 0;
  selectCalls.length = 0;
  requireIamMenuPrefix.mockReset();
  requireIamAction.mockReset();
  requireApiUser.mockReset();
  loadGrantedMenuCodesForUser.mockReset();
  queryOne.mockReset();
});

describe("PUT /api/hris/employees/[id]", () => {
  it("403 tanpa menu kepegawaian dan tidak menulis apa pun", async () => {
    const { ApiError } = await import("@/lib/api/auth");
    requireIamMenuPrefix.mockRejectedValue(ApiError.forbidden());
    const { PUT } = await detailRoute();
    const res = await PUT(req("http://x/api/hris/employees/emp-1", { method: "PUT", body: "{}" }), params);
    expect(res.status).toBe(403);
    expect(updateCalls).toHaveLength(0);
  });

  it("membuang user_id, is_access_app dan kolom di luar allowlist", async () => {
    requireIamMenuPrefix.mockResolvedValue({ id: "hr-1", role: "hrd" });
    const { PUT } = await detailRoute();
    const res = await PUT(
      req("http://x/api/hris/employees/emp-1", {
        method: "PUT",
        body: JSON.stringify({
          full_name: "Budi Baru",
          bank_account: "123",
          user_id: "akun-penyerang",
          is_access_app: true,
          old_staff_id: "x",
        }),
      }),
      params
    );
    expect(res.status).toBe(200);
    const payload = updateCalls[0];
    expect(payload.full_name).toBe("Budi Baru");
    expect(payload.bank_account).toBe("123");
    expect("user_id" in payload).toBe(false);
    expect("is_access_app" in payload).toBe(false);
    expect("old_staff_id" in payload).toBe(false);
    expect("phone" in payload).toBe(false);
  });
});

describe("DELETE /api/hris/employees/[id]", () => {
  it("401 tanpa sesi", async () => {
    const { ApiError } = await import("@/lib/api/auth");
    requireIamAction.mockRejectedValue(ApiError.unauthorized());
    const { DELETE } = await detailRoute();
    const res = await DELETE(req("http://x?reason=Karyawan%20keluar", { method: "DELETE" }), params);
    expect(res.status).toBe(401);
    expect(requireIamAction).toHaveBeenCalledWith(["hris.kepegawaian.users"], "delete");
    expect(updateCalls).toHaveLength(0);
  });

  it("400 tanpa alasan hapus", async () => {
    requireIamAction.mockResolvedValue({ id: "u-1", full_name: "HR", role: "hrd", brand_id: null });
    const { DELETE } = await detailRoute();
    const res = await DELETE(req("http://x", { method: "DELETE" }), params);
    expect(res.status).toBe(400);
    expect(updateCalls).toHaveLength(0);
  });
});

describe("GET /api/hris/employees/[id] (ESS)", () => {
  it("karyawan boleh membaca record miliknya sendiri", async () => {
    requireApiUser.mockResolvedValue({ id: "u-1", role: "employee" });
    loadGrantedMenuCodesForUser.mockResolvedValue(["ess.home", "hris.performance.review"]);
    queryOne.mockResolvedValue({ id: "emp-1" });
    const { GET } = await detailRoute();
    const res = await GET(req("http://x/api/hris/employees/emp-1"), params);
    expect(res.status).toBe(200);
  });

  it("karyawan lain → 403, meski punya grant hris.performance", async () => {
    requireApiUser.mockResolvedValue({ id: "u-2", role: "employee" });
    loadGrantedMenuCodesForUser.mockResolvedValue(["hris.performance.review"]);
    queryOne.mockResolvedValue({ id: "emp-2" });
    const { GET } = await detailRoute();
    const res = await GET(req("http://x/api/hris/employees/emp-1"), params);
    expect(res.status).toBe(403);
    expect(selectCalls).toHaveLength(0);
  });
});

describe("GET /api/hris/employees", () => {
  it("403 tanpa menu direktori karyawan", async () => {
    const { ApiError } = await import("@/lib/api/auth");
    requireIamMenuPrefix.mockRejectedValue(ApiError.forbidden());
    const { GET } = await listRoute();
    const res = await GET(req("http://x/api/hris/employees"));
    expect(res.status).toBe(403);
    expect(selectCalls).toHaveLength(0);
  });

  it("tidak memilih kolom pribadi (rekening, NPWP, KTP)", async () => {
    requireIamMenuPrefix.mockResolvedValue({ id: "hr-1", role: "hrd" });
    const { GET } = await listRoute();
    const res = await GET(req("http://x/api/hris/employees?sort_by=bank_account"));
    expect(res.status).toBe(200);
    const cols = selectCalls.find((c) => c.startsWith("employees:")) ?? "";
    expect(cols).not.toMatch(/\*|bank_account|npwp|ktp|bpjs|address|emergency/);
  });
});
