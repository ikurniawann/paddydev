// /api/users: guard IAM dulu, query list divalidasi, galat service dipetakan tanpa bocor.
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { NextRequest } from "next/server";

const requireIamMenuPrefix = vi.fn();
const service = {
  listUserEmployees: vi.fn(),
  createUserEmployee: vi.fn(),
  getUserEmployeeById: vi.fn(),
  updateUserEmployee: vi.fn(),
  resetUserEmployeePassword: vi.fn(),
};
const single = vi.fn();
// Guard role sistem (system-account-guard → pg-client IAM).
const iamDbQuery = vi.fn();
const iamDbQueryOne = vi.fn();
const resolveRoleIds = vi.fn();
const iam = { superAdmin: false, holdsSystem: false };

vi.mock("@/lib/api/auth", async (orig) => ({
  ...(await orig<typeof import("@/lib/api/auth")>()),
  requireIamMenuPrefix: (...a: unknown[]) => requireIamMenuPrefix(...a),
}));
vi.mock("@/lib/users/user-service", () => service);
vi.mock("@/lib/iam/pg-client", () => ({
  iamDbQuery: (...a: unknown[]) => iamDbQuery(...a),
  iamDbQueryOne: (...a: unknown[]) => iamDbQueryOne(...a),
}));
vi.mock("@/lib/iam/get-user-menus", async (orig) => ({
  ...(await orig<typeof import("@/lib/iam/get-user-menus")>()),
  resolveRoleIds: (...a: unknown[]) => resolveRoleIds(...a),
}));
vi.mock("@/lib/pg/create-client", () => ({
  createPgClient: () => ({
    from: () => ({ select: () => ({ eq: () => ({ single }) }) }),
  }),
}));

const ID = "00000000-0000-4000-8000-000000000001";
const ctx = { params: Promise.resolve({ id: ID }) };
const getReq = (qs = "") => {
  const url = new URL(`http://x/api/users${qs}`);
  return { url: url.toString(), nextUrl: url } as unknown as NextRequest;
};
const jsonReq = (body: unknown) =>
  new Request("http://x/api/users", {
    method: "POST",
    body: JSON.stringify(body),
  }) as unknown as NextRequest;
const validEmployee = {
  full_name: "Budi Santoso",
  email: "budi@example.com",
  join_date: "2026-01-01",
  employment_status: "permanent",
};

beforeEach(async () => {
  Object.values(service).forEach((fn) => fn.mockReset());
  single.mockReset();
  requireIamMenuPrefix.mockReset().mockResolvedValue({ id: "actor-1", role: "hrd" });
  iam.superAdmin = false;
  iam.holdsSystem = false;
  resolveRoleIds.mockReset().mockResolvedValue(["role-1"]);
  iamDbQuery.mockReset().mockResolvedValue([{ code: "super_admin" }, { code: "admin" }]);
  iamDbQueryOne.mockReset().mockImplementation(async (sql: string) => {
    if (sql.includes("hris.employees")) return { user_id: "u-9" };
    if (sql.includes("AS held")) return { held: iam.holdsSystem };
    if (sql.includes("code = 'super_admin'")) return { ok: iam.superAdmin };
    return null;
  });
});

describe("tanpa grant settings.users", () => {
  it.each([
    ["GET list", async () => (await import("./route")).GET(getReq())],
    ["POST", async () => (await import("./route")).POST(jsonReq(validEmployee))],
    ["GET detail", async () => (await import("./[id]/route")).GET(getReq(), ctx)],
    ["PUT", async () => (await import("./[id]/route")).PUT(jsonReq({ phone: "1" }), ctx)],
    [
      "reset-password",
      async () => (await import("./[id]/reset-password/route")).POST(getReq(), ctx),
    ],
  ])("%s → 403 tanpa menyentuh service", async (_name, run) => {
    const { ApiError } = await import("@/lib/api/auth");
    requireIamMenuPrefix.mockRejectedValue(ApiError.forbidden());
    const res = await run();
    expect(res.status).toBe(403);
    expect(await res.json()).toEqual({
      success: false,
      error: "Insufficient permissions",
    });
    Object.values(service).forEach((fn) => expect(fn).not.toHaveBeenCalled());
    expect(single).not.toHaveBeenCalled();
  });
});

describe("GET /api/users", () => {
  it("meneruskan filter yang sudah divalidasi", async () => {
    service.listUserEmployees.mockResolvedValue({
      data: [],
      total: 0,
      page: 2,
      perPage: 15,
    });
    const { GET } = await import("./route");
    const res = await GET(getReq("?page=2&limit=15&is_access_app=true&search=budi"));
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({
      data: [],
      total: 0,
      page: 2,
      perPage: 15,
    });
    expect(service.listUserEmployees).toHaveBeenCalledWith(
      expect.objectContaining({
        page: 2,
        limit: 15,
        isAccessApp: true,
        search: "budi",
        sortBy: "full_name",
      })
    );
  });

  it("sort_by di luar allowlist → 400", async () => {
    const { GET } = await import("./route");
    const res = await GET(getReq("?sort_by=password_hash"));
    expect(res.status).toBe(400);
    expect(service.listUserEmployees).not.toHaveBeenCalled();
  });
});

