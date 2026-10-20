// Guard role sistem untuk Manajemen User (fallback Next, temuan lanjutan EPIC-061).
import { beforeEach, describe, expect, it, vi } from "vitest";

const iamDbQuery = vi.fn();
const iamDbQueryOne = vi.fn();
const requireIamSuperAdmin = vi.fn();

vi.mock("@/lib/iam/pg-client", () => ({
  iamDbQuery: (...a: unknown[]) => iamDbQuery(...a),
  iamDbQueryOne: (...a: unknown[]) => iamDbQueryOne(...a),
}));
vi.mock("@/lib/iam/system-role-guard", () => ({
  requireIamSuperAdmin: (...a: unknown[]) => requireIamSuperAdmin(...a),
}));

const user = { id: "actor-1", full_name: "HR", role: "hrd" as const, brand_id: null };

beforeEach(() => {
  iamDbQuery.mockReset().mockResolvedValue([]);
  iamDbQueryOne.mockReset().mockResolvedValue({ held: false });
  requireIamSuperAdmin.mockReset().mockResolvedValue(undefined);
});

describe("systemRoleCodes", () => {
  it("tetap super_admin & admin walau iam.roles belum di-seed", async () => {
    const { systemRoleCodes } = await import("./system-account-guard");
    expect(await systemRoleCodes()).toEqual(["super_admin", "admin"]);
  });

  it("menambah role is_system dari DB tanpa duplikat", async () => {
    iamDbQuery.mockResolvedValue([{ code: "admin" }, { code: "owner" }]);
    const { systemRoleCodes } = await import("./system-account-guard");
    expect((await systemRoleCodes()).sort()).toEqual(["admin", "owner", "super_admin"]);
  });
});

describe("touchesLogin", () => {
  it("field akun, email dan nama sampai ke akun login; data karyawan lain tidak", async () => {
    const { touchesLogin } = await import("./system-account-guard");
    expect(touchesLogin({ phone: "08", notes: "x" })).toBe(false);
    expect(touchesLogin({ full_name: "Baru" })).toBe(true);
    expect(touchesLogin({ account_status: "inactive" })).toBe(true);
    expect(touchesLogin({ is_access_app: false })).toBe(true);
  });
});

describe("requireSuperAdminForSystemAccount", () => {
  it("role sistem → cek super_admin dengan pesan yang sama dengan Go", async () => {
    const { requireSuperAdminForSystemAccount, SYSTEM_ACCOUNT_SUPER_ADMIN_ERROR } = await import("./system-account-guard");
    await requireSuperAdminForSystemAccount(user, { role: "admin" });
    expect(requireIamSuperAdmin).toHaveBeenCalledWith(user, SYSTEM_ACCOUNT_SUPER_ADMIN_ERROR);
    expect(SYSTEM_ACCOUNT_SUPER_ADMIN_ERROR).toBe("Role Super Admin/Admin hanya bisa diatur oleh Super Admin");
  });

  it("akun target memegang role sistem → cek super_admin", async () => {
    iamDbQueryOne.mockResolvedValue({ held: true });
    const { requireSuperAdminForSystemAccount } = await import("./system-account-guard");
    await requireSuperAdminForSystemAccount(user, { role: "hrd", userId: "u-1" });
    expect(iamDbQueryOne.mock.calls[0][1]).toEqual(["u-1", ["super_admin", "admin"]]);
    expect(requireIamSuperAdmin).toHaveBeenCalledTimes(1);
  });

  it("role biasa pada akun biasa → tidak ada cek", async () => {
    const { requireSuperAdminForSystemAccount } = await import("./system-account-guard");
    await requireSuperAdminForSystemAccount(user, { role: "hrd", userId: "u-1" });
    await requireSuperAdminForSystemAccount(user, { userId: null });
    expect(requireIamSuperAdmin).not.toHaveBeenCalled();
  });
});
