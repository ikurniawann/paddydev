"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowLeft, Loader2, ShieldCheck } from "lucide-react";
import { toast } from "sonner";
import {
  FormPageBody,
  FormPageFooter,
  FormPageLayout,
  FormPageLoading,
} from "@/components/layout/form-page-layout";
import { FormSectionCard } from "@/components/layout/form-section-card";
import { Button } from "@/components/ui/button";
import { useIamAccess } from "@/components/iam/iam-access-provider";
import { useAuth } from "@/hooks/use-auth";
import { useBusinessTree } from "@/features/configuration/business";
import type { BusinessTree } from "@/features/configuration/business/types";
import { IAM } from "@/lib/iam/prefixes";
import {
  buildUserPayload,
  userFormFromItem,
  validateUserForm,
  type UserEmployeeFormValues,
} from "@/lib/hris/users-form";
import type { CreateUserEmployeeInput, UserEmployeeItem } from "../types";
import { EMPLOYEES_ROUTES, emptyUserForm, withAssignableRole } from "../constants";
import { useCreateUser, useUpdateUser } from "../mutations";
import { fetchEmailOwner } from "../api";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { emailCheck } from "@/lib/hris/email-release";
import { useUserDetail, useUserFormLookups } from "../queries";
import { AppAccessFormSection } from "./app-access-form-section";
import { ResetPasswordDialog } from "./reset-password-dialog";
import { ContactSections } from "./user-form/contact-sections";
import { EmploymentSection } from "./user-form/employment-section";
import { PersonalSection } from "./user-form/personal-section";

const EMPTY_TREE: BusinessTree = { holdings: [] };

interface UserFormPageProps {
  mode: "create" | "edit";
  employeeId?: string;
}

/** Halaman Add/Edit Employee. Mode edit menunggu detail dulu supaya form terisi sejak mount. */
export function UserFormPage({ mode, employeeId }: UserFormPageProps) {
  const editId = mode === "edit" && employeeId ? employeeId : null;
  const { data: detailRes, isLoading } = useUserDetail(editId);

  if (mode === "edit" && isLoading) return <FormPageLoading />;

  return (
    <UserEmployeeForm key={detailRes?.data.id ?? "new"} editId={editId} detail={detailRes?.data} />
  );
}

