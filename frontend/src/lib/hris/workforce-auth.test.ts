import { beforeEach, describe, expect, it, vi } from "vitest";

let role = "employee";
const selects: string[] = [];

vi.mock("@/lib/pg/create-client", () => ({
  createServerPgClient: async () => ({
    auth: { getUser: async () => ({ data: { user: { id: "u-1" } } }) },
    from: (table: string) => ({
      select: (cols: string) => {
        selects.push(`${table}:${cols}`);
        return {
          eq: () => ({
            single: async () =>
              table === "users"
                ? { data: { role, full_name: "Siti Aminah" } }
                : { data: { id: "emp-1" } },
          }),
        };
      },
    }),
  }),
}));

import { getWorkforceActor } from "./workforce-auth";

beforeEach(() => {
  selects.length = 0;
});

describe("getWorkforceActor — himpunan HRD (EPIC-063 ruling C1/P3)", () => {
  it.each([
    ["super_admin", true, true],
    ["admin", true, true],
    ["hrd", true, true],
    ["hiring_manager", true, false],
    ["employee", false, false],
  ])("role %s → isHr=%s isHrd=%s", async (r, isHr, isHrd) => {
    role = r;
    const actor = await getWorkforceActor();
    expect(actor).toMatchObject({ role: r, isHr, isHrd, employeeId: "emp-1", fullName: "Siti Aminah" });
  });

  it("membaca nama akun untuk decided_by_name", async () => {
    role = "hrd";
    await getWorkforceActor();
    expect(selects).toContain("users:role, full_name");
  });
});
