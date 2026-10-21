/**
 * Kolom aktor HRIS menyimpan id karyawan yang benar dan gerbang tulis
 * payroll-settings: reviewed_by task departemen, created_by siklus feedback,
 * role tulis pengaturan payroll, dan insert karyawan saat promote kandidat.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

const requireIamMenuPrefix = vi.fn();
const getWorkforceActor = vi.fn();
const queryOne = vi.fn();
const loadPayrollSettings = vi.fn();
const savePayrollSettings = vi.fn();
const rpc = vi.fn();

vi.mock("@/lib/api/auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/auth")>();
  return { ...actual, requireIamMenuPrefix: (...args: unknown[]) => requireIamMenuPrefix(...args) };
});
vi.mock("@/lib/hris/workforce-auth", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/hris/workforce-auth")>();
  return { ...actual, getWorkforceActor: () => getWorkforceActor() };
});
vi.mock("@/lib/db", () => ({
  query: async () => [],
  queryOne: (...args: unknown[]) => queryOne(...args),
}));
vi.mock("@/lib/payroll/settings", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/payroll/settings")>();
  return {
    ...actual,
    loadPayrollSettings: (...args: unknown[]) => loadPayrollSettings(...args),
    savePayrollSettings: (...args: unknown[]) => savePayrollSettings(...args),
  };
});
vi.mock("@/lib/hris/create-contract", () => ({
  createDraftContract: async () => ({ ok: true, contract: { contract_number: "C-1" } }),
}));

interface Call {
  table: string;
  op: "select" | "insert" | "update" | "delete";
  payload?: unknown;
  filters: Record<string, unknown>;
}
const calls: Call[] = [];
let resolveRow: (call: Call) => unknown = () => null;
let authUser: { id: string; email: string } | null = null;

/** Builder pg tiruan: catat operasi + filter eq, hasil dari resolveRow. */
function fakeTable(table: string) {
  const call: Call = { table, op: "select", filters: {} };
  const result = async () => {
    calls.push(call);
    return { data: resolveRow(call) ?? null, error: null };
  };
  const builder: Record<string, unknown> = {
    single: result,
    maybeSingle: result,
    then: (ok: (v: unknown) => unknown, fail: (e: unknown) => unknown) => result().then(ok, fail),
  };
  for (const method of ["select", "order", "limit", "ilike"]) builder[method] = () => builder;
  builder.eq = (column: string, value: unknown) => {
    call.filters[column] = value;
    return builder;
  };
  for (const op of ["insert", "update", "delete"] as const) {
    builder[op] = (payload?: unknown) => {
      call.op = op;
      call.payload = payload;
      return builder;
    };
  }
  return builder;
}
const fakeDb = {
  from: fakeTable,
  rpc: (...args: unknown[]) => rpc(...args),
  auth: { getUser: async () => ({ data: { user: authUser } }) },
};
vi.mock("@/lib/pg/create-client", () => ({
  createPgClient: () => fakeDb,
  createServerPgClient: async () => fakeDb,
}));

const OCC_ID = "00000000-0000-0000-0000-0000000000a1";
const EMP_ID = "00000000-0000-0000-0000-0000000000e1";
const CAND_ID = "00000000-0000-0000-0000-0000000000c1";
const DEPT_ID = "00000000-0000-0000-0000-0000000000d1";
const BOSS_ID = "00000000-0000-0000-0000-0000000000b1";

function req(method: string, body?: unknown) {
  const url = "http://x/api";
  const request = new Request(url, {
    method,
    ...(body === undefined
      ? {}
      : { body: JSON.stringify(body), headers: { "Content-Type": "application/json" } }),
  }) as Request & { nextUrl: URL };
  request.nextUrl = new URL(url);
  return request as never;
}
const params = (id: string) => ({ params: Promise.resolve({ id }) });

async function expectStatus(res: Response, status: number, error?: string) {
  expect(res.status).toBe(status);
  if (error) expect((await res.json()).error).toBe(error);
}

beforeEach(() => {
  calls.length = 0;
  resolveRow = () => null;
  authUser = null;
  requireIamMenuPrefix.mockReset().mockResolvedValue({ id: "u-hr", full_name: "HR", role: "hrd", brand_id: null });
  getWorkforceActor.mockReset();
  queryOne.mockReset().mockResolvedValue(null);
  loadPayrollSettings.mockReset().mockResolvedValue({ settings: null });
  savePayrollSettings.mockReset().mockResolvedValue({ settings: {} });
  rpc.mockReset();
});

describe("review task departemen: reviewed_by = id karyawan", () => {
  const doneOcc = { id: OCC_ID, status: "done", task_id: "t-1", department_id: DEPT_ID, assignee_employee_id: null };

  it("HR tanpa record karyawan → 403, tidak ada UPDATE", async () => {
    getWorkforceActor.mockResolvedValue({ userId: "u-hr", role: "hrd", employeeId: null, isHr: true, isHrd: true });
    queryOne.mockResolvedValueOnce(doneOcc);
    const { PATCH } = await import("@/app/api/hris/dept-tasks/occurrences/[id]/route");
    const res = await PATCH(req("PATCH", { action: "approve" }), params(OCC_ID));
    await expectStatus(res, 403, "Akun ini tidak terhubung ke data karyawan, tidak bisa mereview task");
    expect(queryOne).toHaveBeenCalledTimes(1);
  });

  it("HR dengan record karyawan menulis employeeId, bukan userId", async () => {
    getWorkforceActor.mockResolvedValue({ userId: "u-hr", role: "hrd", employeeId: EMP_ID, isHr: true, isHrd: true });
    queryOne.mockResolvedValueOnce(doneOcc).mockResolvedValueOnce({ full_name: "Rina" });
    const { PATCH } = await import("@/app/api/hris/dept-tasks/occurrences/[id]/route");
    const res = await PATCH(req("PATCH", { action: "approve" }), params(OCC_ID));
    await expectStatus(res, 200);
    const [sql, values] = queryOne.mock.calls.at(-1) as [string, unknown[]];
    expect(sql).toContain("reviewed_by = $4");
    expect(values[3]).toBe(EMP_ID);
    expect(values).not.toContain("u-hr");
  });
});

