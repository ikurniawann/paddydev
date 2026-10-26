/**
 * Perilaku route HRIS setelah migrasi ke apiHandler + lib: validasi zod,
 * aturan status, dan scope aktor tetap memberi status & pesan lama.
 * Database diganti builder palsu per tabel (createServerPgClient) dan
 * queryOne/query tiruan (@/lib/db).
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/auth";

const requireIamMenuPrefix = vi.fn();
const requireIamGuard = vi.fn();
const requireApiUser = vi.fn();
const getWorkforceActor = vi.fn();
const getLogbookActor = vi.fn();
const queryOne = vi.fn();
const query = vi.fn();
const writes: { table: string; op: string; payload?: unknown }[] = [];
let tables: Record<string, unknown> = {};

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
// EPIC-067 R11/R15: jalur HR keputusan = aksi "approve" pada daun (IAM DB).
// Fixture HR memegang approve di ketiga daun; karyawan biasa tidak punya.
const HR_APPROVES = ["hris.workforce.attendance", "hris.workforce.leaves", "hris.kepegawaian.overtime"];
const approves: Record<string, string[]> = { "u-hr": HR_APPROVES };
vi.mock("@/lib/iam/has-menu", async (orig) => ({
  ...(await orig<typeof import("@/lib/iam/has-menu")>()),
  userHasIamAction: async (userId: string, _role: string, prefixes: readonly string[], action: string) =>
    action === "approve" && prefixes.some((p) => (approves[userId] ?? []).includes(p)),
}));
vi.mock("@/lib/db", () => ({
  query: (...args: unknown[]) => query(...args),
  queryOne: (...args: unknown[]) => queryOne(...args),
  withTransaction: () => {
    throw new Error("transaksi tidak diharapkan di tes ini");
  },
  getPool: () => {
    throw new Error("pool tidak diharapkan di tes ini");
  },
}));

/** Builder PostgREST-like: setiap tabel mengembalikan `tables[table]`. */
function fakeTable(table: string) {
  const result = () => ({ data: tables[table] ?? null, error: null, count: 0 });
  const builder: Record<string, unknown> = {};
  const chain = () => builder;
  for (const m of ["select", "eq", "neq", "in", "gt", "gte", "lte", "order", "limit", "range"]) {
    builder[m] = chain;
  }
  for (const op of ["insert", "update", "delete"]) {
    builder[op] = (payload?: unknown) => {
      writes.push({ table, op, payload });
      return builder;
    };
  }
  builder.single = async () => result();
  builder.maybeSingle = async () => result();
  builder.then = (resolve: (v: unknown) => unknown) => resolve(result());
  return builder;
}
const fakeDb = { from: (table: string) => fakeTable(table), rpc: async () => ({ data: null, error: null }) };
vi.mock("@/lib/pg/create-client", () => ({
  createPgClient: () => fakeDb,
  createServerPgClient: async () => fakeDb,
}));

const REASON = "?reason=Data%20ganda%20sistem";
const ID = "00000000-0000-0000-0000-000000000001";
const OTHER = "00000000-0000-0000-0000-000000000002";
const HR_USER = { id: "u-hr", full_name: "HRD", role: "hrd", brand_id: null };
const staff = { userId: "u-1", role: "employee", employeeId: ID, isHr: false, isHrd: false };
const hr = { userId: "u-hr", role: "hrd", employeeId: OTHER, isHr: true, isHrd: true };

function req(method: string, body?: unknown, search = "") {
  const url = `http://x/api${search}`;
  const request = new Request(url, {
    method,
    ...(body === undefined
      ? {}
      : { body: JSON.stringify(body), headers: { "Content-Type": "application/json" } }),
  }) as Request & { nextUrl: URL };
  request.nextUrl = new URL(url);
  return request as never;
}
const params = <T extends Record<string, unknown>>(p: T) => ({ params: Promise.resolve(p) }) as never;

async function expectError(res: Response, status: number, error: string | RegExp) {
  expect(res.status).toBe(status);
  const json = await res.json();
  expect(json.success).toBe(false);
  if (typeof error === "string") expect(json.error).toBe(error);
  else expect(json.error).toMatch(error);
}

