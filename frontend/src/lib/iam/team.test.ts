import { afterEach, describe, expect, it, vi } from "vitest";

const pg = vi.hoisted(() => ({ iamDbQueryOne: vi.fn(), isIamDbConfigured: vi.fn(() => true) }));
vi.mock("./pg-client", () => pg);

import { hasActiveTeam } from "./team";

afterEach(() => {
  vi.clearAllMocks();
  pg.isIamDbConfigured.mockReturnValue(true);
});

describe("hasActiveTeam (EPIC-074 R6)", () => {
  it("akun dengan bawahan aktif → true; kueri memakai user id", async () => {
    pg.iamDbQueryOne.mockResolvedValue({ ok: true });
    await expect(hasActiveTeam("u-boss")).resolves.toBe(true);
    expect(pg.iamDbQueryOne).toHaveBeenCalledWith(expect.stringContaining("reporting_to"), ["u-boss"]);
  });

  it("tanpa bawahan → false", async () => {
    pg.iamDbQueryOne.mockResolvedValue({ ok: false });
    await expect(hasActiveTeam("u-staff")).resolves.toBe(false);
  });

  it("gagal dibaca → false", async () => {
    pg.iamDbQueryOne.mockRejectedValue(new Error("db down"));
    await expect(hasActiveTeam("u-x")).resolves.toBe(false);
  });

  it("DB IAM tidak dikonfigurasi → false tanpa kueri", async () => {
    pg.isIamDbConfigured.mockReturnValue(false);
    await expect(hasActiveTeam("u-y")).resolves.toBe(false);
    expect(pg.iamDbQueryOne).not.toHaveBeenCalled();
  });
});
