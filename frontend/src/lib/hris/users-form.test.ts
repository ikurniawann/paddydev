import { describe, expect, it } from "vitest";
import { mapEmployeeUserRow, type EmployeeUserRow } from "@/lib/users/user-mapper";
import {
  buildUserPayload,
  emptyUserForm,
  userFormFromItem,
  validateUserForm,
  type UserEmployeeFormValues,
} from "./users-form";

const HOLDING = "00000000-0000-4000-8000-0000000000a1";
const COMPANY = "00000000-0000-4000-8000-0000000000b1";
const BRANCH = "00000000-0000-4000-8000-0000000000c1";
const STALL = "00000000-0000-4000-8000-0000000000d1";

const baseForm: UserEmployeeFormValues = {
  ...emptyUserForm,
  full_name: "Budi Santoso",
  email: "budi@example.com",
  join_date: "2026-01-01",
  employment_status: "permanent",
};

const withAccess = (patch: Partial<UserEmployeeFormValues> = {}): UserEmployeeFormValues => ({
  ...baseForm,
  is_access_app: true,
  password: "rahasia123",
  role: "admin",
  business_scope: "branch",
  holding_id: HOLDING,
  company_id: COMPANY,
  branch_id: BRANCH,
  default_warehouse_id: STALL,
  ...patch,
});

const create = { isEdit: false, hasExistingAppAccount: false };
const editWithAccount = { isEdit: true, hasExistingAppAccount: true };

describe("validateUserForm", () => {
  it("field inti wajib", () => {
    expect(validateUserForm({ ...baseForm, email: "" }, create)).toBe(
      "Full name, email, join date, and employment status are required"
    );
    expect(validateUserForm(baseForm, create)).toBeNull();
  });

  it("akun baru butuh password 8 karakter; edit akun lama boleh kosong", () => {
    expect(validateUserForm(withAccess({ password: "pendek" }), create)).toBe(
      "Password must be at least 8 characters for app access"
    );
    expect(validateUserForm(withAccess({ password: "" }), editWithAccount)).toBeNull();
  });

  it("scope bertingkat sesuai level", () => {
    expect(validateUserForm(withAccess({ business_scope: "" }), create)).toBe(
      "Data access scope is required"
    );
    expect(
      validateUserForm(withAccess({ business_scope: "holding", holding_id: "" }), create)
    ).toBe("Holding is required");
    expect(
      validateUserForm(withAccess({ business_scope: "company", company_id: "" }), create)
    ).toBe("Holding and company are required");
    expect(validateUserForm(withAccess({ branch_id: "" }), create)).toBe(
      "Holding, company, and branch are required"
    );
    expect(validateUserForm(withAccess({ default_warehouse_id: "" }), create)).toBe(
      "Pilih outlet default untuk scope branch"
    );
  });

  it("super admin tidak butuh scope", () => {
    expect(
      validateUserForm(withAccess({ role: "super_admin", business_scope: "" }), create)
    ).toBeNull();
  });

  it("workflow harus cocok dengan modul", () => {
    const form = withAccess({
      approval_permissions: [
        {
          module: "hris",
          workflow: "purchase_request",
          approval_level: "approver",
          approval_limit: null,
          is_active: true,
        },
      ],
    });
    expect(validateUserForm(form, create)).toBe("Approval workflow must match the selected module");
  });
});

describe("buildUserPayload", () => {
  it("tanpa akses: string kosong → null, data akun tidak ikut", () => {
    const payload = buildUserPayload({ ...baseForm, password: "x", role: "hrd" }, false);
    expect(payload).toMatchObject({
      full_name: "Budi Santoso",
      phone: null,
      is_active: true,
      is_access_app: false,
    });
    expect(payload).not.toHaveProperty("password");
    expect(payload).not.toHaveProperty("role");
    expect(payload).not.toHaveProperty("warehouse_ids");
  });

  it("dengan akses branch: stall default jadi satu-satunya stall", () => {
    const payload = buildUserPayload(withAccess({ warehouse_ids: ["lain"] }), false);
    expect(payload).toMatchObject({
      is_access_app: true,
      role: "admin",
      warehouse_ids: [STALL],
      default_warehouse_id: STALL,
      business_scope: "branch",
      branch_id: BRANCH,
      password: "rahasia123",
    });
  });

  it("edit dengan password kosong tidak mengirim password", () => {
    expect(buildUserPayload(withAccess({ password: "" }), true)).not.toHaveProperty("password");
    expect(buildUserPayload(withAccess({ password: "" }), false)).toHaveProperty("password", "");
  });

  it("super admin: scope dikosongkan", () => {
    const payload = buildUserPayload(withAccess({ role: "super_admin" }), false);
    expect(payload.business_scope).toBeNull();
  });

  it("role HRIS dikirim hanya bila berubah dan peran aplikasi hrd", () => {
    const loaded = withAccess({ role: "hrd", hris_role: "hris_non_payroll", hris_role_loaded: "hris_non_payroll" });
    expect(buildUserPayload(loaded, true)).not.toHaveProperty("hris_role");
    expect(buildUserPayload({ ...loaded, hris_role: "hris_manager" }, true)).toHaveProperty("hris_role", "hris_manager");
    expect(buildUserPayload({ ...loaded, hris_role: "" }, true)).toHaveProperty("hris_role", null);
    expect(buildUserPayload({ ...loaded, role: "admin", hris_role: "hris_manager" }, true)).not.toHaveProperty("hris_role");
  });

  it("role HRIS tidak pernah dikirim oleh yang bukan super_admin (server menolak key-nya)", () => {
    const changed = withAccess({ role: "hrd", hris_role: "hris_manager", hris_role_loaded: "hris_non_payroll" });
    expect(buildUserPayload(changed, true, { canSetHrisRole: false })).not.toHaveProperty("hris_role");
    expect(buildUserPayload(changed, true, { canSetHrisRole: true })).toHaveProperty("hris_role", "hris_manager");
  });
});

