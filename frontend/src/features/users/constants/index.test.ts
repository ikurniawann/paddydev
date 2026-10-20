import { describe, it, expect } from "vitest";
import {
  EMPLOYEES_ROUTES,
  emptyUserForm,
  formatCurrency,
  isSystemRole,
  levelLabel,
  roleOptionsFor,
  withAssignableRole,
  moduleLabel,
  workflowLabel,
  workflowsForModule,
} from "./index";

describe("users constants helpers", () => {
  it("EMPLOYEES_ROUTES follow insert/edit convention", () => {
    expect(EMPLOYEES_ROUTES.insert).toBe("/dashboard/employees/insert");
    expect(EMPLOYEES_ROUTES.edit("x")).toBe("/dashboard/employees/edit/x");
  });

  it("emptyUserForm has safe defaults", () => {
    expect(emptyUserForm.is_access_app).toBe(false);
    expect(emptyUserForm.account_status).toBe("active");
    expect(emptyUserForm.approval_permissions).toEqual([]);
    expect(emptyUserForm.role).toBe("admin");
    expect(emptyUserForm.can_switch_stall).toBe(false);
    expect(emptyUserForm.can_central_checkout).toBe(false);
    expect(emptyUserForm.default_warehouse_id).toBe("");
  });

  it("workflowsForModule returns only matching workflows", () => {
    const hrisWorkflows = workflowsForModule("hris");
    expect(hrisWorkflows.every((w) => w.module === "hris")).toBe(true);
    expect(hrisWorkflows.some((w) => w.value === "leave_request")).toBe(true);
  });

  it("labels resolve known values", () => {
    expect(moduleLabel("purchasing")).toBe("Purchasing");
    expect(workflowLabel("purchase_request")).toBe("Purchase Request");
    expect(levelLabel("approver")).toBe("Approver");
  });

  it("formatCurrency formats IDR and handles null", () => {
    expect(formatCurrency(null)).toBe("Tanpa limit");
    expect(formatCurrency(1500000)).toContain("1.500.000");
  });
});

describe("role sistem hanya Super Admin (temuan lanjutan EPIC-061)", () => {
  const values = (opts: { value: string }[]) => opts.map((o) => o.value);

  it("isSystemRole: super_admin dan admin", () => {
    expect(isSystemRole("super_admin")).toBe(true);
    expect(isSystemRole("admin")).toBe(true);
    expect(isSystemRole("hrd")).toBe(false);
    expect(isSystemRole("")).toBe(false);
  });

  it("Super Admin melihat semua role", () => {
    expect(values(roleOptionsFor(true))).toContain("super_admin");
    expect(values(roleOptionsFor(true))).toContain("admin");
  });

  it("selain Super Admin: opsi role sistem disembunyikan", () => {
    const opts = values(roleOptionsFor(false));
    expect(opts).not.toContain("super_admin");
    expect(opts).not.toContain("admin");
    expect(opts).toContain("hrd");
  });

  it("role sistem yang sedang dipegang tetap tampil sebagai label", () => {
    const opts = values(roleOptionsFor(false, "super_admin"));
    expect(opts).toContain("super_admin");
    expect(opts).not.toContain("admin");
  });

  it("withAssignableRole mengosongkan role sistem bawaan bagi selain Super Admin", () => {
    expect(withAssignableRole(emptyUserForm, false).role).toBe("");
    expect(withAssignableRole(emptyUserForm, true).role).toBe("admin");
    expect(withAssignableRole({ ...emptyUserForm, role: "hrd" }, false).role).toBe("hrd");
  });
});
