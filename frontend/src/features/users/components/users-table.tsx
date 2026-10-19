"use client";

import Image from "next/image";
import {
  ArrowRightEndOnRectangleIcon,
  BuildingOfficeIcon,
  CalendarDaysIcon,
  EnvelopeIcon,
  EyeIcon,
  KeyIcon,
  MapPinIcon,
  PencilIcon,
  PhoneIcon,
  ShieldCheckIcon,
} from "@heroicons/react/24/outline";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { TableRow } from "@/components/ui/table";
import { formatDate } from "@/lib/format";
import { scheduleTypeLabel } from "@/lib/hris/schedule-type";
import type { UserEmployeeItem } from "@/lib/users/user-mapper";
import { ROLE_LABELS, STATUS_COLORS, STATUS_LABELS } from "../constants";

interface UsersTableProps {
  rows: UserEmployeeItem[];
  onView: (id: string) => void;
  onEdit: (id: string) => void;
  onResetPassword?: (row: UserEmployeeItem) => void;
  /** Shortcut buat akun login utk karyawan yang belum punya (Super Admin/Admin/HRD). */
  onCreateAccount?: (row: UserEmployeeItem) => void;
  /** Login As (impersonation) — hanya super_admin, target harus punya akun login. */
  onLoginAs?: (row: UserEmployeeItem) => void;
  loginAsEmployeeId?: string | null;
  showAppActions?: boolean;
  /** Seleksi baris untuk ubah massal tipe jadwal (direktori Karyawan). */
  selectedIds?: Set<string>;
  onSelectedChange?: (ids: Set<string>) => void;
}

