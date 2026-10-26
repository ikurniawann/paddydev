import { beforeEach, describe, expect, it, vi } from "vitest";

/** EPIC-067 R11/R15: fallback Next memutus sebagai HR hanya dengan "approve" pada daun. */
const iam = vi.hoisted(() => ({ approves: [] as string[], calls: [] as unknown[] }));
vi.mock("@/lib/iam/has-menu", () => ({
  userHasIamAction: vi.fn(async (userId: string, role: string, prefixes: readonly string[], action: string) => {
    iam.calls.push([userId, role, prefixes, action]);
    return action === "approve" && prefixes.some((p) => iam.approves.includes(p));
  }),
}));
const row = vi.hoisted(() => ({ current: null as Record<string, unknown> | null }));
vi.mock("@/lib/pg/create-client", () => {
  const client = {
    from: () => ({ select: () => ({ eq: () => ({ maybeSingle: async () => ({ data: row.current }) }) }) }),
  };
  return { createServerPgClient: async () => client, createPgClient: () => client };
});

import { canApproveLeaf, HRIS_APPROVE_LEAF } from "./approve-access";
import { decideLeave } from "./leaves-repo";
import { decideOvertime } from "./overtime-repo";
import type { WorkforceActor } from "./workforce-auth";

const ID = "00000000-0000-4000-8000-000000000001";
const NOT_DECIDER = "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini";

function actor(role: string, employeeId: string): WorkforceActor {
  const hrd = ["super_admin", "admin", "hrd"].includes(role);
  return { userId: `u-${employeeId}`, role, employeeId, isHr: hrd, isHrd: hrd, fullName: "Akun Uji" };
}

beforeEach(() => {
  iam.approves = [];
  iam.calls = [];
});

describe("canApproveLeaf", () => {
  it("memeriksa aksi approve pada daun", async () => {
    iam.approves = [HRIS_APPROVE_LEAF.cuti];
    expect(await canApproveLeaf(actor("pos", "e1"), HRIS_APPROVE_LEAF.cuti)).toBe(true);
    expect(await canApproveLeaf(actor("pos", "e1"), HRIS_APPROVE_LEAF.lembur)).toBe(false);
    expect(iam.calls).toEqual([
      ["u-e1", "pos", ["hris.workforce.leaves"], "approve"],
      ["u-e1", "pos", ["hris.kepegawaian.overtime"], "approve"],
    ]);
  });
});

describe("decideLeave — jalur HR", () => {
  const leave = (status: string, employeeId = "staff") => ({
    id: ID, employee_id: employeeId, status, employee: { id: employeeId, reporting_to: "boss" },
  });

  it("hrd tanpa approve pada Cuti ditolak", async () => {
    row.current = leave("pending");
    await expect(decideLeave(actor("hrd", "hr"), { leave_id: ID, action: "approve" })).rejects.toMatchObject({
      status: 403, message: NOT_DECIDER,
    });
  });

  it("role non-HR dengan approve pada Cuti lolos sebagai HR; tetap tidak memutus miliknya", async () => {
    iam.approves = [HRIS_APPROVE_LEAF.cuti];
    // Lolos kapasitas → pemeriksaan berikutnya (status) yang menjawab.
    row.current = leave("approved");
    await expect(decideLeave(actor("pos", "admin"), { leave_id: ID, action: "approve" })).rejects.toMatchObject({
      status: 400, message: "Leave request already approved",
    });
    row.current = leave("pending", "admin");
    await expect(decideLeave(actor("pos", "admin"), { leave_id: ID, action: "approve" })).rejects.toMatchObject({
      status: 403, message: "Tidak bisa memutuskan pengajuan cuti sendiri",
    });
  });

  it("atasan langsung tanpa menu tetap lolos", async () => {
    row.current = leave("approved");
    await expect(decideLeave(actor("pos", "boss"), { leave_id: ID, action: "approve" })).rejects.toMatchObject({
      status: 400,
    });
  });
});

describe("decideOvertime — jalur HR", () => {
  const overtime = (employeeId = "staff") => ({
    id: ID, employee_id: employeeId, requested_by: employeeId, source: "employee", status: "pending",
    employee: { id: employeeId, reporting_to: "boss" },
  });

  it("hrd tanpa approve pada Lembur ditolak", async () => {
    row.current = overtime();
    await expect(decideOvertime(actor("hrd", "hr"), { overtime_id: ID, action: "approve" })).rejects.toMatchObject({
      status: 403, message: NOT_DECIDER,
    });
  });

  it("role non-HR dengan approve pada Lembur lolos sebagai HR; tetap tidak memutus miliknya", async () => {
    iam.approves = [HRIS_APPROVE_LEAF.lembur];
    row.current = overtime();
    // Lolos canDecideOvertime → validasi alasan penolakan yang menjawab.
    await expect(decideOvertime(actor("pos", "admin"), { overtime_id: ID, action: "reject" })).rejects.toMatchObject({
      status: 400, message: "Alasan penolakan wajib diisi",
    });
    row.current = overtime("admin");
    await expect(decideOvertime(actor("pos", "admin"), { overtime_id: ID, action: "approve" })).rejects.toMatchObject({
      status: 403, message: "Tidak bisa memutuskan pengajuan lembur sendiri",
    });
  });

  it("approve pada Cuti tidak memberi jalur HR Lembur; atasan langsung tetap lolos", async () => {
    iam.approves = [HRIS_APPROVE_LEAF.cuti];
    row.current = overtime();
    await expect(decideOvertime(actor("pos", "admin"), { overtime_id: ID, action: "approve" })).rejects.toMatchObject({
      status: 403, message: NOT_DECIDER,
    });
    await expect(decideOvertime(actor("pos", "boss"), { overtime_id: ID, action: "reject" })).rejects.toMatchObject({
      status: 400, message: "Alasan penolakan wajib diisi",
    });
  });
});
