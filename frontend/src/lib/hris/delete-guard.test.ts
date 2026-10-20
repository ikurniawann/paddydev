import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/auth";

const iam = vi.hoisted(() => ({ allowed: true, calls: [] as unknown[] }));
vi.mock("@/lib/api/auth", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/auth")>()),
  requireIamAction: vi.fn(async (prefixes: readonly string[], action: string) => {
    iam.calls.push([prefixes, action]);
    if (!iam.allowed) throw ApiError.forbidden("Insufficient permissions");
    return { id: "u1", full_name: "Rina HR", role: "hrd", brand_id: null };
  }),
}));
const audit = vi.hoisted(() => ({ recordAuditAfterCommit: vi.fn(async () => {}) }));
vi.mock("@/lib/audit", async (orig) => ({
  ...(await orig<typeof import("@/lib/audit")>()),
  recordAuditAfterCommit: audit.recordAuditAfterCommit,
}));

import { HRIS_LEAF, auditHrisDelete, requireHrisDelete } from "./delete-guard";
import { HRIS_DELETE_REASON_ERROR } from "./delete-reason";

describe("requireHrisDelete", () => {
  beforeEach(() => {
    iam.allowed = true;
    iam.calls = [];
    audit.recordAuditAfterCommit.mockClear();
  });

  it("memeriksa aksi delete pada menu daun", async () => {
    const out = await requireHrisDelete(new Request("http://x/api/hris/shifts/1?reason=Shift%20dobel", { method: "DELETE" }), HRIS_LEAF.shift);
    expect(iam.calls).toEqual([[["hris.kepegawaian.shifts"], "delete"]]);
    expect(out.reason).toBe("Shift dobel");
  });

  it("menolak tanpa hak hapus atau tanpa alasan", async () => {
    iam.allowed = false;
    await expect(requireHrisDelete(new Request("http://x/a?reason=Shift%20dobel"), HRIS_LEAF.shift)).rejects.toMatchObject({ status: 403 });
    iam.allowed = true;
    await expect(requireHrisDelete(new Request("http://x/a?reason=abcd"), HRIS_LEAF.shift)).rejects.toMatchObject({
      status: 400,
      message: HRIS_DELETE_REASON_ERROR,
    });
  });

  it("alasan dari body JSON", async () => {
    const req = new Request("http://x/a", { method: "DELETE", body: JSON.stringify({ reason: "Data ganda sistem" }) });
    expect((await requireHrisDelete(req, HRIS_LEAF.absensi)).reason).toBe("Data ganda sistem");
  });

  it("audit memakai entity.delete", async () => {
    const user = { id: "u1", full_name: "Rina HR", role: "hrd", brand_id: null } as const;
    await auditHrisDelete(new Request("http://x/a"), user, "hris.shifts", "s1", "Shift dobel");
    expect(audit.recordAuditAfterCommit).toHaveBeenCalledWith(
      expect.objectContaining({ action: "hris.shifts.delete", entity: "hris.shifts", entityId: "s1", reason: "Shift dobel", actor: { id: "u1", name: "Rina HR" } })
    );
  });

  it("audit hapus absensi memakai attendance.delete (sama dengan Go)", async () => {
    const user = { id: "u1", full_name: "Rina HR", role: "hrd", brand_id: null } as const;
    await auditHrisDelete(new Request("http://x/a"), user, "hris.attendance", "a1", "Data ganda sistem");
    expect(audit.recordAuditAfterCommit).toHaveBeenCalledWith(
      expect.objectContaining({ action: "attendance.delete", entity: "hris.attendance", entityId: "a1" })
    );
  });
});
