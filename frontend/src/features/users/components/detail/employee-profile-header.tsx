"use client";

import Image from "next/image";
import { useRouter } from "next/navigation";
import {
  CalendarDaysIcon,
  CheckCircleIcon,
  EnvelopeIcon,
  KeyIcon,
  PencilIcon,
  PhoneIcon,
  XCircleIcon,
} from "@heroicons/react/24/outline";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { formatDate } from "@/lib/format";
import { calculateTenure } from "@/lib/hris/employee-profile-summary";
import { scheduleTypeLabel } from "@/lib/hris/schedule-type";
import type { Employee } from "@/types/hris";
import { EMPLOYEES_ROUTES, STATUS_COLORS, STATUS_LABELS } from "../../constants";

interface EmployeeProfileHeaderProps {
  employee: Employee;
  canManageUsers: boolean;
  onCreateAccount: () => void;
  onResetPassword: () => void;
  /** HR: buka dialog ubah tipe jadwal (Shift / Non-Shift). */
  onChangeScheduleType?: () => void;
  /** HR: buka dialog Resign (hanya karyawan aktif). */
  onResign?: () => void;
}

export function EmployeeProfileHeader({
  employee,
  canManageUsers,
  onCreateAccount,
  onResetPassword,
  onChangeScheduleType,
  onResign,
}: EmployeeProfileHeaderProps) {
  const router = useRouter();

  return (
    <Card>
      <CardContent className="p-6">
        <div className="flex flex-col sm:flex-row items-start sm:items-center gap-5">
          {employee.photo_url ? (
            <Image
              src={employee.photo_url}
              alt={employee.full_name}
              width={80}
              height={80}
              unoptimized
              className="w-20 h-20 rounded-full object-cover border-2 border-gray-200"
            />
          ) : (
            <div className="w-20 h-20 rounded-full bg-gradient-to-br from-blue-500 to-indigo-600 flex items-center justify-center text-2xl font-bold text-white shrink-0">
              {employee.full_name.charAt(0).toUpperCase()}
            </div>
          )}
          <div className="flex-1 min-w-0">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-bold text-gray-900">{employee.full_name}</h1>
              <Badge
                className={STATUS_COLORS[employee.employment_status] || "bg-gray-100 text-gray-600"}
              >
                {STATUS_LABELS[employee.employment_status] || employee.employment_status}
              </Badge>
              {employee.is_active ? (
                <span className="flex items-center gap-1 text-xs text-green-600">
                  <CheckCircleIcon className="w-3.5 h-3.5" /> Active
                </span>
              ) : (
                <span className="flex items-center gap-1 text-xs text-red-500">
                  <XCircleIcon className="w-3.5 h-3.5" /> Inactive
                </span>
              )}
              <Badge variant={employee.schedule_type === "non_shift" ? "info" : "secondary"}>
                {scheduleTypeLabel(employee.schedule_type ?? "shift")}
              </Badge>
            </div>
            <p className="text-sm text-gray-500 mt-1">
              {employee.job_title?.title || "—"} · {employee.department?.name || "—"}
              {employee.section ? ` · ${employee.section.name}` : ""}
            </p>
            <div className="flex flex-wrap gap-4 mt-2 text-xs text-gray-500">
              <span className="font-mono bg-gray-100 px-1.5 py-0.5 rounded">{employee.nip}</span>
              {employee.email && (
                <span className="flex items-center gap-1">
                  <EnvelopeIcon className="w-3.5 h-3.5" /> {employee.email}
                </span>
              )}
              {employee.phone && (
                <span className="flex items-center gap-1">
                  <PhoneIcon className="w-3.5 h-3.5" /> {employee.phone}
                </span>
              )}
              <span className="flex items-center gap-1">
                <CalendarDaysIcon className="w-3.5 h-3.5" />
                Joined {formatDate(employee.join_date)} · {calculateTenure(employee.join_date)}
              </span>
            </div>
          </div>
          <div className="flex gap-2 shrink-0">
            {onResign && employee.is_active ? (
              <Button variant="outline" size="sm" onClick={onResign} className="border-danger/30 text-danger hover:bg-danger-soft">
                Resign
              </Button>
            ) : null}
            {!employee.is_active ? (
              <Badge variant="destructive">
                {employee.employment_status === "terminated" ? "Diberhentikan" : "Resign"}
                {employee.end_date ? ` · ${formatDate(employee.end_date)}` : ""}
              </Badge>
            ) : null}
            {onChangeScheduleType && employee.is_active ? (
              <Button variant="outline" size="sm" onClick={onChangeScheduleType}>
                Ubah tipe jadwal
              </Button>
            ) : null}
            {canManageUsers && !employee.user_id ? (
              <Button
                variant="outline"
                size="sm"
                onClick={onCreateAccount}
                className="gap-1 text-emerald-700 border-emerald-200 hover:bg-emerald-50"
              >
                <KeyIcon className="w-4 h-4" /> Buat Akun Login
              </Button>
            ) : null}
            {canManageUsers && employee.user_id ? (
              <Button variant="outline" size="sm" onClick={onResetPassword} className="gap-1">
                <KeyIcon className="w-4 h-4" /> Reset Password
              </Button>
            ) : null}
            <Button
              variant="outline"
              size="sm"
              onClick={() => router.push(`/dashboard/hris/onboarding/${employee.id}`)}
              className="gap-1"
            >
              Onboarding
            </Button>
            <Button
              size="sm"
              onClick={() => router.push(EMPLOYEES_ROUTES.edit(employee.id))}
              className="gap-1"
            >
              <PencilIcon className="w-4 h-4" /> Edit
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
