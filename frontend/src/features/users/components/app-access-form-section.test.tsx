// Role sistem di form user: selain Super Admin tidak bisa memilih/mengubahnya (server tetap penentu).
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { emptyUserForm } from "../constants";
import { AppAccessFormSection } from "./app-access-form-section";

const NOTE = "Role Super Admin/Admin hanya bisa diatur oleh Super Admin.";

function renderSection(role: string, canAssignSystemRoles: boolean) {
  render(
    <AppAccessFormSection
      form={{ ...emptyUserForm, is_access_app: true, role: role as typeof emptyUserForm.role }}
      businessTree={{ holdings: [] }}
      isEdit
      hasExistingAppAccount
      canAssignSystemRoles={canAssignSystemRoles}
      onChange={vi.fn()}
    />
  );
  // Combobox pertama di bagian akun adalah Role.
  return screen.getAllByRole("combobox")[0];
}

describe("AppAccessFormSection — role sistem", () => {
  it("akun Super Admin dikunci bagi selain Super Admin", () => {
    const role = renderSection("super_admin", false);
    expect(role).toBeDisabled();
    expect(role).toHaveTextContent("Super Admin");
    expect(screen.getByText(NOTE)).toBeInTheDocument();
  });

  it("Super Admin tetap bisa mengubahnya", () => {
    const role = renderSection("super_admin", true);
    expect(role).not.toBeDisabled();
    expect(screen.queryByText(NOTE)).not.toBeInTheDocument();
  });

  it("akun biasa tetap bisa diubah selain Super Admin", () => {
    const role = renderSection("hrd", false);
    expect(role).not.toBeDisabled();
    expect(screen.queryByText(NOTE)).not.toBeInTheDocument();
  });
});
