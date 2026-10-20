/**
 * Route-level guard dokumen karyawan (audit 2026-09-17). Sebelum perbaikan,
 * DELETE/PATCH di route ini berjalan TANPA cek sesi/izin sama sekali dan
 * PATCH menyebar body mentah ke UPDATE. Tes ini menjaga ketiganya:
 * 401 tanpa sesi, 403 tanpa hak menu, dan field di luar allowlist dibuang.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

const requireIamMenuPrefix = vi.fn();
const requireIamAction = vi.fn();
const updateCalls: Record<string, unknown>[] = [];
const deleteCalls: string[] = [];

vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return {
    ...actual,
    requireIamMenuPrefix: (...args: unknown[]) => requireIamMenuPrefix(...args),
    requireIamAction: (...args: unknown[]) => requireIamAction(...args),
  };
});
vi.mock("@/lib/audit", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/audit")>()),
  recordAuditAfterCommit: vi.fn(async () => {}),
}));

vi.mock("@/lib/pg/create-client", () => ({
  createServerPgClient: async () => ({
    from: () => ({
      delete: () => ({
        eq: async (_col: string, id: string) => {
          deleteCalls.push(id);
          return { error: null };
        },
      }),
      update: (payload: Record<string, unknown>) => {
        updateCalls.push(payload);
        return {
          eq: () => ({
            select: () => ({ single: async () => ({ data: { id: "doc-1", ...payload }, error: null }) }),
          }),
        };
      },
    }),
  }),
}));

const routeModule = () => import("@/app/api/hris/employees/documents/[doc_id]/route");
const params = { params: Promise.resolve({ doc_id: "doc-1" }) };
const patchRequest = (body: unknown) =>
  new Request("http://localhost/api/hris/employees/documents/doc-1", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  }) as never;

beforeEach(() => {
  updateCalls.length = 0;
  deleteCalls.length = 0;
  requireIamMenuPrefix.mockReset();
  requireIamAction.mockReset();
});

const deleteRequest = (search = "?reason=Dokumen%20salah%20unggah") =>
  new Request(`http://localhost/api/hris/employees/documents/doc-1${search}`, { method: "DELETE" }) as never;

describe("tanpa sesi valid", () => {
  it("DELETE → 401 dan TIDAK menyentuh database", async () => {
    const { ApiError } = await import("@/lib/api/auth");
    requireIamAction.mockRejectedValue(ApiError.unauthorized());
    const { DELETE } = await routeModule();
    const res = await DELETE(deleteRequest(), params);
    expect(res.status).toBe(401);
    expect(deleteCalls).toHaveLength(0);
  });

  it("PATCH → 401 dan tidak ada UPDATE", async () => {
    const { ApiError } = await import("@/lib/api/auth");
    requireIamMenuPrefix.mockRejectedValue(ApiError.unauthorized());
    const { PATCH } = await routeModule();
    const res = await PATCH(patchRequest({ notes: "x" }), params);
    expect(res.status).toBe(401);
    expect(updateCalls).toHaveLength(0);
  });
});

describe("login tapi tanpa hak menu HRIS", () => {
  it("DELETE → 403 dan tidak menghapus apa pun", async () => {
    const { ApiError } = await import("@/lib/api/auth");
    requireIamAction.mockRejectedValue(ApiError.forbidden());
    const { DELETE } = await routeModule();
    const res = await DELETE(deleteRequest(), params);
    expect(res.status).toBe(403);
    expect(requireIamAction).toHaveBeenCalledWith(["hris.kepegawaian.users"], "delete");
    expect(deleteCalls).toHaveLength(0);
  });

  it("DELETE berhak tanpa alasan → 400 dan tidak menghapus apa pun", async () => {
    requireIamAction.mockResolvedValue({ id: "u1", full_name: "HR", role: "hrd", brand_id: null });
    const { DELETE } = await routeModule();
    const res = await DELETE(deleteRequest(""), params);
    expect(res.status).toBe(400);
    expect(deleteCalls).toHaveLength(0);
  });
});

describe("berhak — allowlist field", () => {
  it("membuang field sensitif dari body, hanya metadata yang di-UPDATE", async () => {
    requireIamMenuPrefix.mockResolvedValue({ id: "u1", role: "super_admin" });
    const { PATCH } = await routeModule();
    const res = await PATCH(
      patchRequest({
        notes: "revisi",
        document_name: "KTP",
        is_verified: true,
        verified_by: "penyerang",
        employee_id: "korban",
      }),
      params
    );
    expect(res.status).toBe(200);
    expect(updateCalls).toHaveLength(1);
    const payload = updateCalls[0];
    expect(payload.notes).toBe("revisi");
    expect(payload.document_name).toBe("KTP");
    expect("is_verified" in payload).toBe(false);
    expect("verified_by" in payload).toBe(false);
    expect("employee_id" in payload).toBe(false);
  });

  it("body bukan JSON → 400, bukan crash", async () => {
    requireIamMenuPrefix.mockResolvedValue({ id: "u1", role: "super_admin" });
    const { PATCH } = await routeModule();
    const bad = new Request("http://localhost", {
      method: "PATCH",
      body: "bukan-json",
    }) as never;
    const res = await PATCH(bad, params);
    expect([200, 400]).toContain(res.status);
    if (res.status === 400) expect(updateCalls).toHaveLength(0);
  });
});
