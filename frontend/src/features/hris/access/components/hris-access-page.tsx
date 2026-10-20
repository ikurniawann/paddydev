"use client";

import { useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { PageHeader } from "@/components/ui/page-header";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { fetchHrisAccess, fetchPermissionAudit, saveHrisAccount, saveHrisRoleAccess } from "../api";
import { hrisAccessQueryKeys } from "../query-keys";
import type { HrisAccessChange, HrisAccessMenu, HrisAccessRole, HrisAccessView, PermissionAuditRow } from "../types";

const SOURCE_LABEL: Record<PermissionAuditRow["source"], string> = {
  role_permissions: "Role Management",
  hris_access: "Hak Akses HRIS",
  hris_mask: "Masker role HRIS",
  user_role: "Penugasan role",
};

function actionsText(value: unknown): string {
  if (value == null) return "tidak ada";
  if (Array.isArray(value)) return value.join(", ") || "-";
  return JSON.stringify(value);
}

function auditText(row: PermissionAuditRow): string {
  if (row.source === "user_role") {
    return `${row.target_name ?? "Akun"}: ${(row.roles_before ?? []).join(", ") || "-"} → ${(row.roles_after ?? []).join(", ") || "-"}`;
  }
  return `${row.role_code ?? "-"} · ${row.menu_code ?? "-"}: ${actionsText(row.actions_before)} → ${actionsText(row.actions_after)}`;
}

function AccessCell({
  role,
  menu,
  actions,
  disabled,
  onChange,
}: {
  role: HrisAccessRole;
  menu: HrisAccessMenu;
  actions: string[] | undefined;
  disabled: boolean;
  onChange: (change: HrisAccessChange) => void;
}) {
  const has = actions !== undefined;
  return (
    <div className="flex flex-col gap-1 text-xs text-gray-700">
      {menu.access_toggle ? (
        <label className="inline-flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={has}
            disabled={disabled}
            aria-label={`${role.name} — akses ${menu.name}`}
            onChange={(e) => onChange({ menu_code: menu.code, access: e.target.checked })}
          />
          Akses
        </label>
      ) : null}
      {menu.delete_toggle ? (
        <label className="inline-flex items-center gap-1.5">
          <input
            type="checkbox"
            checked={actions?.includes("delete") ?? false}
            disabled={disabled || !has}
            aria-label={`${role.name} — hapus ${menu.name}`}
            onChange={(e) => onChange({ menu_code: menu.code, delete: e.target.checked })}
          />
          Hapus
        </label>
      ) : null}
    </div>
  );
}

/**
 * HRIS → Hak Akses HRIS (EPIC-061, RA-03): AdminHRIS mengatur akses
 * Penggajian, Rekrutmen, Kehadiran & Cuti dan hak hapus untuk role HR, serta
 * role HRIS tiap akun HR. Role lain dan menu non-HRIS diatur Super Admin.
 */
export function HrisAccessPage() {
  const qc = useQueryClient();
  const view = useQuery({ queryKey: hrisAccessQueryKeys.view(), queryFn: fetchHrisAccess });
  const audit = useQuery({ queryKey: hrisAccessQueryKeys.audit(), queryFn: fetchPermissionAudit });
  const saved = (data: HrisAccessView) => {
    qc.setQueryData(hrisAccessQueryKeys.view(), data);
    void qc.invalidateQueries({ queryKey: hrisAccessQueryKeys.audit() });
    toast.success("Hak akses disimpan");
  };
  const roleMutation = useMutation({
    mutationFn: ({ role, change }: { role: string; change: HrisAccessChange }) => saveHrisRoleAccess(role, [change]),
    retry: false,
    onSuccess: saved,
    onError: (err) => toast.error(err.message || "Gagal menyimpan hak akses"),
  });
  const accountMutation = useMutation({
    mutationFn: ({ userId, role }: { userId: string; role: string }) => saveHrisAccount(userId, role),
    retry: false,
    onSuccess: saved,
    onError: (err) => toast.error(err.message || "Gagal menyimpan role akun"),
  });

  useEffect(() => {
    if (view.isError) toast.error("Gagal memuat Hak Akses HRIS");
  }, [view.isError]);

  const data = view.data;
  const busy = roleMutation.isPending || accountMutation.isPending;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Hak Akses HRIS"
        description="Atur akses Penggajian, Rekrutmen, Kehadiran & Cuti, dan hak hapus untuk role HR. Role lain diatur Super Admin di Role Management."
      />
      {!data ? (
        <p className="text-sm text-muted-foreground">{view.isLoading ? "Memuat..." : "Data tidak tersedia"}</p>
      ) : (
        <>
          <section className="rounded-lg border border-gray-200/70 bg-white">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Menu</TableHead>
                  {data.roles.map((role) => (
                    <TableHead key={role.code}>{role.name}</TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.menus.map((menu) => (
                  <TableRow key={menu.code}>
                    <TableCell className={menu.level > 2 ? "pl-6" : "font-medium"}>
                      {menu.name}
                      <span className="block text-xs text-gray-400">{menu.code}</span>
                    </TableCell>
                    {data.roles.map((role) => (
                      <TableCell key={role.code}>
                        <AccessCell
                          role={role}
                          menu={menu}
                          actions={data.grants[role.code]?.[menu.code]}
                          disabled={busy}
                          onChange={(change) => roleMutation.mutate({ role: role.code, change })}
                        />
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </section>

          <section className="space-y-2">
            <h2 className="text-base font-semibold text-gray-900">Akun HR</h2>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Nama</TableHead>
                  <TableHead>Email</TableHead>
                  <TableHead>Role HRIS</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.accounts.map((account) => (
                  <TableRow key={account.user_id}>
                    <TableCell>{account.full_name}</TableCell>
                    <TableCell>{account.email ?? "-"}</TableCell>
                    <TableCell>
                      <select
                        aria-label={`Role HRIS ${account.full_name}`}
                        className="h-9 rounded-lg border border-gray-200 px-2 text-sm"
                        value={account.hris_role ?? ""}
                        disabled={busy}
                        onChange={(e) => {
                          if (e.target.value) accountMutation.mutate({ userId: account.user_id, role: e.target.value });
                        }}
                      >
                        <option value="" disabled>
                          Belum ada role HRIS
                        </option>
                        {data.roles.map((role) => (
                          <option key={role.code} value={role.code}>
                            {role.name}
                          </option>
                        ))}
                      </select>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </section>
        </>
      )}

      <section className="space-y-2">
        <h2 className="text-base font-semibold text-gray-900">Riwayat perubahan</h2>
        {(audit.data ?? []).length === 0 ? (
          <p className="text-sm text-muted-foreground">Belum ada perubahan tercatat.</p>
        ) : (
          <ul className="divide-y divide-gray-100 rounded-lg border border-gray-200/70 bg-white text-sm">
            {(audit.data ?? []).map((row) => (
              <li key={row.id} className="px-4 py-2">
                <span className="text-gray-500">{new Date(row.created_at).toLocaleString("id-ID")}</span>
                {" · "}
                <span className="font-medium">{row.actor_name ?? "Sistem"}</span>
                {" · "}
                {SOURCE_LABEL[row.source]}
                <span className="block text-gray-700">{auditText(row)}</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