describe("userFormFromItem", () => {
  it("mengisi form dari detail API, hanya izin approval aktif", () => {
    const row = {
      id: "emp-1",
      user_id: "u-1",
      full_name: "Budi",
      nip: "EMP-1",
      email: "budi@example.com",
      phone: "0812",
      join_date: "2024-01-15T00:00:00Z",
      end_date: null,
      employment_status: "permanent",
      is_active: true,
      is_access_app: true,
      department_id: null,
      section_id: null,
      job_title_id: null,
      reporting_to: null,
      ktp: null,
      npwp: null,
      birth_date: null,
      gender: "male",
      marital_status: null,
      address: null,
      city: null,
      province: null,
      postal_code: null,
      bank_name: null,
      bank_account: null,
      bpjs_tk: null,
      bpjs_kesehatan: null,
      emergency_contact_name: null,
      emergency_contact_phone: null,
      emergency_contact_relationship: null,
      photo_url: null,
      notes: null,
      created_at: "2024-01-15T00:00:00Z",
      updated_at: null,
      app_user: {
        id: "u-1",
        role: "hrd",
        status: "active",
        brand_id: null,
        hris_role: "hris_manager",
        business_scope: "branch",
        holding_id: HOLDING,
        company_id: COMPANY,
        branch_id: BRANCH,
        user_warehouses: [
          {
            id: "uw",
            warehouse_id: STALL,
            name: "Stall",
            code: "S1",
            branch_id: BRANCH,
          },
        ],
        user_approval_permissions: [
          {
            id: "p1",
            module: "hris",
            workflow: "leave_request",
            approval_level: "approver",
            approval_limit: null,
            is_active: true,
          },
          {
            id: "p2",
            module: "hris",
            workflow: "leave_request",
            approval_level: "checker",
            approval_limit: null,
            is_active: false,
          },
        ],
      },
    } satisfies EmployeeUserRow;
    const form = userFormFromItem(mapEmployeeUserRow(row));
    expect(form).toMatchObject({
      join_date: "2024-01-15",
      gender: "male",
      ktp: "",
      role: "hrd",
      hris_role: "hris_manager",
      hris_role_loaded: "hris_manager",
      business_scope: "branch",
      default_warehouse_id: STALL,
      warehouse_ids: [STALL],
      password: "",
    });
    expect(form.approval_permissions).toHaveLength(1);
    expect(form.approval_permissions[0].id).toBe("p1");
  });
});

describe("atasan = diri sendiri (EPIC-063 P11)", () => {
  const SELF = "3f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b";
  const OTHER = "9f2b8c1e-4a5d-4e6f-8a9b-0c1d2e3f4a5b";
  const item = (reportingTo: string | null) =>
    ({ id: SELF, fullName: "Retail Ops", email: "r@example.com", isActive: true, reportingTo }) as unknown as Parameters<
      typeof userFormFromItem
    >[0];

  it("nilai lama atasan = diri sendiri tidak diisikan ke form, jadi tidak dikirim ulang", () => {
    for (const stale of [SELF, SELF.toUpperCase(), SELF.replaceAll("-", "")]) {
      const form = userFormFromItem(item(stale));
      expect(form.reporting_to).toBe("");
      expect(buildUserPayload(form, true).reporting_to).toBeNull();
    }
  });

  it("atasan lain tetap diisikan dan dikirim", () => {
    const form = userFormFromItem(item(OTHER));
    expect(form.reporting_to).toBe(OTHER);
    expect(buildUserPayload(form, true).reporting_to).toBe(OTHER);
  });
});

describe("tipe jadwal", () => {
  it("default Shift dan wajib dipilih", () => {
    expect(emptyUserForm.schedule_type).toBe("shift");
    const form = { ...emptyUserForm, full_name: "A", email: "a@b.id", join_date: "2026-10-21", employment_status: "contract" };
    expect(validateUserForm({ ...form, schedule_type: "" as never }, { isEdit: false, hasExistingAppAccount: false })).toBe(
      "Tipe jadwal wajib dipilih"
    );
  });
  it("dikirim hanya saat tambah karyawan", () => {
    const form = { ...emptyUserForm, schedule_type: "non_shift" as const };
    expect(buildUserPayload(form, false).schedule_type).toBe("non_shift");
    expect("schedule_type" in buildUserPayload(form, true)).toBe(false);
  });
});
