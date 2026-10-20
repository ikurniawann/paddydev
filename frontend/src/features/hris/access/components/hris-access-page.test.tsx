import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { HrisAccessPage } from "./hris-access-page";

const api = vi.hoisted(() => {
  const view = {
    roles: [
      { code: "hris_manager", name: "HR Manager" },
      { code: "hris_non_payroll", name: "HR Staff (Non-Payroll)" },
    ],
    menus: [
      { code: "hris.workforce.attendance", name: "Absensi", parent_code: "hris.workforce", level: 3, access_toggle: true, delete_toggle: true },
    ],
    grants: {
      hris_manager: { "hris.workforce.attendance": ["delete", "read"] },
      hris_non_payroll: { "hris.workforce.attendance": ["read"] },
    },
    accounts: [{ user_id: "u1", full_name: "Rina HR", email: "rina@paddy.test", hris_role: "hris_non_payroll" }],
  };
  return {
    fetchHrisAccess: vi.fn(async () => view),
    fetchPermissionAudit: vi.fn(async () => []),
    saveHrisRoleAccess: vi.fn(async () => view),
    saveHrisAccount: vi.fn(async () => view),
  };
});
vi.mock("../api", () => api);
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
afterEach(cleanup);

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <HrisAccessPage />
    </QueryClientProvider>
  );
}

it("memberi hak hapus Absensi ke Non-Payroll", async () => {
  renderPage();
  const box = (await screen.findByLabelText("HR Staff (Non-Payroll) — hapus Absensi")) as HTMLInputElement;
  expect(box.checked).toBe(false);
  expect((screen.getByLabelText("HR Manager — hapus Absensi") as HTMLInputElement).checked).toBe(true);
  fireEvent.click(box);
  await waitFor(() =>
    expect(api.saveHrisRoleAccess).toHaveBeenCalledWith("hris_non_payroll", [{ menu_code: "hris.workforce.attendance", delete: true }])
  );
});

it("mengganti role HRIS sebuah akun", async () => {
  renderPage();
  fireEvent.change(await screen.findByLabelText("Role HRIS Rina HR"), { target: { value: "hris_manager" } });
  await waitFor(() => expect(api.saveHrisAccount).toHaveBeenCalledWith("u1", "hris_manager"));
});