beforeEach(() => {
  tables = {};
  writes.length = 0;
  requireIamMenuPrefix.mockReset().mockResolvedValue(HR_USER);
  requireIamGuard.mockReset().mockResolvedValue({ error: null, user: HR_USER });
  requireApiUser.mockReset().mockResolvedValue(HR_USER);
  getWorkforceActor.mockReset().mockResolvedValue(staff);
  getLogbookActor.mockReset().mockResolvedValue(null);
  queryOne.mockReset().mockResolvedValue(null);
  query.mockReset().mockResolvedValue([]);
});

describe("payroll", () => {
  it("POST periode tidak valid → 400 pesan lama, tanpa insert", async () => {
    const { POST } = await import("@/app/api/hris/payroll/route");
    const res = await POST(req("POST", { period_month: 13, period_year: 2026 }));
    await expectError(res, 400, "Bulan dan tahun periode wajib diisi dengan benar");
    expect(writes).toEqual([]);
  });

  it("POST periode yang sudah ada → 400", async () => {
    tables.payroll_runs = { id: "run-1" };
    const { POST } = await import("@/app/api/hris/payroll/route");
    const res = await POST(req("POST", { period_month: 6, period_year: 2026 }));
    await expectError(res, 400, "Payroll untuk periode ini sudah ada");
  });

  it("PUT transisi mundur ditolak", async () => {
    tables.payroll_runs = { id: "run-1", status: "completed", period_month: 6, period_year: 2026 };
    const { PUT } = await import("@/app/api/hris/payroll/[id]/route");
    const res = await PUT(req("PUT", { status: "draft" }), params({ id: "run-1" }));
    await expectError(res, 400, "Transisi status 'completed' → 'draft' tidak diizinkan");
    expect(writes).toEqual([]);
  });

  it("DELETE run paid ditolak; run tak ada → 404", async () => {
    const { DELETE } = await import("@/app/api/hris/payroll/[id]/route");
    await expectError(
      await DELETE(req("DELETE", undefined, REASON), params({ id: "x" })),
      404,
      "Payroll run tidak ditemukan"
    );
    tables.payroll_runs = { id: "run-1", status: "paid" };
    await expectError(
      await DELETE(req("DELETE", undefined, REASON), params({ id: "run-1" })),
      400,
      "Payroll yang sudah dibayar tidak bisa dihapus"
    );
    expect(writes).toEqual([]);
  });

  it("calculate hanya untuk draft", async () => {
    tables.payroll_runs = { id: "run-1", status: "processing", period_month: 6, period_year: 2026 };
    const { POST } = await import("@/app/api/hris/payroll/[id]/calculate/route");
    const res = await POST(req("POST", { include_thr: false }), params({ id: "run-1" }));
    await expectError(res, 400, "Hanya payroll draft yang bisa dihitung");
    expect(writes).toEqual([]);
  });

  it("payroll-settings PUT kosong → 400", async () => {
    const { PUT } = await import("@/app/api/hris/payroll-settings/route");
    await expectError(await PUT(req("PUT", {})), 400, "Tidak ada perubahan yang dikirim");
  });

  it("payroll-settings PUT bracket tidak naik → 400 dengan nama field", async () => {
    const { PUT } = await import("@/app/api/hris/payroll-settings/route");
    const res = await PUT(req("PUT", { settings: { pph21_bracket_1: 100, pph21_bracket_2: 50 } }));
    await expectError(res, 400, "pph21_bracket_2 harus lebih besar dari pph21_bracket_1");
  });
});

describe("slip gaji", () => {
  it("PDF: id bukan UUID → 400", async () => {
    const { GET } = await import("@/app/api/hris/payslips/[id]/pdf/route");
    await expectError(await GET(req("GET"), params({ id: "abc" })), 400, "ID slip tidak valid");
  });

  it("PDF slip milik orang lain → 403 untuk non-HR", async () => {
    queryOne.mockResolvedValue({ employee_id: OTHER, full_name: "Sari", period_month: 6, period_year: 2026 });
    const { GET } = await import("@/app/api/hris/payslips/[id]/pdf/route");
    await expectError(await GET(req("GET"), params({ id: ID })), 403, "Insufficient permissions");
  });

  it("daftar ESS hanya slip run paid", async () => {
    tables.payroll_details = [
      { id: "a", payroll_run: { status: "paid" } },
      { id: "b", payroll_run: { status: "completed" } },
    ];
    const { GET } = await import("@/app/api/hris/payslips/route");
    const res = await GET(req("GET"));
    expect(res.status).toBe(200);
    expect((await res.json()).data.map((r: { id: string }) => r.id)).toEqual(["a"]);
  });
});

