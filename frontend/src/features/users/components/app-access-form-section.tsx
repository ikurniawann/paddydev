"use client";

import { PlusIcon, XMarkIcon } from "@heroicons/react/24/outline";
import { KeyRound } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Combobox } from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import type { BusinessTree } from "@/features/configuration/business/types";
import {
  FormFieldLabel,
  formComboboxClassName,
  formInputClassName,
} from "@/components/layout/form-field";
import type { UserEmployeeFormValues } from "../types";
import { APPROVAL_LEVELS, APPROVAL_MODULES } from "@/lib/admin/user-management";
import {
  HRIS_ROLE_OPTIONS,
  ROLE_OPTIONS,
  emptyApprovalPermission,
  formatCurrency,
  levelLabel,
  moduleLabel,
  workflowLabel,
  workflowsForModule,
} from "../constants";
import type { UserRole } from "@/types";
import { BusinessScopePicker } from "./business-scope-picker";
import { StallAssignmentPicker } from "./stall-assignment-picker";
import {
  findBranchStallsFromTree,
  requiresStallAssignment,
  shouldShowStallPicker,
} from "@/lib/users/stall-assignment";

interface AppAccessFormSectionProps {
  form: UserEmployeeFormValues;
  businessTree: BusinessTree;
  isEdit: boolean;
  hasExistingAppAccount?: boolean;
  /** Hanya super_admin yang boleh mengubah Role HRIS di sini (server menolak yang lain). */
  canSetHrisRole?: boolean;
  onChange: (patch: Partial<UserEmployeeFormValues>) => void;
  onResetPassword?: () => void;
}

const ACCOUNT_STATUS_OPTIONS = [
  { value: "active", label: "Active" },
  { value: "inactive", label: "Inactive" },
];

const MODULE_OPTIONS = APPROVAL_MODULES.map((m) => ({
  value: m.value,
  label: m.label,
}));
const LEVEL_OPTIONS = APPROVAL_LEVELS.map((l) => ({
  value: l.value,
  label: l.label,
}));