describe("POST /api/hris/feedback-cycles: created_by", () => {
  it("akun tanpa karyawan tertaut → 403, tanpa fallback NIP dan tanpa insert", async () => {
    authUser = { id: "u-admin", email: "admin@x.id" };
    const { POST } = await import("@/app/api/hris/feedback-cycles/route");
    const res = await POST(req("POST", { name: "Q3" }));
    await expectStatus(res, 403, "Akun ini tidak terhubung ke data karyawan, tidak bisa membuat siklus feedback");
    expect(calls.some((c) => c.op === "insert")).toBe(false);
    expect(calls.filter((c) => c.table === "employees").map((c) => c.filters)).toEqual([
      { user_id: "u-admin" },
      { email: "admin@x.id" },
    ]);
  });

  it("karyawan tertaut lewat user_id jadi created_by", async () => {
    authUser = { id: "u-1", email: "rina@x.id" };
    resolveRow = (c) =>
      c.table === "employees" && c.filters.user_id === "u-1"
        ? { id: EMP_ID }
        : c.table === "feedback_cycles"
          ? { id: "cy-1" }
          : null;
    const { POST } = await import("@/app/api/hris/feedback-cycles/route");
    await expectStatus(await POST(req("POST", { name: "Q3" })), 201);
    const insert = calls.find((c) => c.op === "insert");
    expect(insert?.payload).toMatchObject({ name: "Q3", created_by: EMP_ID });
  });
});

describe("payroll-settings: tulis khusus super_admin/HRD", () => {
  const financeStaff = { id: "u-fin", full_name: "Fin", role: "finance_staff", brand_id: null };

  it("finance_staff dengan grant menu tetap bisa GET", async () => {
    requireIamMenuPrefix.mockResolvedValue(financeStaff);
    const { GET } = await import("@/app/api/hris/payroll-settings/route");
    await expectStatus(await GET(req("GET")), 200);
    expect(loadPayrollSettings).toHaveBeenCalledOnce();
  });

  it("finance_staff PUT → 403 tanpa menyimpan", async () => {
    requireIamMenuPrefix.mockResolvedValue(financeStaff);
    const { PUT } = await import("@/app/api/hris/payroll-settings/route");
    const res = await PUT(req("PUT", { settings: {} }));
    await expectStatus(res, 403, "Hanya super admin dan HRD yang boleh mengubah pengaturan payroll");
    expect(savePayrollSettings).not.toHaveBeenCalled();
  });

  it("HRD PUT tersimpan; gerbang menu tetap dicek", async () => {
    const { PUT } = await import("@/app/api/hris/payroll-settings/route");
    await expectStatus(await PUT(req("PUT", { settings: {} })), 200);
    expect(requireIamMenuPrefix).toHaveBeenCalledOnce();
    expect(savePayrollSettings).toHaveBeenCalledOnce();
  });
});

describe("POST /api/hris/promote", () => {
  it("memanggil fungsi DB promosi (NIP, jabatan, atasan, tautan kandidat dalam satu transaksi)", async () => {
    resolveRow = (c) => {
      if (c.table === "candidates" && c.op === "select") {
        return { id: CAND_ID, full_name: "Budi", email: "budi@x.id", phone: null, status: "hired", position: { id: "pos-1" } };
      }
      if (c.table === "employees" && c.filters.id === EMP_ID) return { id: EMP_ID, nip: "EMP-2026-00001" };
      return null;
    };
    queryOne.mockImplementation(async (sql: string) =>
      sql.includes("promote_candidate_to_employee") ? { id: EMP_ID } : null
    );
    const { POST } = await import("@/app/api/hris/promote/route");
    const res = await POST(
      req("POST", { candidate_id: CAND_ID, join_date: "2026-10-01", department_id: DEPT_ID, reporting_to: BOSS_ID })
    );
    await expectStatus(res, 200);
    const [sql, values] = queryOne.mock.calls.find(([q]) => String(q).includes("promote_candidate_to_employee")) as [
      string,
      unknown[],
    ];
    expect(sql).toContain("public.promote_candidate_to_employee($1, $2::date, $3, $4, $5)");
    expect(values).toEqual([CAND_ID, "2026-10-01", "probation", DEPT_ID, BOSS_ID]);
    // Insert & tautan kandidat terjadi di dalam fungsi DB, bukan lewat shim.
    expect(calls.some((c) => c.table === "employees" && c.op === "insert")).toBe(false);
    expect(calls.some((c) => c.table === "candidates" && c.op === "update")).toBe(false);
  });

  it("promosi ganda serentak (unique_violation dari fungsi) → 409", async () => {
    resolveRow = (c) =>
      c.table === "candidates" && c.op === "select"
        ? { id: CAND_ID, full_name: "Budi", email: "budi@x.id", phone: null, status: "hired", position: { id: "pos-1" } }
        : null;
    queryOne.mockImplementation(async (sql: string) => {
      if (sql.includes("promote_candidate_to_employee")) throw Object.assign(new Error("dup"), { code: "23505" });
      return null;
    });
    const { POST } = await import("@/app/api/hris/promote/route");
    await expectStatus(await POST(req("POST", { candidate_id: CAND_ID })), 409);
  });
});
