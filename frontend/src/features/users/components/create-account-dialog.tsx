"use client";

import { useState } from "react";
import { CheckIcon, ClipboardIcon, KeyIcon } from "@heroicons/react/24/outline";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Combobox } from "@/components/ui/combobox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { useBusinessTree } from "@/features/configuration/business";
import { normalizeBusinessScopePayload } from "@/lib/configuration/business-scope";
import {
  findBranchStallsFromTree,
  requiresStallAssignment,
  shouldShowStallPicker,
} from "@/lib/users/stall-assignment";
import type { BusinessTree } from "@/features/configuration/business/types";
import type { UserRole } from "@/types";
import type { UserEmployeeFormValues } from "../types";
import { useAuth } from "@/hooks/use-auth";
import { emptyUserForm, roleOptionsFor, withAssignableRole } from "../constants";
import { useUpdateUser } from "../mutations";
import { BusinessScopePicker } from "./business-scope-picker";
import { StallAssignmentPicker } from "./stall-assignment-picker";

interface CreateAccountDialogProps {
  employee: { id: string; full_name: string; email: string };
  onClose: () => void;
}

const EMPTY_TREE: BusinessTree = { holdings: [] };

function generatePassword() {
  const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789";
  let pass = "";
  const bytes = new Uint32Array(11);
  crypto.getRandomValues(bytes);
  for (const b of bytes) pass += chars[b % chars.length];
  return `${pass.slice(0, 8)}!${pass.slice(8)}`;
}

/**
 * Dialog "Buat Akun Login" utk karyawan yang belum punya akses aplikasi —
 * Super Admin / Admin / HRD menentukan password, role, dan scope data.
 * Memakai jalur provisioning yang sama dgn halaman edit
 * (PUT /api/users/[id] dgn is_access_app: true); kredensial ditampilkan
 * sekali utk disalin & dibagikan ke karyawan.
 */
