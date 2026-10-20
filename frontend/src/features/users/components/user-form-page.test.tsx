// Edit karyawan yang akun loginnya memegang role sistem: selain Super Admin melihat
// pemberitahuan read-only dan tombol simpan mati (server tetap penentu, temuan EPIC-061).
import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import type { UserEmployeeItem } from "../types";
import { UserFormPage } from "./user-form-page";

const NOTICE = "Akun ini memegang role Super Admin/Admin; hanya Super Admin yang bisa mengubahnya.";
const EMP_ID = "33333333-3333-4333-8333-333333333333";

const state = vi.hoisted(() => ({ callerRole: "hrd", accountRole: "admin" as string | null }));

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => <a href={href}>{children}</a>,
}));
vi.mock("@/components/iam/iam-access-provider", () => ({ useIamAccess: () => ({ hasPrefix: () => true }) }));
vi.mock("@/hooks/use-auth", () => ({ useAuth: () => ({ user: { role: state.callerRole } }) }));
vi.mock("@/features/configuration/business", () => ({ useBusinessTree: () => ({ data: undefined }) }));
vi.mock("../queries", () => ({
  useUserDetail: () => ({
    isLoading: false,
    data: {
      data: {
        id: EMP_ID,
        fullName: "Akun Sistem",
        email: "sistem@example.com",
        isActive: true,
        isAccessApp: state.accountRole !== null,
        userId: state.accountRole !== null ? "user-1" : null,
        appAccount: state.accountRole !== null ? { id: "user-1", role: state.accountRole } : null,
      } as unknown as UserEmployeeItem,
    },
  }),
  useUserFormLookups: () => ({ data: undefined }),
}));
vi.mock("../mutations", () => ({
  useCreateUser: () => ({ isPending: false, mutateAsync: vi.fn() }),
  useUpdateUser: () => ({ isPending: false, mutateAsync: vi.fn() }),
  useResetUserPassword: () => ({ isPending: false, mutateAsync: vi.fn() }),
}));

const saveButton = () => screen.getByRole("button", { name: "Save Changes" });

beforeEach(() => {
  state.callerRole = "hrd";
  state.accountRole = "admin";
});

describe("UserFormPage — akun ber-role sistem", () => {
  it.each(["admin", "super_admin"])("akun %s oleh selain Super Admin: notice + simpan mati", (role) => {
    state.accountRole = role;
    render(<UserFormPage mode="edit" employeeId={EMP_ID} />);
    expect(screen.getByText(NOTICE)).toBeInTheDocument();
    expect(saveButton()).toBeDisabled();
  });

  it("Super Admin tetap bisa menyimpan", () => {
    state.callerRole = "super_admin";
    render(<UserFormPage mode="edit" employeeId={EMP_ID} />);
    expect(screen.queryByText(NOTICE)).not.toBeInTheDocument();
    expect(saveButton()).not.toBeDisabled();
  });

  it("akun biasa tidak dikunci", () => {
    state.accountRole = "hrd";
    render(<UserFormPage mode="edit" employeeId={EMP_ID} />);
    expect(screen.queryByText(NOTICE)).not.toBeInTheDocument();
    expect(saveButton()).not.toBeDisabled();
  });
});