export function UsersTable({
  rows,
  onView,
  onEdit,
  onResetPassword,
  onCreateAccount,
  onLoginAs,
  loginAsEmployeeId = null,
  showAppActions = false,
  selectedIds,
  onSelectedChange,
}: UsersTableProps) {
  const selectable = !!selectedIds && !!onSelectedChange;
  const allSelected = selectable && rows.length > 0 && rows.every((r) => selectedIds?.has(r.id));
  const toggle = (id: string) => {
    if (!selectedIds || !onSelectedChange) return;
    const next = new Set(selectedIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    onSelectedChange(next);
  };
  const toggleAll = () => {
    if (!selectedIds || !onSelectedChange) return;
    const next = new Set(selectedIds);
    rows.forEach((r) => (allSelected ? next.delete(r.id) : next.add(r.id)));
    onSelectedChange(next);
  };
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <TableRow className="border-b border-gray-200/70 bg-gray-50/80 text-xs uppercase tracking-wide text-gray-500 hover:bg-gray-50/80">
            {selectable ? (
              <th className="w-10 px-4 py-3">
                <input type="checkbox" aria-label="Pilih semua di halaman ini" checked={allSelected} onChange={toggleAll} />
              </th>
            ) : null}
            <th className="px-4 py-3 text-left font-semibold">Employee</th>
            <th className="px-4 py-3 text-left font-semibold">Employee ID</th>
            <th className="px-4 py-3 text-left font-semibold">Department</th>
            <th className="px-4 py-3 text-left font-semibold">Status</th>
            <th className="px-4 py-3 text-left font-semibold">App Access</th>
            {showAppActions ? <th className="px-4 py-3 text-left font-semibold">Outlet</th> : null}
            <th className="px-4 py-3 text-left font-semibold">Contact</th>
            <th className="px-4 py-3 text-right font-semibold">Actions</th>
          </TableRow>
        </thead>
        <tbody className="divide-y divide-gray-200/50">
          {rows.map((emp) => (
            <TableRow
              key={emp.id}
              className="cursor-pointer transition-colors hover:bg-gray-50/80"
              onClick={() => onView(emp.id)}
            >
              {selectable ? (
                <td className="px-4 py-3" onClick={(e) => e.stopPropagation()}>
                  <input
                    type="checkbox"
                    aria-label={`Pilih ${emp.fullName}`}
                    checked={selectedIds?.has(emp.id) ?? false}
                    onChange={() => toggle(emp.id)}
                  />
                </td>
              ) : null}
              <td className="px-4 py-3">
                <div className="flex items-center gap-3">
                  {emp.photoUrl ? (
                    <Image
                      src={emp.photoUrl}
                      alt={emp.fullName}
                      width={32}
                      height={32}
                      unoptimized
                      className="h-8 w-8 rounded-full object-cover"
                    />
                  ) : (
                    <div className="flex h-8 w-8 items-center justify-center rounded-full bg-gray-200 text-xs font-medium text-gray-600">
                      {emp.fullName.charAt(0).toUpperCase()}
                    </div>
                  )}
                  <div>
                    <p className="font-medium text-gray-900">{emp.fullName}</p>
                    <p className="text-xs text-gray-400">
                      {emp.isActive ? (
                        <span className="text-green-600">● Active</span>
                      ) : (
                        <span className="text-gray-400">● Inactive</span>
                      )}
                      {emp.jobTitle?.title ? ` · ${emp.jobTitle.title}` : ""}
                    </p>
                  </div>
                </div>
              </td>
              <td className="px-4 py-3">
                <span className="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs">
                  {emp.nip || "-"}
                </span>
              </td>
              <td className="px-4 py-3">
                <div className="flex items-center gap-1 text-gray-700">
                  <BuildingOfficeIcon className="h-3.5 w-3.5 shrink-0 text-gray-400" />
                  <span className="text-xs">{emp.department?.name || "-"}</span>
                </div>
              </td>
              <td className="px-4 py-3">
                <Badge
                  className={`border-0 font-normal ${
                    STATUS_COLORS[emp.employmentStatus] || "bg-gray-100 text-gray-600"
                  }`}
                >
                  {STATUS_LABELS[emp.employmentStatus] || emp.employmentStatus}
                </Badge>
                {!showAppActions ? (
                  <p className="mt-1 text-[11px] text-gray-500">{scheduleTypeLabel(emp.scheduleType)}</p>
                ) : null}
              </td>
              <td className="px-4 py-3">
                {emp.isAccessApp && emp.appAccount ? (
                  <div className="space-y-1">
                    <Badge className="border-0 bg-pink-50 font-normal text-pink-700">
                      <ShieldCheckIcon className="mr-1 inline h-3 w-3" />
                      {ROLE_LABELS[emp.appAccount.role]}
                    </Badge>
                    <p className="text-[11px] text-gray-400">
                      {emp.appAccount.status === "active" ? "Active account" : "Inactive account"}
                    </p>
                  </div>
                ) : (
                  <span className="text-xs text-gray-400">None</span>
                )}
              </td>
              {showAppActions ? (
                <td className="px-4 py-3">
                  {emp.isAccessApp && emp.appAccount?.warehouses?.length ? (
                    <div className="space-y-1">
                      {emp.appAccount.warehouses.map((warehouse) => (
                        <p
                          key={warehouse.id}
                          className="flex items-center gap-1 text-xs text-gray-600"
                        >
                          <MapPinIcon className="h-3 w-3 shrink-0 text-gray-400" />
                          <span>{warehouse.name}</span>
                        </p>
                      ))}
                    </div>
                  ) : emp.isAccessApp && emp.appAccount?.businessScope === "branch" ? (
                    <span className="text-xs text-amber-600">Not assigned</span>
                  ) : (
                    <span className="text-xs text-gray-400">-</span>
                  )}
                </td>
              ) : null}
              <td className="px-4 py-3">
                <div className="space-y-0.5 text-xs text-gray-500">
                  {emp.phone && (
                    <p className="flex items-center gap-1">
                      <PhoneIcon className="h-3 w-3" /> {emp.phone}
                    </p>
                  )}
                  {emp.email && (
                    <p className="flex items-center gap-1">
                      <EnvelopeIcon className="h-3 w-3" />
                      <span className="max-w-[140px] truncate">{emp.email}</span>
                    </p>
                  )}
                  <p className="flex items-center gap-1 text-gray-400">
                    <CalendarDaysIcon className="h-3 w-3" />
                    {formatDate(emp.joinDate)}
                  </p>
                </div>
              </td>
              <td className="px-4 py-3 text-right">
                <div
                  className="flex items-center justify-end gap-1"
                  onClick={(e) => e.stopPropagation()}
                >
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-8 w-8 p-0 text-gray-500 hover:bg-gray-100 hover:text-gray-800"
                    onClick={() => onView(emp.id)}
                    aria-label={`View ${emp.fullName}`}
                  >
                    <EyeIcon className="h-4 w-4" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-8 w-8 p-0 text-pink-600 hover:bg-pink-50 hover:text-pink-700"
                    onClick={() => onEdit(emp.id)}
                    aria-label={`Edit ${emp.fullName}`}
                  >
                    <PencilIcon className="h-4 w-4" />
                  </Button>
                  {onLoginAs &&
                  emp.isAccessApp &&
                  emp.userId &&
                  emp.appAccount?.role !== "super_admin" ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-8 w-8 p-0 text-blue-600 hover:bg-blue-50 hover:text-blue-700"
                      onClick={() => onLoginAs(emp)}
                      disabled={loginAsEmployeeId !== null}
                      aria-label={`Login as ${emp.fullName}`}
                      title={`Login as ${emp.fullName}`}
                    >
                      {loginAsEmployeeId === emp.id ? (
                        <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-500" />
                      ) : (
                        <ArrowRightEndOnRectangleIcon className="h-4 w-4" />
                      )}
                    </Button>
                  ) : null}
                  {!emp.userId && onCreateAccount ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-8 w-8 p-0 text-emerald-600 hover:bg-emerald-50 hover:text-emerald-700"
                      onClick={() => onCreateAccount(emp)}
                      aria-label={`Buat akun login ${emp.fullName}`}
                      title="Buat akun login"
                    >
                      <KeyIcon className="h-4 w-4" />
                    </Button>
                  ) : null}
                  {showAppActions && emp.isAccessApp && emp.userId && onResetPassword ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      className="h-8 w-8 p-0 text-gray-500 hover:bg-gray-100 hover:text-gray-800"
                      onClick={() => onResetPassword(emp)}
                      aria-label={`Reset password ${emp.fullName}`}
                      title="Reset password"
                    >
                      <KeyIcon className="h-4 w-4" />
                    </Button>
                  ) : null}
                </div>
              </td>
            </TableRow>
          ))}
        </tbody>
      </table>
    </div>
  );
}
