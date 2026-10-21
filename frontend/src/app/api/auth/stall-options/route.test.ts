// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  requireApiUser: vi.fn(),
  getApiUserScope: vi.fn(),
  getStallAccess: vi.fn(),
  findStallOption: vi.fn(),
  resolveActiveStallFromCookies: vi.fn(),
}));

vi.mock("@/lib/api/auth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/auth")>()),
  requireApiUser: mocks.requireApiUser,
}));
vi.mock("@/lib/api/scope", () => ({ getApiUserScope: mocks.getApiUserScope }));
vi.mock("@/lib/auth/stall-access", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/auth/stall-access")>()),
  getStallAccess: mocks.getStallAccess,
  findStallOption: mocks.findStallOption,
}));
vi.mock("@/lib/auth/active-stall", () => ({
  resolveActiveStallFromCookies: mocks.resolveActiveStallFromCookies,
}));
vi.mock("@/lib/db", () => ({ queryOne: vi.fn(), query: vi.fn() }));

import { GET } from "./route";

const stall = (
  id: string,
  name: string,
  code: string,
  branch: [string, string],
  is_default = false
) => ({ id, name, code, is_default, branch_id: branch[0], branch_name: branch[1] });

const ALPHA: [string, string] = ["b-alpha", "Alpha"];
const ZETA: [string, string] = ["b-zeta", "Zeta"];
const mainZeta = stall("m-z", "Main Storage", "MAIN", ZETA, true);
const mainAlpha = stall("m-a", "Main Storage", "MAIN", ALPHA, true);
const stallAlpha = stall("s-a", "Stall 1", "STALL-01", ALPHA);

const get = async () => (await GET()).json();

beforeEach(() => {
  vi.clearAllMocks();
  mocks.requireApiUser.mockResolvedValue({ id: "u1", role: "super_admin", full_name: "A", brand_id: null });
  mocks.getApiUserScope.mockResolvedValue(null);
  mocks.resolveActiveStallFromCookies.mockResolvedValue({ mode: "unset" });
  mocks.findStallOption.mockResolvedValue(null);
});

describe("GET /api/auth/stall-options", () => {
  it("lintas branch: label memuat nama branch, urut per nama branch lalu Main Storage", async () => {
    mocks.getStallAccess.mockResolvedValue({
      allAccess: true,
      stalls: [mainZeta, stallAlpha, mainAlpha],
    });
    const { data } = await get();
    expect(data.stalls.map((s: { label: string }) => s.label)).toEqual([
      "Main Storage — Alpha",
      "Stall 1 — Alpha",
      "Main Storage — Zeta",
    ]);
    expect(data.stalls[0]).toEqual({
      id: "m-a",
      name: "Main Storage",
      code: "MAIN",
      branch_id: "b-alpha",
      branch_name: "Alpha",
      label: "Main Storage — Alpha",
    });
    expect(data.active).toBeNull();
  });

  it("satu branch: label = nama polos", async () => {
    mocks.getStallAccess.mockResolvedValue({ allAccess: true, stalls: [stallAlpha, mainAlpha] });
    const { data } = await get();
    expect(data.stalls.map((s: { label: string }) => s.label)).toEqual(["Main Storage", "Stall 1"]);
    expect(data.stalls[0].branch_name).toBe("Alpha");
  });

  it("stall aktif dari cookie memakai label yang sama dengan daftar", async () => {
    mocks.getStallAccess.mockResolvedValue({ allAccess: true, stalls: [mainAlpha, mainZeta] });
    mocks.resolveActiveStallFromCookies.mockResolvedValue({
      mode: "stall",
      stall: { id: "m-z", name: "Main Storage", code: "MAIN" },
    });
    mocks.findStallOption.mockResolvedValue(mainZeta);
    const { data } = await get();
    expect(mocks.findStallOption).toHaveBeenCalledWith("m-z");
    expect(data.active.label).toBe("Main Storage — Zeta");
    expect(data.active.branch_id).toBe("b-zeta");
  });

  it("user bukan switcher: active default = Main Storage penempatan, tanpa daftar", async () => {
    mocks.requireApiUser.mockResolvedValue({ id: "u2", role: "pos", full_name: "K", brand_id: null });
    mocks.getStallAccess.mockResolvedValue({ allAccess: false, stalls: [mainAlpha] });
    const { data } = await get();
    expect(data.can_switch).toBe(false);
    expect(data.stalls).toEqual([]);
    expect(data.active).toMatchObject({ id: "m-a", label: "Main Storage", branch_name: "Alpha" });
  });

  it("penempatan lintas branch: default aktif tetap Main Storage pertama, bukan urutan branch", async () => {
    mocks.requireApiUser.mockResolvedValue({ id: "u3", role: "pos", full_name: "K", brand_id: null });
    mocks.getStallAccess.mockResolvedValue({ allAccess: false, stalls: [stallAlpha, mainZeta] });
    const { data } = await get();
    expect(data.active.id).toBe("m-z");
    expect(data.active.label).toBe("Main Storage — Zeta");
    expect(data.stalls.map((s: { id: string }) => s.id)).toEqual(["s-a", "m-z"]);
  });
});