describe("pinjaman", () => {
  it("tenor di luar 1–60 → 400 pesan lama", async () => {
    const { POST } = await import("@/app/api/hris/loans/route");
    const res = await POST(req("POST", { loan_type: "kasbon", principal_amount: 1_000_000, tenor_months: 61 }));
    await expectError(res, 400, "Jenis pinjaman, jumlah (> 0), dan tenor (1–60 bulan) wajib valid");
  });

  it("akun tanpa karyawan → 400", async () => {
    getWorkforceActor.mockResolvedValue({ ...staff, employeeId: null });
    const { POST } = await import("@/app/api/hris/loans/route");
    const res = await POST(req("POST", { loan_type: "kasbon", principal_amount: 1_000_000, tenor_months: 3 }));
    await expectError(res, 400, "Akun ini tidak tertaut ke data karyawan");
  });

  it("approve pinjaman yang sudah diproses → 400", async () => {
    tables.loans = { id: "loan-1", status: "approved" };
    const { POST } = await import("@/app/api/hris/loans/[id]/approve/route");
    const res = await POST(req("POST", { approved: true }), params({ id: "loan-1" }));
    await expectError(res, 400, "Pinjaman sudah diproses");
  });
});

describe("cuti", () => {
  it("karyawan tanpa record → 403 di daftar", async () => {
    getWorkforceActor.mockResolvedValue({ ...staff, employeeId: null });
    const { GET } = await import("@/app/api/hris/leaves/route");
    await expectError(await GET(req("GET")), 403, "Akun ini tidak terhubung ke data karyawan");
  });

  it("POST tidak valid → 400 Validation failed", async () => {
    const { POST } = await import("@/app/api/hris/leaves/route");
    const res = await POST(req("POST", { leave_type: "annual", start_date: "2026-06-01" }));
    await expectError(res, 400, "Validation failed");
  });

  it("approve oleh bukan HR/atasan → 403", async () => {
    tables.leaves = { id: ID, status: "pending", employee: { reporting_to: "someone-else" } };
    const { POST } = await import("@/app/api/hris/leaves/approve/route");
    const res = await POST(req("POST", { leave_id: "11111111-1111-4111-8111-111111111111", action: "approve" }));
    await expectError(res, 403, /atasan langsung/);
  });

  it("detail cuti orang lain ditolak untuk karyawan biasa", async () => {
    tables.leaves = { id: ID, employee_id: OTHER, employee: { reporting_to: null } };
    const { GET } = await import("@/app/api/hris/leaves/[id]/route");
    await expectError(await GET(req("GET"), params({ id: ID })), 403, "Insufficient permissions");
  });

  it("hapus butuh aksi delete di menu Cuti & Izin dan alasan", async () => {
    const { DELETE } = await import("@/app/api/hris/leaves/[id]/route");
    requireIamMenuPrefix.mockRejectedValue(ApiError.forbidden("Insufficient permissions"));
    await expectError(
      await DELETE(req("DELETE", undefined, REASON), params({ id: ID })),
      403,
      "Insufficient permissions"
    );
    expect(writes).toEqual([]);
    requireIamMenuPrefix.mockResolvedValue(HR_USER);
    await expectError(
      await DELETE(req("DELETE"), params({ id: ID })),
      400,
      "Alasan hapus wajib diisi (minimal 5 karakter)"
    );
    expect(writes).toEqual([]);
    const ok = await DELETE(req("DELETE", undefined, REASON), params({ id: ID }));
    expect(ok.status).toBe(200);
    expect(writes).toEqual([{ table: "leaves", op: "delete", payload: undefined }]);
  });

  it("lampiran: employee_id HR wajib UUID (tidak bisa keluar folder)", async () => {
    getWorkforceActor.mockResolvedValue(hr);
    const { POST } = await import("@/app/api/hris/leaves/attachment/route");
    const res = await POST(req("POST", { photo: "data:image/png;base64,AA==", employee_id: "../contracts" }));
    await expectError(res, 400, "ID karyawan tidak valid");
  });

  it("saldo cuti karyawan lain ditolak untuk karyawan biasa", async () => {
    const { GET } = await import("@/app/api/hris/leave-balances/[employee_id]/route");
    const res = await GET(req("GET"), params({ employee_id: OTHER }));
    await expectError(res, 403, "Forbidden: Can only view own leave balance");
  });
});