describe("POST /api/users", () => {
  it("201 dengan bentuk respons lama", async () => {
    service.createUserEmployee.mockResolvedValue({ id: ID });
    const { POST } = await import("./route");
    const res = await POST(jsonReq(validEmployee));
    expect(res.status).toBe(201);
    expect(await res.json()).toEqual({
      data: { id: ID },
      message: "Employee created successfully",
    });
    expect(service.createUserEmployee.mock.calls[0][0]).toBe("actor-1");
  });

  it("email dipakai → 409 berpesan jelas", async () => {
    const { ApiError } = await import("@/lib/api/auth");
    const { EMAIL_IN_USE_MESSAGE } = await import("@/lib/users/email-conflict");
    service.createUserEmployee.mockRejectedValue(ApiError.conflict(EMAIL_IN_USE_MESSAGE));
    const { POST } = await import("./route");
    const res = await POST(jsonReq(validEmployee));
    expect(res.status).toBe(409);
    expect(await res.json()).toEqual({ success: false, error: EMAIL_IN_USE_MESSAGE });
  });

  it("galat internal → 500 tanpa membocorkan pesan", async () => {
    vi.spyOn(console, "error").mockImplementation(() => {});
    service.createUserEmployee.mockRejectedValue(new Error("connection refused 10.0.0.5"));
    const { POST } = await import("./route");
    const res = await POST(jsonReq(validEmployee));
    expect(res.status).toBe(500);
    expect((await res.json()).error).not.toContain("10.0.0.5");
  });

  it("body tidak valid → 400 tanpa memanggil service", async () => {
    const { POST } = await import("./route");
    const res = await POST(jsonReq({ full_name: "B" }));
    expect(res.status).toBe(400);
    expect(service.createUserEmployee).not.toHaveBeenCalled();
  });
});

describe("/api/users/[id]", () => {
  it("GET: tidak ada → 404", async () => {
    service.getUserEmployeeById.mockResolvedValue(null);
    const { GET } = await import("./[id]/route");
    const res = await GET(getReq(), ctx);
    expect(res.status).toBe(404);
    expect((await res.json()).error).toBe("Employee not found");
  });

  it("PUT: atasan langsung = karyawan itu sendiri → 400 tanpa memanggil service (EPIC-063 R9)", async () => {
    const { PUT } = await import("./[id]/route");
    const res = await PUT(jsonReq({ reporting_to: ID.toUpperCase() }), ctx);
    expect(res.status).toBe(400);
    expect((await res.json()).error).toBe("Atasan langsung tidak boleh karyawan itu sendiri");
    expect(service.updateUserEmployee).not.toHaveBeenCalled();
  });

  it("PUT: karyawan tidak ada → 400 berpesan sama", async () => {
    service.updateUserEmployee.mockRejectedValue(new Error("Employee not found"));
    const { PUT } = await import("./[id]/route");
    const res = await PUT(jsonReq({ phone: "0812" }), ctx);
    expect(res.status).toBe(400);
    expect((await res.json()).error).toBe("Employee not found");
  });
});

describe("POST /api/users/[id]/reset-password", () => {
  it("karyawan tanpa akses aplikasi → 400", async () => {
    single.mockResolvedValue({
      data: { user_id: null, is_access_app: false },
      error: null,
    });
    const { POST } = await import("./[id]/reset-password/route");
    const res = await POST(getReq(), ctx);
    expect(res.status).toBe(400);
    expect(service.resetUserEmployeePassword).not.toHaveBeenCalled();
  });

  it("karyawan tidak ada → 404", async () => {
    single.mockResolvedValue({ data: null, error: { code: "PGRST116" } });
    const { POST } = await import("./[id]/reset-password/route");
    expect((await POST(getReq(), ctx)).status).toBe(404);
  });

  it("sukses → message + tempPassword", async () => {
    single.mockResolvedValue({
      data: { user_id: "u-9", is_access_app: true },
      error: null,
    });
    service.resetUserEmployeePassword.mockResolvedValue({
      message: "Password reset successfully",
      tempPassword: "ArkivX!",
    });
    const { POST } = await import("./[id]/reset-password/route");
    const res = await POST(getReq(), ctx);
    expect(await res.json()).toEqual({
      message: "Password reset successfully",
      tempPassword: "ArkivX!",
    });
    expect(service.resetUserEmployeePassword).toHaveBeenCalledWith("u-9");
  });
});