function UserEmployeeForm({
  editId,
  detail,
}: {
  editId: string | null;
  detail: UserEmployeeItem | undefined;
}) {
  const router = useRouter();
  const isEdit = editId !== null;
  // Akun login (akses aplikasi, role, scope, password) hanya untuk Manajemen User; tanpa itu
  // (mis. Admin HRIS lewat HRIS → Karyawan) form hanya mengubah data karyawan.
  const canManageAccounts = useIamAccess().hasPrefix(IAM.settingsUsers);
  // Role HRIS (EPIC-061): server menolak key hris_role dari selain super_admin.
  const canSetHrisRole = useAuth().user?.role === "super_admin";
  // Role sistem (Super Admin/Admin) juga hanya oleh super_admin; server tetap penentu.
  const canAssignSystemRoles = canSetHrisRole;
  const { data: businessTree = EMPTY_TREE } = useBusinessTree(canManageAccounts);
  const { data: lookups } = useUserFormLookups();
  const createMutation = useCreateUser();
  const updateMutation = useUpdateUser();

  const [form, setForm] = useState<UserEmployeeFormValues>(() => {
    const initial = detail ? userFormFromItem(detail) : emptyUserForm;
    // Akun baru: role sistem bawaan dikosongkan bagi selain super_admin.
    return detail?.userId ? initial : withAssignableRole(initial, canAssignSystemRoles);
  });
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const [releasePrompt, setReleasePrompt] = useState<string | null>(null);

  const hasExistingAppAccount = Boolean(detail?.userId);
  const isSubmitting = createMutation.isPending || updateMutation.isPending;
  const backHref = editId ? EMPLOYEES_ROUTES.detail(editId) : EMPLOYEES_ROUTES.list;

  function setField(patch: Partial<UserEmployeeFormValues>) {
    setForm((prev) => ({ ...prev, ...patch }));
  }

  async function handleSubmit(releaseEmail = false) {
    const values = canManageAccounts ? form : { ...form, is_access_app: false };
    const invalid = validateUserForm(values, { isEdit, hasExistingAppAccount });
    if (invalid) {
      toast.error(invalid);
      return;
    }

    // Tambah karyawan: email karyawan aktif ditolak, email karyawan resign
    // perlu konfirmasi "lepas email" (EPIC-056).
    if (!isEdit && !releaseEmail) {
      try {
        const check = emailCheck((await fetchEmailOwner(values.email)).data);
        if (check.kind === "taken") {
          toast.error(check.message);
          return;
        }
        if (check.kind === "release") {
          setReleasePrompt(check.message);
          return;
        }
      } catch {
        // pemeriksaan gagal: biarkan server yang memutuskan (409)
      }
    }

    const payload = buildUserPayload(values, isEdit, { canSetHrisRole });
    if (releaseEmail) payload.release_email = true;
    // Tanpa Manajemen User: tidak ada field akun sama sekali saat edit (akun yang ada tidak disentuh).
    if (!canManageAccounts && isEdit) delete payload.is_access_app;
    try {
      if (editId) {
        const res = await updateMutation.mutateAsync({
          id: editId,
          ...payload,
        });
        toast.success(res.message || "Employee updated successfully");
        router.push(EMPLOYEES_ROUTES.detail(editId));
      } else {
        const res = await createMutation.mutateAsync(payload as CreateUserEmployeeInput);
        toast.success(res.message || "Employee created successfully");
        router.push(EMPLOYEES_ROUTES.detail(res.data.id));
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save data");
    }
  }

  return (
    <FormPageLayout>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-4">
          <Link href={backHref}>
            <Button variant="ghost" size="icon" className="h-9 w-9 text-gray-600">
              <ArrowLeft className="h-5 w-5" />
            </Button>
          </Link>
          <div>
            <h1 className="text-2xl font-bold text-gray-900">
              {isEdit ? "Edit Employee" : "Add Employee"}
            </h1>
            <p className="text-sm text-gray-500">Employment data and app access in one form</p>
          </div>
        </div>
        <Link href={backHref}>
          <Button variant="outline" className="h-10 rounded-lg border-gray-200/80">
            Back
          </Button>
        </Link>
      </div>

      <FormPageBody>
        <PersonalSection form={form} onChange={setField} emailLocked={!canManageAccounts && hasExistingAppAccount} />
        <EmploymentSection form={form} lookups={lookups} onChange={setField} isEdit={isEdit} />
        <ContactSections form={form} onChange={setField} />

        {canManageAccounts ? (
          <FormSectionCard
            icon={ShieldCheck}
            title="App Access"
            description="Paddy login, role, business scope, and approval permissions."
            bodyClassName="p-0"
          >
            <div className="px-5 py-5">
              <AppAccessFormSection
                form={form}
                businessTree={businessTree}
                isEdit={isEdit}
                hasExistingAppAccount={hasExistingAppAccount}
                canSetHrisRole={canSetHrisRole}
                canAssignSystemRoles={canAssignSystemRoles}
                onChange={setField}
                onResetPassword={
                  isEdit && hasExistingAppAccount ? () => setResetDialogOpen(true) : undefined
                }
              />
            </div>
          </FormSectionCard>
        ) : (
          <FormSectionCard icon={ShieldCheck} title="App Access" description="Akun login karyawan.">
            <p className="text-sm text-muted-foreground">
              {hasExistingAppAccount ? "Karyawan ini sudah punya akun login. " : ""}
              Akun login (akses aplikasi, role, password) dikelola admin di Settings → Manajemen User.
            </p>
          </FormSectionCard>
        )}
      </FormPageBody>

      <FormPageFooter>
        <Button
          variant="outline"
          onClick={() => router.push(backHref)}
          disabled={isSubmitting}
          className="h-10 rounded-lg border-gray-200/80"
        >
          Cancel
        </Button>
        <Button
          onClick={() => void handleSubmit()}
          disabled={isSubmitting}
          className="h-10 gap-2 rounded-lg bg-pink-600 px-4 text-sm font-semibold text-white shadow-sm hover:bg-pink-700"
        >
          {isSubmitting ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
          {isSubmitting ? "Saving..." : isEdit ? "Save Changes" : "Add Employee"}
        </Button>
      </FormPageFooter>

      <ConfirmDialog
        open={releasePrompt !== null}
        onOpenChange={(open) => !open && setReleasePrompt(null)}
        title="Email milik karyawan resign"
        confirmLabel="Lepas email & simpan"
        variant="default"
        loading={isSubmitting}
        onConfirm={() => {
          setReleasePrompt(null);
          void handleSubmit(true);
        }}
      >
        <p className="text-sm text-gray-700">{releasePrompt}</p>
        <p className="mt-2 text-xs text-gray-500">
          Email data lama diganti alamat arsip; riwayat data lama tetap tersimpan. Bila data lama punya akun login, akun itu
          dipindah ke karyawan baru.
        </p>
      </ConfirmDialog>

      {editId && canManageAccounts ? (
        <ResetPasswordDialog
          target={{
            id: editId,
            fullName: form.full_name || detail?.fullName || "Employee",
          }}
          open={resetDialogOpen}
          onOpenChange={setResetDialogOpen}
        />
      ) : null}
    </FormPageLayout>
  );
}