describe("logbook", () => {
  const actor = {
    userId: "u-1",
    role: "employee",
    employeeId: ID,
    departmentId: "dept-1",
    isFullAccess: false,
    canReview: false,
  };

  beforeEach(() => getLogbookActor.mockResolvedValue(actor));

  it("aksi tak dikenal → 422", async () => {
    const { POST, PATCH } = await import("@/app/api/hris/logbook/route");
    await expectError(await POST(req("POST", { action: "x" })), 422, "Unknown action");
    await expectError(await PATCH(req("PATCH", { action: "x" })), 422, "Unknown action");
  });

  it("create-entry tanggal salah format → 400", async () => {
    const { POST } = await import("@/app/api/hris/logbook/route");
    const res = await POST(req("POST", { action: "create-entry", template_id: "t", entry_date: "01-06-2026" }));
    await expectError(res, 400, "Format entry_date harus YYYY-MM-DD");
  });

  it("department lain → 403", async () => {
    const { GET } = await import("@/app/api/hris/logbook/route");
    const res = await GET(req("GET", undefined, "?resource=templates&department_id=dept-9"));
    await expectError(res, 403, "Anda tidak berhak mengakses department ini");
  });

  it("review oleh non-reviewer → 403", async () => {
    const { PATCH } = await import("@/app/api/hris/logbook/route");
    const res = await PATCH(req("PATCH", { action: "review-entry", entry_id: "e-1" }));
    await expectError(res, 403, "Anda tidak berhak me-review logbook");
  });

  it("DELETE tanpa id → 400; resource tak dikenal → 422", async () => {
    const { DELETE } = await import("@/app/api/hris/logbook/route");
    await expectError(await DELETE(req("DELETE")), 400, "id is required");
    await expectError(await DELETE(req("DELETE", undefined, "?id=x&resource=foo")), 422, "Unknown resource");
  });

  it("summary dirata-rata per department", async () => {
    tables.hris_logbook_entries = [
      { department_id: "dept-1", department: { name: "Bar" }, status: "submitted", completion_percentage: 40, kpi_score: 60 },
      { department_id: "dept-1", department: { name: "Bar" }, status: "reviewed", completion_percentage: 80, kpi_score: 90 },
    ];
    const { GET } = await import("@/app/api/hris/logbook/route");
    const res = await GET(req("GET", undefined, "?resource=summary"));
    expect(res.status).toBe(200);
    expect((await res.json()).data).toEqual([
      {
        department: { name: "Bar" },
        total_entries: 2,
        submitted_entries: 1,
        reviewed_entries: 1,
        avg_completion: 60,
        avg_kpi_score: 75,
      },
    ]);
  });
});

describe("lembur", () => {
  // Tanggal depan: pengajuan sendiri tidak boleh lampau (EPIC-063 R3), jadi tes lain tetap menjangkau aturannya.
  const body = { date: "2099-06-01", start_time: "18:00", end_time: "20:00", reason: "Stock opname" };

  it("non-HR menugaskan orang lain → 403", async () => {
    const { POST } = await import("@/app/api/hris/overtime/route");
    const res = await POST(req("POST", { ...body, employee_id: OTHER }));
    await expectError(res, 403, "Hanya HRD yang bisa membuat penugasan lembur untuk karyawan lain");
  });

  it("jam mulai = selesai → 400", async () => {
    tables.employees = { id: ID, is_active: true };
    tables.overtime_requests = [];
    const { POST } = await import("@/app/api/hris/overtime/route");
    const res = await POST(req("POST", { ...body, end_time: "18:00" }));
    await expectError(res, 400, "Jam mulai dan selesai tidak boleh sama");
  });

  it("keputusan pada pengajuan non-pending → 400", async () => {
    tables.overtime_requests = { id: ID, status: "approved" };
    const { POST } = await import("@/app/api/hris/overtime/decide/route");
    const res = await POST(req("POST", { overtime_id: "11111111-1111-4111-8111-111111111111", action: "approve" }));
    await expectError(res, 400, "Pengajuan sudah approved");
  });
});

