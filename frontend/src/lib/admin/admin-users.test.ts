// Fallback Next /api/admin/users: perubahan role ikut menyinkronkan iam.user_roles
// (sama dengan Go accounts updateAdmin/createAdmin), agar penurunan role benar-benar mencabut.
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createAdminUser, updateAdminUser } from "./admin-users";

const ID = "00000000-0000-4000-8000-000000000009";

const mocks = vi.hoisted(() => ({
  query: vi.fn(),
  syncIamPrimaryRole: vi.fn(),
  updateUserById: vi.fn(),
  createUser: vi.fn(),
  deleteUser: vi.fn(),
}));

vi.mock("@/lib/db", () => ({ query: mocks.query, queryOne: vi.fn() }));
vi.mock("@/lib/iam/sync-user-role", () => ({ syncIamPrimaryRole: mocks.syncIamPrimaryRole }));
vi.mock("@/lib/users/user-service", () => ({ resetUserEmployeePassword: vi.fn() }));
vi.mock("@/lib/pg/create-client", () => {
  // Builder PostgREST minimal: setiap metode berantai, single() mengembalikan baris.
  const builder = (): unknown =>
    new Proxy(
      {},
      {
        get: (_t, key) => {
          if (key === "then") return undefined;
          if (key === "single" || key === "maybeSingle") return async () => ({ data: { id: ID }, error: null });
          return () => builder();
        },
      }
    );
  return {
    createPgClient: () => ({
      from: () => builder(),
      auth: {
        admin: {
          updateUserById: mocks.updateUserById,
          createUser: mocks.createUser,
          deleteUser: mocks.deleteUser,
        },
      },
    }),
  };
});

beforeEach(() => {
  vi.clearAllMocks();
  mocks.updateUserById.mockResolvedValue({ error: null });
  mocks.createUser.mockResolvedValue({ data: { user: { id: ID } }, error: null });
  mocks.query.mockResolvedValue([{ role: "super_admin" }]);
});

describe("updateAdminUser", () => {
  it("role berubah → iam.user_roles disinkronkan ke role baru", async () => {
    await updateAdminUser("actor-1", ID, { role: "hrd" });
    expect(mocks.syncIamPrimaryRole).toHaveBeenCalledWith(ID, "hrd", "actor-1");
  });

  it("role sama → tidak menyentuh iam.user_roles", async () => {
    mocks.query.mockResolvedValue([{ role: "hrd" }]);
    await updateAdminUser("actor-1", ID, { role: "hrd", full_name: "Nama Baru" });
    expect(mocks.syncIamPrimaryRole).not.toHaveBeenCalled();
  });

  it("tanpa role → tidak menyentuh iam.user_roles", async () => {
    await updateAdminUser("actor-1", ID, { full_name: "Nama Baru" });
    expect(mocks.syncIamPrimaryRole).not.toHaveBeenCalled();
  });
});

describe("createAdminUser", () => {
  it("role akun baru masuk iam.user_roles", async () => {
    await createAdminUser("actor-1", {
      email: "baru@example.com",
      password: "rahasia123",
      full_name: "Admin Baru",
      role: "hrd",
      status: "active",
      approval_permissions: [],
    });
    expect(mocks.syncIamPrimaryRole).toHaveBeenCalledWith(ID, "hrd", "actor-1");
  });
});
