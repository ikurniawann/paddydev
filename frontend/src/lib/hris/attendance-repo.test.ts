import { describe, expect, it, vi } from "vitest";

vi.mock("@/lib/db", () => ({ query: vi.fn(), queryOne: vi.fn() }));
vi.mock("@/lib/pg/create-client", () => ({ createServerPgClient: vi.fn() }));
vi.mock("@/lib/storage-private", () => ({ savePrivateImage: vi.fn() }));

const { resolveAttendanceListParams, resolveStatsPeriod, todayWib } = await import("./attendance-repo");

const hr = { userId: "u-hr", role: "hrd", employeeId: "emp-hr", isHr: true, isHrd: true };
const staff = { userId: "u-1", role: "employee", employeeId: "emp-me", isHr: false, isHrd: false };

describe("resolveAttendanceListParams", () => {
  it("non-HR selalu dipaksa ke record sendiri", () => {
    const params = resolveAttendanceListParams(new URLSearchParams("employee_id=emp-lain"), staff);
    expect(params?.employeeId).toBe("emp-me");
  });
  it("HR boleh memilih karyawan; me = record sendiri", () => {
    expect(resolveAttendanceListParams(new URLSearchParams("employee_id=emp-x"), hr)?.employeeId).toBe("emp-x");
    expect(resolveAttendanceListParams(new URLSearchParams("employee_id=me"), hr)?.employeeId).toBe("emp-hr");
  });
  it("me tanpa record karyawan → daftar kosong (null)", () => {
    expect(resolveAttendanceListParams(new URLSearchParams("employee_id=me"), { ...hr, employeeId: null })).toBeNull();
  });
  it("non-HR tanpa record karyawan → 403", () => {
    expect(() => resolveAttendanceListParams(new URLSearchParams(), { ...staff, employeeId: null })).toThrow(
      expect.objectContaining({ status: 403 })
    );
  });
  it("halaman & limit dijaga tetap positif", () => {
    const params = resolveAttendanceListParams(new URLSearchParams("page=0&limit=abc&is_late=true"), hr);
    expect(params).toMatchObject({ page: 1, limit: 20, lateOnly: true });
  });
});

describe("tanggal WIB", () => {
  it("todayWib menggeser 7 jam dari UTC", () => {
    expect(todayWib(Date.parse("2026-10-03T18:00:00Z"))).toBe("2026-10-04");
    expect(todayWib(Date.parse("2026-10-03T16:00:00Z"))).toBe("2026-10-03");
  });
  it("periode statistik default bulan berjalan WIB", () => {
    expect(resolveStatsPeriod(new URLSearchParams(), Date.parse("2026-09-30T18:00:00Z"))).toEqual({
      month: 10,
      year: 2026,
    });
    expect(resolveStatsPeriod(new URLSearchParams("month=2&year=2025"))).toEqual({ month: 2, year: 2025 });
  });
});
