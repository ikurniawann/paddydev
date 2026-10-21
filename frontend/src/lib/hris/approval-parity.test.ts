/**
 * Paritas fallback Next untuk paket Approval atasan (EPIC-063 Task 7): keputusan
 * cuti/lembur (manager lebih dulu, HRD cadangan, bukan diri sendiri), lembur
 * sebelum dimulai, antrean bawahan tanpa diri sendiri, WA cuti, badge Persetujuan Tim.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

const recordAudit = vi.fn();
const recordAuditAfterCommit = vi.fn();
const txQueries: { sql: string; params: unknown[] }[] = [];
let txRowCount = 1;
const query = vi.fn();
const queryOne = vi.fn();

vi.mock("@/lib/audit", () => ({
  recordAudit: (...args: unknown[]) => recordAudit(...args),
  recordAuditAfterCommit: (...args: unknown[]) => recordAuditAfterCommit(...args),
}));
vi.mock("@/lib/db", () => ({
  query: (...args: unknown[]) => query(...args),
  queryOne: (...args: unknown[]) => queryOne(...args),
  getPool: () => ({ query: vi.fn() }),
  withTransaction: async (fn: (client: unknown) => Promise<unknown>) =>
    fn({
      query: async (sql: string, params: unknown[]) => {
        txQueries.push({ sql, params });
        return { rowCount: txRowCount, rows: [] };
      },
    }),
}));

interface Call {
  table: string;
  op: string;
  payload?: unknown;
  filters: [string, unknown, unknown?][];
}
const calls: Call[] = [];
const tables: Record<string, unknown> = {};

function fakeTable(table: string) {
  const call: Call = { table, op: "select", filters: [] };
  const done = async () => {
    calls.push(call);
    return { data: tables[table] ?? null, error: null };
  };
  const builder: Record<string, unknown> = {
    single: done,
    maybeSingle: done,
    then: (ok: (v: unknown) => unknown, fail: (e: unknown) => unknown) => done().then(ok, fail),
  };
  for (const m of ["select", "order", "limit", "gte", "lte", "range", "or"]) builder[m] = () => builder;
  for (const m of ["eq", "neq", "in"]) {
    builder[m] = (col: string, val: unknown) => {
      call.filters.push([m, col, val]);
      return builder;
    };
  }
  for (const op of ["insert", "update", "delete"]) {
    builder[op] = (payload?: unknown) => {
      call.op = op;
      call.payload = payload;
      return builder;
    };
  }
  return builder;
}
vi.mock("@/lib/pg/create-client", () => ({
  createServerPgClient: async () => ({ from: (t: string) => fakeTable(t) }),
  createPgClient: () => ({ from: (t: string) => fakeTable(t) }),
}));

import { decideLeave } from "./leaves-repo";
import { buildLeaveRequestWaMessage, notifyLeaveRequestWa } from "./leave-wa";
import { buildNavBadges, TEAM_APPROVALS_HREF } from "./nav-badges";
import { createOvertime, decideOvertime, listOvertime } from "./overtime-repo";
import { OVERTIME_TOO_LATE } from "./overtime-rules";
import type { WorkforceActor } from "./workforce-auth";

const EMP = "00000000-0000-4000-8000-000000000001";
const MGR = "00000000-0000-4000-8000-000000000002";
const HRD = "00000000-0000-4000-8000-000000000003";
const OTHER = "00000000-0000-4000-8000-000000000004";
const LEAVE_ID = "11111111-1111-4111-8111-111111111111";

const actor = (over: Partial<WorkforceActor>): WorkforceActor => ({
  userId: "u-x",
  role: "employee",
  employeeId: OTHER,
  isHr: false,
  isHrd: false,
  fullName: "Nama Akun",
  ...over,
});
const manager = actor({ userId: "u-mgr", employeeId: MGR });
const hrd = actor({ userId: "u-hrd", role: "hrd", employeeId: HRD, isHr: true, isHrd: true });
const hiringManager = actor({ userId: "u-hm", role: "hiring_manager", employeeId: OTHER, isHr: true, isHrd: false });

async function rejects(p: Promise<unknown>) {
  try {
    await p;
  } catch (e) {
    return e as { status: number; message: string };
  }
  throw new Error("diharapkan ditolak");
}

beforeEach(() => {
  calls.length = 0;
  txQueries.length = 0;
  txRowCount = 1;
  for (const k of Object.keys(tables)) delete tables[k];
  recordAudit.mockReset();
  recordAuditAfterCommit.mockReset();
  query.mockReset();
  queryOne.mockReset();
});

describe("decideLeave — Q-APV-3 (EPIC-063 R1/R2)", () => {
  const leave = (over: Record<string, unknown> = {}) => ({
    id: LEAVE_ID,
    employee_id: EMP,
    status: "pending",
    leave_type: "annual",
    start_date: "2026-11-02",
    end_date: "2026-11-03",
    total_days: 2,
    employee: { id: EMP, full_name: "Adi", phone: null, reporting_to: MGR },
    ...over,
  });
  const approve = { leave_id: LEAVE_ID, action: "approve" as const };

  it("atasan langsung memutus sebagai manager, nama akun & audit tersimpan", async () => {
    tables.leaves = leave();
    await decideLeave(manager, approve);
    const update = txQueries.find((q) => q.sql.includes("UPDATE hris.leaves"));
    expect(update?.sql).toContain("decided_as");
    expect(update?.params).toEqual([LEAVE_ID, "approved", MGR, null, "manager", "Nama Akun"]);
    expect(recordAudit).toHaveBeenCalledTimes(1);
    const entry = recordAudit.mock.calls[0][1];
    expect(entry).toMatchObject({
      action: "leave.approve",
      entity: "hris.leaves",
      entityId: LEAVE_ID,
      actor: { id: "u-mgr", name: "Nama Akun" },
      before: { status: "pending" },
      after: { status: "approved", decided_as: "manager", rejection_reason: null },
    });
    // kuota tahunan terpotong di transaksi yang sama
    expect(txQueries.some((q) => q.sql.includes("leave_balances"))).toBe(true);
  });

  it("HRD bukan atasan memutus sebagai hr; nama kosong → HRD; reject mengaudit alasan", async () => {
    tables.leaves = leave();
    await decideLeave({ ...hrd, fullName: null }, { leave_id: LEAVE_ID, action: "reject", rejection_reason: "Bentrok" });
    const update = txQueries.find((q) => q.sql.includes("UPDATE hris.leaves"));
    expect(update?.params).toEqual([LEAVE_ID, "rejected", HRD, "Bentrok", "hr", "HRD"]);
    expect(recordAudit.mock.calls[0][1]).toMatchObject({
      action: "leave.reject",
      after: { status: "rejected", decided_as: "hr", rejection_reason: "Bentrok" },
    });
    expect(txQueries.some((q) => q.sql.includes("leave_balances"))).toBe(false);
  });

  it("HRD yang mengajukan sendiri tidak bisa memutus → 403", async () => {
    tables.leaves = leave({ employee_id: HRD, employee: { id: HRD, full_name: "H", reporting_to: MGR } });
    const err = await rejects(decideLeave(hrd, approve));
    expect(err.status).toBe(403);
    expect(err.message).toBe("Tidak bisa memutuskan pengajuan cuti sendiri");
    expect(txQueries).toEqual([]);
  });

  it("atasan yang reporting_to-nya menunjuk dirinya sendiri tidak memutus cutinya sendiri", async () => {
    tables.leaves = leave({ employee_id: MGR, employee: { id: MGR, full_name: "M", reporting_to: MGR } });
    const err = await rejects(decideLeave(manager, approve));
    expect(err.status).toBe(403);
    expect(err.message).toBe("Tidak bisa memutuskan pengajuan cuti sendiri");
  });

  it("hiring_manager (isHr tetapi bukan HRD) bukan cadangan → 403", async () => {
    tables.leaves = leave();
    const err = await rejects(decideLeave(hiringManager, approve));
    expect(err.status).toBe(403);
    expect(err.message).toBe("Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini");
  });

  it("diputus proses lain saat transaksi berjalan → 409", async () => {
    tables.leaves = leave();
    txRowCount = 0;
    const err = await rejects(decideLeave(manager, approve));
    expect(err.status).toBe(409);
    expect(err.message).toBe("Pengajuan sudah diproses oleh orang lain");
    expect(recordAudit).not.toHaveBeenCalled();
  });
});

describe("createOvertime — lembur sebelum dimulai (EPIC-063 R3)", () => {
  const base = { start_time: "18:00", end_time: "20:00", reason: "Stock opname" };
  const employee = actor({ employeeId: EMP });

  it("pengajuan sendiri dengan jam mulai yang sudah lewat → 400", async () => {
    tables.employees = { id: EMP, is_active: true };
    const err = await rejects(createOvertime(employee, { ...base, date: "2020-01-01" }));
    expect(err.status).toBe(400);
    expect(err.message).toBe(OVERTIME_TOO_LATE);
    expect(calls.some((c) => c.op === "insert")).toBe(false);
  });

  it("HRD yang mengajukan untuk dirinya sendiri pun tidak boleh lampau", async () => {
    tables.employees = { id: HRD, is_active: true };
    const err = await rejects(createOvertime(hrd, { ...base, date: "2020-01-01" }));
    expect(err.message).toBe(OVERTIME_TOO_LATE);
  });

  it("pengajuan sendiri tanggal depan diterima", async () => {
    tables.employees = { id: EMP, is_active: true };
    tables.overtime_requests = [];
    await createOvertime(employee, { ...base, date: "2099-01-01" });
    expect(calls.find((c) => c.op === "insert")?.payload).toMatchObject({ source: "employee", employee_id: EMP });
  });

  it("HRD menugaskan karyawan lain boleh tanggal lampau (R3)", async () => {
    tables.employees = { id: EMP, is_active: true };
    tables.overtime_requests = [];
    await createOvertime(hrd, { ...base, date: "2020-01-01", employee_id: EMP });
    expect(calls.find((c) => c.op === "insert")?.payload).toMatchObject({ source: "company", employee_id: EMP });
  });

  it("hiring_manager tidak bisa menugaskan orang lain (ruling C2) → 403", async () => {
    const err = await rejects(createOvertime(hiringManager, { ...base, date: "2099-01-01", employee_id: EMP }));
    expect(err.status).toBe(403);
    expect(err.message).toBe("Hanya HRD yang bisa membuat penugasan lembur untuk karyawan lain");
  });
});

describe("decideOvertime — kapasitas & audit (EPIC-063 R1)", () => {
  const request = (over: Record<string, unknown> = {}) => ({
    id: LEAVE_ID,
    status: "pending",
    source: "employee",
    date: "2026-11-02",
    employee_id: EMP,
    requested_by: EMP,
    employee: { id: EMP, full_name: "Adi", reporting_to: MGR },
    ...over,
  });
  const approve = { overtime_id: LEAVE_ID, action: "approve" as const };
  const updateOf = () => calls.find((c) => c.op === "update");

  it("atasan langsung: decided_as manager, nama akun, audit overtime.approve", async () => {
    tables.overtime_requests = request();
    await decideOvertime(manager, approve);
    expect(updateOf()?.payload).toMatchObject({
      status: "approved",
      decided_by: MGR,
      decided_as: "manager",
      decided_by_name: "Nama Akun",
    });
    expect(recordAuditAfterCommit).toHaveBeenCalledTimes(1);
    expect(recordAuditAfterCommit.mock.calls[0][0]).toMatchObject({
      action: "overtime.approve",
      entity: "hris.overtime_requests",
      entityId: LEAVE_ID,
      after: { status: "approved", decided_as: "manager", rejection_reason: null },
    });
  });

  it("HRD bukan atasan: hr; reject → overtime.reject", async () => {
    tables.overtime_requests = request();
    await decideOvertime({ ...hrd, fullName: "" }, { ...approve, action: "reject", rejection_reason: "Tidak perlu" });
    expect(updateOf()?.payload).toMatchObject({ status: "rejected", decided_as: "hr", decided_by_name: "HRD" });
    expect(recordAuditAfterCommit.mock.calls[0][0]).toMatchObject({ action: "overtime.reject" });
  });

  it("hiring_manager bukan cadangan HRD → 403, tidak menulis", async () => {
    tables.overtime_requests = request();
    const err = await rejects(decideOvertime(hiringManager, approve));
    expect(err.status).toBe(403);
    expect(err.message).toBe("Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini");
    expect(updateOf()).toBeUndefined();
  });

  it("HRD tidak memutus pengajuan lemburnya sendiri → 403", async () => {
    tables.overtime_requests = request({
      employee_id: HRD,
      requested_by: HRD,
      employee: { id: HRD, full_name: "H", reporting_to: MGR },
    });
    const err = await rejects(decideOvertime(hrd, approve));
    expect(err.status).toBe(403);
    expect(err.message).toBe("Tidak bisa memutuskan pengajuan lembur sendiri");
  });

  it("konfirmasi penugasan company oleh karyawan: tanpa decided_as dan tanpa audit", async () => {
    tables.overtime_requests = request({ source: "company", requested_by: HRD });
    await decideOvertime(actor({ employeeId: EMP }), approve);
    expect(updateOf()?.payload).not.toHaveProperty("decided_as");
    expect(updateOf()?.payload).not.toHaveProperty("decided_by_name");
    expect(recordAuditAfterCommit).not.toHaveBeenCalled();
  });
});

describe("listOvertime scope=approvals", () => {
  it("bawahan yang reporting_to-nya diri sendiri dikecualikan (EPIC-063 R2)", async () => {
    tables.employees = [{ id: EMP }];
    tables.overtime_requests = [];
    await listOvertime(manager, { scope: "approvals" });
    const subs = calls.find((c) => c.table === "employees");
    expect(subs?.filters).toContainEqual(["eq", "reporting_to", MGR]);
    expect(subs?.filters).toContainEqual(["neq", "id", MGR]);
  });
});

describe("WA cuti ke atasan", () => {
  it("footer menunjuk Persetujuan Tim", () => {
    const text = buildLeaveRequestWaMessage({
      leaveId: "x",
      employeeName: "Adi",
      leaveType: "annual",
      startDate: "2026-11-02",
      endDate: "2026-11-02",
      totalDays: 1,
    });
    expect(text).toContain("Mohon ditinjau di dashboard Arkiv OS (Area Karyawan → Persetujuan Tim).");
    expect(text).not.toContain("Kehadiran & Cuti");
  });

  it("atasan yang sama dengan pemohon dilewati lewat SQL", async () => {
    queryOne.mockResolvedValue(null);
    await notifyLeaveRequestWa({
      leaveId: "l1",
      employeeId: EMP,
      employeeName: "Adi",
      leaveType: "annual",
      startDate: "2026-11-02",
      endDate: "2026-11-02",
      totalDays: 1,
    });
    const sql = String(queryOne.mock.calls[0][0]);
    expect(sql).toContain("a.is_active = true AND a.id <> e.id");
  });
});

describe("buildNavBadges — Persetujuan Tim (ruling P7)", () => {
  it("karyawan mendapat badge antrean timnya: cuti + lembur + perjalanan dinas + kasbon tahap atasan", async () => {
    queryOne.mockImplementation(async (sql: string) => {
      if (sql.includes("manager_approver_id")) return { total: "5" };
      return { total: "0" };
    });
    const badges = await buildNavBadges(actor({ employeeId: MGR }));
    expect(TEAM_APPROVALS_HREF).toBe("/dashboard/me/persetujuan-tim");
    expect(badges[TEAM_APPROVALS_HREF]).toBe(5);
    const sql = String(queryOne.mock.calls.map((c) => c[0]).find((s: string) => s.includes("manager_approver_id")));
    expect(sql).toContain("hris.leaves");
    expect(sql).toContain("hris.overtime_requests");
    expect(sql).toContain("hris.business_trips");
    expect(sql).toContain("hris.loans");
    expect(sql).toContain("m.id <> $1"); // reporting_to diri sendiri bukan bawahan
    expect(sql).toContain("o.source = 'employee'");
  });

  it("akun tanpa data karyawan tidak mendapat badge tim", async () => {
    queryOne.mockResolvedValue({ total: "0" });
    const badges = await buildNavBadges(actor({ employeeId: null }));
    expect(badges).not.toHaveProperty(TEAM_APPROVALS_HREF);
  });
});