export function CreateAccountDialog({ employee, onClose }: CreateAccountDialogProps) {
  const updateUser = useUpdateUser();
  const { data: businessTreeData } = useBusinessTree();
  const businessTree = businessTreeData ?? EMPTY_TREE;
  // Role Super Admin/Admin hanya oleh super_admin (server tetap penentu).
  const canAssignSystemRoles = useAuth().user?.role === "super_admin";

  const [form, setForm] = useState<UserEmployeeFormValues>(() =>
    withAssignableRole(
      { ...emptyUserForm, is_access_app: true, password: generatePassword() },
      canAssignSystemRoles
    )
  );
  // email login bisa disetel ulang oleh HRD / Super Admin (default email karyawan)
  const [loginEmail, setLoginEmail] = useState(employee.email);
  const [createdCredentials, setCreatedCredentials] = useState<{
    email: string;
    password: string;
  } | null>(null);
  const [copied, setCopied] = useState(false);

  const setField = (patch: Partial<UserEmployeeFormValues>) => setForm((f) => ({ ...f, ...patch }));

  const showStalls = shouldShowStallPicker({
    isAccessApp: true,
    role: form.role,
    businessScope: form.business_scope || null,
    branchId: form.branch_id || null,
  });
  const stallOptions = form.branch_id ? findBranchStallsFromTree(businessTree, form.branch_id) : [];

  const handleSubmit = () => {
    const email = loginEmail.trim().toLowerCase();
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      toast.error("Format email login tidak valid");
      return;
    }
    if (!form.password || form.password.length < 8) {
      toast.error("Password minimal 8 karakter");
      return;
    }
    if (!form.role) {
      toast.error("Pilih role akun");
      return;
    }
    const scope = normalizeBusinessScopePayload(
      form.role === "super_admin" ? null : form.business_scope || null,
      form.holding_id || null,
      form.company_id || null,
      form.branch_id || null
    );
    if (form.role !== "super_admin" && !scope.business_scope) {
      toast.error("Scope akses data wajib dipilih untuk role ini");
      return;
    }
    if (
      requiresStallAssignment(form.role, scope.business_scope ?? null, true) &&
      !form.default_warehouse_id
    ) {
      toast.error("Pilih outlet default untuk scope branch");
      return;
    }

    updateUser.mutate(
      {
        id: employee.id,
        // email login sekaligus memperbarui email karyawan (satu identitas)
        ...(email !== employee.email.trim().toLowerCase() ? { email } : {}),
        is_access_app: true,
        password: form.password,
        role: form.role,
        account_status: "active",
        warehouse_ids: form.default_warehouse_id ? [form.default_warehouse_id] : [],
        default_warehouse_id: form.default_warehouse_id || null,
        can_switch_stall: form.can_switch_stall,
        can_central_checkout: form.can_central_checkout,
        ...scope,
      },
      {
        onSuccess: () => {
          setCreatedCredentials({ email, password: form.password });
          toast.success(`Akun login untuk ${employee.full_name} berhasil dibuat`);
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : "Gagal membuat akun"),
      }
    );
  };

  const handleCopyCredentials = async () => {
    if (!createdCredentials) return;
    await navigator.clipboard
      .writeText(`Email: ${createdCredentials.email}\nPassword: ${createdCredentials.password}`)
      .catch(() => undefined);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[85vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <KeyIcon className="h-5 w-5" /> Buat Akun Login
          </DialogTitle>
        </DialogHeader>

        {createdCredentials ? (
          <div className="space-y-3 py-2">
            <p className="text-sm text-emerald-700">
              Akun untuk <b>{employee.full_name}</b> berhasil dibuat. Salin kredensial berikut dan
              bagikan ke karyawan — password tidak akan ditampilkan lagi setelah dialog ditutup.
            </p>
            <div className="space-y-2 rounded-lg border border-gray-200 bg-gray-50 p-4 font-mono text-sm">
              <p>
                <span className="text-gray-500">Email:</span> {createdCredentials.email}
              </p>
              <p>
                <span className="text-gray-500">Password:</span> {createdCredentials.password}
              </p>
            </div>
            <Button
              type="button"
              variant="outline"
              className="gap-1"
              onClick={handleCopyCredentials}
            >
              {copied ? (
                <CheckIcon className="h-4 w-4 text-emerald-600" />
              ) : (
                <ClipboardIcon className="h-4 w-4" />
              )}
              Salin Kredensial
            </Button>
          </div>
        ) : (
          <div className="space-y-4 py-2">
            <div>
              <label className="text-xs font-medium text-gray-600">Email Login</label>
              <Input
                type="email"
                value={loginEmail}
                onChange={(e) => setLoginEmail(e.target.value)}
                className="mt-1"
                placeholder="email@perusahaan.com"
              />
              <p className="mt-1 text-xs text-gray-400">
                Default mengikuti email karyawan — boleh disetel ulang; email karyawan ikut
                diperbarui.
              </p>
            </div>
            <div className="flex items-end gap-2">
              <div className="flex-1">
                <label className="text-xs font-medium text-gray-600">Password</label>
                <Input
                  value={form.password}
                  onChange={(e) => setField({ password: e.target.value })}
                  className="mt-1 font-mono"
                />
              </div>
              <Button
                type="button"
                variant="outline"
                onClick={() => setField({ password: generatePassword() })}
              >
                Generate
              </Button>
            </div>
            <div>
              <label className="text-xs font-medium text-gray-600">Role</label>
              <Combobox
                options={roleOptionsFor(canAssignSystemRoles)}
                value={form.role}
                onChange={(value) =>
                  setField({
                    role: value as UserRole,
                    warehouse_ids: [],
                    default_warehouse_id: "",
                    can_switch_stall: false,
                    can_central_checkout: false,
                  })
                }
                placeholder="Pilih role"
                searchPlaceholder="Cari role..."
                emptyMessage="Role tidak ditemukan"
                className="!w-full mt-1 h-9 text-sm"
              />
            </div>
            {form.role !== "super_admin" && (
              <BusinessScopePicker form={form} tree={businessTree} onChange={setField} />
            )}
            {showStalls && (
              <StallAssignmentPicker
                stalls={stallOptions}
                defaultWarehouseId={form.default_warehouse_id}
                canSwitchStall={form.can_switch_stall}
                canCentralCheckout={form.can_central_checkout}
                showCentralCheckout
                required={requiresStallAssignment(form.role, form.business_scope || null, true)}
                onChange={(patch) => setField(patch)}
              />
            )}
            <p className="text-xs text-gray-400">
              Pengaturan lanjutan (izin approval, status nonaktif) tersedia di halaman Edit karyawan
              setelah akun dibuat.
            </p>
          </div>
        )}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {createdCredentials ? "Tutup" : "Batal"}
          </Button>
          {!createdCredentials && (
            <Button type="button" onClick={handleSubmit} disabled={updateUser.isPending}>
              {updateUser.isPending && <Loader2 className="h-4 w-4 animate-spin" />}
              Buat Akun
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
