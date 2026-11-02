// Edit karyawan yang akun loginnya memegang role sistem: selain Super Admin melihat
// pemberitahuan read-only dan tombol simpan mati (server tetap penentu, temuan EPIC-061).
import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { UserEmployeeItem } from "../types";
import { UserFormPage } from "./user-form-page";

const NOTICE = "Akun ini memegang role Super Admin/Admin; hanya Super Admin yang bisa mengubahnya.";
const EMP_ID = "33333333-3333-4333-8333-333333333333";

const state = vi.hoisted(() => ({
  callerRole: "hrd",
  accountRole: "admin" as string | null,
  nip: "202509397" as string | null,
  update: vi.fn(),
  create: vi.fn(),
}));
const toastError = vi.hoisted(() => vi.fn());
vi.mock("sonner", () => ({ toast: { error: toastError, success: vi.fn() } }));

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
        nip: state.nip,
        joinDate: "2026-01-01T00:00:00.000Z",
        employmentStatus: "permanent",
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
  useCreateUser: () => ({ isPending: false, mutateAsync: state.create }),
  useUpdateUser: () => ({ isPending: false, mutateAsync: state.update }),
  useResetUserPassword: () => ({ isPending: false, mutateAsync: vi.fn() }),
}));

const saveButton = () => screen.getByRole("button", { name: "Save Changes" });

beforeEach(() => {
  state.callerRole = "hrd";
  state.accountRole = "admin";
  state.nip = "202509397";
  state.update = vi.fn().mockResolvedValue({ message: "ok", data: { id: EMP_ID } });
  state.create = vi.fn();
  toastError.mockClear();
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

describe("UserFormPage — NIP manual wajib (EPIC-081)", () => {
  const nipInput = () => screen.getByLabelText(/NIP/) as HTMLInputElement;

  it("menampilkan kolom NIP terisi dari data karyawan", () => {
    state.accountRole = null;
    render(<UserFormPage mode="edit" employeeId={EMP_ID} />);
    expect(nipInput().value).toBe("202509397");
    expect(nipInput().placeholder).toBe("mis. 202511001");
  });

  it("ubah NIP: nilai baru (dipangkas) dikirim ke server", async () => {
    state.accountRole = null;
    render(<UserFormPage mode="edit" employeeId={EMP_ID} />);
    fireEvent.change(nipInput(), { target: { value: " 202511001 " } });
    fireEvent.click(saveButton());
    await waitFor(() => expect(state.update).toHaveBeenCalled());
    expect(state.update.mock.calls[0][0]).toMatchObject({ id: EMP_ID, nip: "202511001" });
  });

  it("NIP dikosongkan: error di kolom NIP dan tidak dikirim", async () => {
    state.accountRole = null;
    render(<UserFormPage mode="edit" employeeId={EMP_ID} />);
    fireEvent.change(nipInput(), { target: { value: "  " } });
    fireEvent.click(saveButton());
    expect(await screen.findByText("NIP wajib diisi")).toBeInTheDocument();
    expect(nipInput()).toHaveAttribute("aria-invalid", "true");
    expect(state.update).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith("NIP wajib diisi");
    // mengetik lagi menghapus error
    fireEvent.change(nipInput(), { target: { value: "1" } });
    expect(screen.queryByText("NIP wajib diisi")).not.toBeInTheDocument();
  });

  it("bentrok NIP dari server tampil di kolom NIP", async () => {
    state.accountRole = null;
    state.update.mockRejectedValue(new Error("NIP sudah dipakai karyawan Budi (NIP 1, aktif)"));
    render(<UserFormPage mode="edit" employeeId={EMP_ID} />);
    fireEvent.click(saveButton());
    expect(await screen.findByText("NIP sudah dipakai karyawan Budi (NIP 1, aktif)")).toBeInTheDocument();
  });
});