describe("role sistem hanya Super Admin (temuan lanjutan EPIC-061)", () => {
  const SYSTEM_MSG = "Role Super Admin/Admin hanya bisa diatur oleh Super Admin";
  const withAccount = (role: string) => ({
    ...validEmployee,
    is_access_app: true,
    password: "rahasia123",
    role,
    ...(role === "super_admin" ? {} : { business_scope: "holding", holding_id: ID }),
  });
  const forbidden = async (res: Response) => {
    expect(res.status).toBe(403);
    expect(await res.json()).toEqual({ success: false, error: SYSTEM_MSG });
  };

  it.each(["super_admin", "admin"])("POST memberi role %s tanpa super_admin → 403, tidak menulis", async (role) => {
    const { POST } = await import("./route");
    await forbidden(await POST(jsonReq(withAccount(role))));
    expect(service.createUserEmployee).not.toHaveBeenCalled();
  });

  it("POST role super_admin oleh super_admin → 201", async () => {
    iam.superAdmin = true;
    service.createUserEmployee.mockResolvedValue({ id: ID });
    const { POST } = await import("./route");
    expect((await POST(jsonReq(withAccount("super_admin")))).status).toBe(201);
  });

  it("POST role biasa tidak butuh super_admin", async () => {
    service.createUserEmployee.mockResolvedValue({ id: ID });
    const { POST } = await import("./route");
    expect((await POST(jsonReq(withAccount("hrd")))).status).toBe(201);
  });

  it("PUT mempromosikan ke admin → 403", async () => {
    const { PUT } = await import("./[id]/route");
    await forbidden(await PUT(jsonReq({ is_access_app: true, role: "admin", business_scope: "holding", holding_id: ID }), ctx));
    expect(service.updateUserEmployee).not.toHaveBeenCalled();
  });

  it.each([
    ["menonaktifkan", { account_status: "inactive" }],
    ["mencabut akses", { is_access_app: false }],
    ["mengganti email login", { email: "baru@example.com" }],
    ["menurunkan role", { is_access_app: true, role: "hrd", business_scope: "holding", holding_id: ID }],
  ])("PUT %s akun ber-role sistem → 403", async (_name, body) => {
    iam.holdsSystem = true;
    const { PUT } = await import("./[id]/route");
    await forbidden(await PUT(jsonReq(body), ctx));
    expect(service.updateUserEmployee).not.toHaveBeenCalled();
  });

  it("PUT data karyawan saja pada akun ber-role sistem tetap boleh", async () => {
    iam.holdsSystem = true;
    service.updateUserEmployee.mockResolvedValue({ id: ID });
    const { PUT } = await import("./[id]/route");
    expect((await PUT(jsonReq({ phone: "0812" }), ctx)).status).toBe(200);
  });

  it("PUT akun ber-role sistem oleh super_admin → 200", async () => {
    iam.holdsSystem = true;
    iam.superAdmin = true;
    service.updateUserEmployee.mockResolvedValue({ id: ID });
    const { PUT } = await import("./[id]/route");
    expect((await PUT(jsonReq({ account_status: "inactive" }), ctx)).status).toBe(200);
  });

  it("reset password akun ber-role sistem → 403", async () => {
    iam.holdsSystem = true;
    single.mockResolvedValue({ data: { user_id: "u-9", is_access_app: true }, error: null });
    const { POST } = await import("./[id]/reset-password/route");
    await forbidden(await POST(getReq(), ctx));
    expect(service.resetUserEmployeePassword).not.toHaveBeenCalled();
  });
});

describe("id karyawan dikanonikkan sebagai UUID (temuan re-review EPIC-061)", () => {
  const employeeParams = () =>
    iamDbQueryOne.mock.calls.filter((c) => String(c[0]).includes("hris.employees")).map((c) => (c[1] as unknown[])[0]);

  it("PUT id huruf besar: akun dicari & diubah dengan id kanonik", async () => {
    iam.holdsSystem = true;
    const { PUT } = await import("./[id]/route");
    const res = await PUT(jsonReq({ account_status: "inactive" }), { params: Promise.resolve({ id: ID.toUpperCase() }) });
    expect(res.status).toBe(403);
    expect(employeeParams()).toEqual([ID]);
  });

  it("reset password id tanpa tanda hubung: dicari dengan id kanonik", async () => {
    iam.holdsSystem = true;
    const { POST } = await import("./[id]/reset-password/route");
    const res = await POST(getReq(), { params: Promise.resolve({ id: ID.replaceAll("-", "") }) });
    expect(res.status).toBe(403);
    expect(employeeParams()).toEqual([ID]);
  });

  it("id rusak → 400 sebelum guard", async () => {
    const bad = { params: Promise.resolve({ id: "nope" }) };
    const { PUT } = await import("./[id]/route");
    const { POST } = await import("./[id]/reset-password/route");
    for (const res of [await PUT(jsonReq({ account_status: "inactive" }), bad), await POST(getReq(), bad)]) {
      expect(res.status).toBe(400);
      expect(await res.json()).toEqual({ success: false, error: "Invalid id" });
    }
    expect(service.updateUserEmployee).not.toHaveBeenCalled();
    expect(service.resetUserEmployeePassword).not.toHaveBeenCalled();
  });
});