export function AppAccessFormSection({
  form,
  businessTree,
  isEdit,
  hasExistingAppAccount = false,
  canSetHrisRole = false,
  onChange,
  onResetPassword,
}: AppAccessFormSectionProps) {
  function updatePermission(
    index: number,
    patch: Partial<UserEmployeeFormValues["approval_permissions"][number]>
  ) {
    onChange({
      approval_permissions: form.approval_permissions.map((permission, itemIndex) => {
        if (itemIndex !== index) return permission;
        const next = { ...permission, ...patch };
        if (patch.module && patch.module !== permission.module) {
          next.workflow = workflowsForModule(patch.module)[0]?.value ?? "";
        }
        return next;
      }),
    });
  }

  function handleRoleChange(value: string) {
    const role = value as UserRole;
    if (role === "super_admin") {
      onChange({
        role,
        business_scope: "",
        holding_id: "",
        company_id: "",
        branch_id: "",
        warehouse_ids: [],
        default_warehouse_id: "",
        can_switch_stall: false,
        can_central_checkout: false,
      });
      return;
    }
    onChange({ role });
  }

  const showStallPicker = shouldShowStallPicker({
    isAccessApp: form.is_access_app,
    role: form.role,
    businessScope: form.business_scope || null,
    branchId: form.branch_id || null,
    isEdit,
  });

  const branchStalls = form.branch_id ? findBranchStallsFromTree(businessTree, form.branch_id) : [];

  const requiresNewPassword = form.is_access_app && (!isEdit || !hasExistingAppAccount);

  return (
    <div className="space-y-5">
      {isEdit && hasExistingAppAccount && form.is_access_app && onResetPassword ? (
        <div className="flex flex-col gap-3 rounded-lg border border-gray-200/70 bg-white px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-sm font-semibold text-gray-900">Account password</p>
            <p className="mt-1 text-xs text-gray-500">
              Generate a new temporary password if the employee cannot sign in.
            </p>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-9 shrink-0 gap-1.5 rounded-lg border-gray-200/80"
            onClick={onResetPassword}
          >
            <KeyRound className="h-4 w-4" />
            Reset Password
          </Button>
        </div>
      ) : null}

      <div className="flex items-start gap-3 rounded-lg border border-gray-200/70 bg-gray-50/50 p-4">
        <Checkbox
          id="is_access_app"
          checked={form.is_access_app}
          onCheckedChange={(checked) => onChange({ is_access_app: checked === true })}
        />
        <div>
          <label
            htmlFor="is_access_app"
            className="cursor-pointer text-sm font-semibold text-gray-900"
          >
            Enable Paddy login access
          </label>
          <p className="mt-1 text-xs text-gray-500">
            Employee can sign in with the assigned role and data scope.
          </p>
        </div>
      </div>

      {form.is_access_app ? (
        <div className="space-y-5">
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            {!requiresNewPassword ? (
              <div>
                <FormFieldLabel>New password (optional)</FormFieldLabel>
                <Input
                  type="password"
                  value={form.password}
                  onChange={(e) => onChange({ password: e.target.value })}
                  placeholder="Leave blank to keep current password"
                  className={formInputClassName}
                />
              </div>
            ) : (
              <div>
                <FormFieldLabel required>Temporary password</FormFieldLabel>
                <Input
                  type="password"
                  value={form.password}
                  onChange={(e) => onChange({ password: e.target.value })}
                  placeholder="At least 8 characters"
                  className={formInputClassName}
                />
              </div>
            )}
            <div>
              <FormFieldLabel required>Role</FormFieldLabel>
              <Combobox
                options={ROLE_OPTIONS}
                value={form.role}
                onChange={handleRoleChange}
                placeholder="Select role"
                searchPlaceholder="Search role..."
                emptyMessage="No role found"
                className={formComboboxClassName}
              />
            </div>
            {form.role === "hrd" ? (
              <div>
                <FormFieldLabel>Role HRIS</FormFieldLabel>
                <Combobox
                  options={HRIS_ROLE_OPTIONS}
                  value={form.hris_role || "auto"}
                  onChange={(value) => onChange({ hris_role: value === "auto" ? "" : value })}
                  placeholder="Pilih role HRIS"
                  searchPlaceholder="Cari role..."
                  emptyMessage="Role tidak ditemukan"
                  disabled={!canSetHrisRole}
                  className={formComboboxClassName}
                />
                <p className="mt-1 text-xs text-gray-500">
                  Hanya Super Admin yang bisa mengubah di sini; AdminHRIS memakai HRIS → Hak Akses HRIS.
                </p>
              </div>
            ) : null}
            <div>
              <FormFieldLabel>Account status</FormFieldLabel>
              <Combobox
                options={ACCOUNT_STATUS_OPTIONS}
                value={form.account_status}
                onChange={(value) => onChange({ account_status: value as "active" | "inactive" })}
                placeholder="Select status"
                searchPlaceholder="Search status..."
                emptyMessage="No status found"
                className={formComboboxClassName}
              />
            </div>
          </div>

          <BusinessScopePicker
            form={form}
            tree={businessTree}
            isEdit={isEdit}
            onChange={onChange}
          />

          {showStallPicker ? (
            <div className="space-y-3">
              {form.role === "super_admin" ? (
                <p className="rounded-lg border border-amber-200/80 bg-amber-50/60 px-4 py-3 text-xs text-amber-800">
                  Outlet assignments are only saved when the role is changed from Super Admin.
                </p>
              ) : null}
              <StallAssignmentPicker
                stalls={branchStalls}
                defaultWarehouseId={form.default_warehouse_id}
                canSwitchStall={form.can_switch_stall}
                canCentralCheckout={form.can_central_checkout}
                showCentralCheckout
                required={requiresStallAssignment(form.role, form.business_scope || null, true)}
                onChange={(patch) => onChange(patch)}
              />
            </div>
          ) : null}

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <p className="text-sm font-semibold text-gray-900">Approval Authority</p>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-9 gap-1.5 rounded-lg border-gray-200/80"
                onClick={() =>
                  onChange({
                    approval_permissions: [
                      ...form.approval_permissions,
                      { ...emptyApprovalPermission },
                    ],
                  })
                }
              >
                <PlusIcon className="h-4 w-4" />
                Add
              </Button>
            </div>

            {form.approval_permissions.length === 0 ? (
              <p className="rounded-lg border border-dashed border-gray-200/70 py-6 text-center text-xs text-gray-400">
                No approval permissions yet
              </p>
            ) : (
              <div className="space-y-2">
                {form.approval_permissions.map((permission, index) => {
                  const workflowOptions = workflowsForModule(permission.module).map((workflow) => ({
                    value: workflow.value,
                    label: workflow.label,
                  }));

                  return (
                    <div key={index} className="rounded-lg border border-gray-200/70 bg-white p-3">
                      <div className="grid grid-cols-1 gap-2 md:grid-cols-[1fr_1fr_1fr_1fr_auto]">
                        <Combobox
                          options={MODULE_OPTIONS}
                          value={permission.module}
                          onChange={(value) => updatePermission(index, { module: value })}
                          placeholder="Module"
                          searchPlaceholder="Search module..."
                          emptyMessage="No module found"
                          className={formComboboxClassName}
                        />
                        <Combobox
                          options={workflowOptions}
                          value={permission.workflow}
                          onChange={(value) => updatePermission(index, { workflow: value })}
                          placeholder="Workflow"
                          searchPlaceholder="Search workflow..."
                          emptyMessage="No workflow found"
                          className={formComboboxClassName}
                        />
                        <Combobox
                          options={LEVEL_OPTIONS}
                          value={permission.approval_level}
                          onChange={(value) =>
                            updatePermission(index, {
                              approval_level: value as typeof permission.approval_level,
                            })
                          }
                          placeholder="Level"
                          searchPlaceholder="Search level..."
                          emptyMessage="No level found"
                          className={formComboboxClassName}
                        />
                        <Input
                          type="number"
                          min="0"
                          placeholder="Amount limit"
                          value={permission.approval_limit ?? ""}
                          onChange={(e) =>
                            updatePermission(index, {
                              approval_limit: e.target.value === "" ? null : Number(e.target.value),
                            })
                          }
                          className={formInputClassName}
                        />
                        <Button
                          type="button"
                          variant="ghost"
                          onClick={() =>
                            onChange({
                              approval_permissions: form.approval_permissions.filter(
                                (_, itemIndex) => itemIndex !== index
                              ),
                            })
                          }
                          className="h-10 w-10 p-0 text-red-500 hover:bg-red-50 hover:text-red-600"
                        >
                          <XMarkIcon className="h-4 w-4" />
                        </Button>
                      </div>
                      <p className="mt-2 text-xs text-gray-500">
                        {moduleLabel(permission.module)} / {workflowLabel(permission.workflow)} /{" "}
                        {levelLabel(permission.approval_level)} /{" "}
                        {formatCurrency(permission.approval_limit)}
                      </p>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}