describe("KPI", () => {
  it("target dengan dua scope → 400", async () => {
    const { POST } = await import("@/app/api/hris/kpi/targets/route");
    const res = await POST(req("POST", { indicator_id: "i", target: 1, role_code: "pos", employee_id: ID }));
    await expectError(res, 400, "Pilih satu scope saja (role ATAU department ATAU karyawan)");
  });

  it("rubrik di luar 1–5 → 400", async () => {
    const { POST } = await import("@/app/api/hris/kpi/rubric/route");
    const res = await POST(req("POST", { employee_id: ID, period_month: 6, period_year: 2026, value: 6 }));
    await expectError(res, 400, "Nilai rubrik harus 1-5");
  });

  it("rubrik oleh bukan atasan langsung → 403", async () => {
    queryOne.mockResolvedValue({ reporting_to: "someone-else" });
    const { POST } = await import("@/app/api/hris/kpi/rubric/route");
    const res = await POST(req("POST", { employee_id: OTHER, period_month: 6, period_year: 2026, value: 4 }));
    await expectError(res, 403, "Hanya HRD atau atasan langsung yang boleh menilai");
  });

  it("scorecard PATCH aksi tidak valid → 400", async () => {
    const { PATCH } = await import("@/app/api/hris/kpi/scorecards/route");
    const res = await PATCH(req("PATCH", { action: "delete", scorecard_id: "s" }));
    await expectError(res, 400, "action (finalize|reopen) dan scorecard_id wajib");
  });

  it("kpi-config ditolak untuk role bukan pengelola KPI", async () => {
    requireIamMenuPrefix.mockResolvedValue({ ...HR_USER, role: "employee" });
    const { GET } = await import("@/app/api/hris/kpi-config/route");
    await expectError(await GET(), 403, /pengelola KPI/);
  });

  it("snapshot periode tidak valid → 400", async () => {
    const { POST } = await import("@/app/api/hris/kpi/snapshot/route");
    await expectError(await POST(req("POST", { period_month: 13 })), 400, "Periode tidak valid");
  });
});

describe("lain-lain", () => {
  it("reports bulan tidak valid → 400", async () => {
    const { GET } = await import("@/app/api/hris/reports/route");
    await expectError(await GET(req("GET", undefined, "?month=13")), 400, "Periode tidak valid");
  });

  it("me tanpa record karyawan", async () => {
    getWorkforceActor.mockResolvedValue({ ...staff, employeeId: null });
    const { GET } = await import("@/app/api/hris/me/route");
    expect(await (await GET()).json()).toEqual({ data: { employee: null, leave_balance: null } });
  });

  it("nav-badges POST modul tidak dikenal → 400", async () => {
    const { POST } = await import("@/app/api/hris/nav-badges/route");
    await expectError(await POST(req("POST", { module: "gaji" })), 400, "Modul tidak valid");
  });

  it("notifikasi POST tanpa target → 400", async () => {
    const { POST } = await import("@/app/api/hris/notifications/route");
    await expectError(await POST(req("POST", {})), 400, "notification_id or mark_all is required");
  });

  it("offboarding: karyawan hanya boleh resign sukarela", async () => {
    const { POST } = await import("@/app/api/hris/offboarding/[employee_id]/route");
    const res = await POST(
      req("POST", { resignation_type: "termination", resignation_date: "2026-06-01", last_working_day: "2026-06-30" }),
      params({ employee_id: ID })
    );
    await expectError(res, 403, "Only HRD/Manager can initiate non-voluntary resignation");
  });

  it("onboarding PUT tanpa task_id → 400", async () => {
    getWorkforceActor.mockResolvedValue(hr);
    const { PUT } = await import("@/app/api/hris/onboarding/[employee_id]/route");
    await expectError(await PUT(req("PUT", {}), params({ employee_id: ID })), 400, "task_id is required");
  });

  it("performance: buka siklus hanya HRD; cycle_id wajib", async () => {
    const cycles = await import("@/app/api/hris/performance/cycles/route");
    const res = await cycles.POST(req("POST", { period_year: 2026, period_quarter: 2 }));
    await expectError(res, 403, "Hanya HRD yang boleh membuka siklus review");
    const reviews = await import("@/app/api/hris/performance/reviews/route");
    await expectError(await reviews.GET(req("GET")), 400, "Parameter cycle_id wajib");
  });

  it("shift baru tanpa nama → 400", async () => {
    const { POST } = await import("@/app/api/hris/shifts/route");
    await expectError(await POST(req("POST", { start_time: "08:00", end_time: "16:00" })), 400, "Nama shift wajib diisi");
  });

  it("promote tanpa candidate_id → 400", async () => {
    const { POST } = await import("@/app/api/hris/promote/route");
    await expectError(await POST(req("POST", {})), 400, "candidate_id wajib diisi");
  });
});
