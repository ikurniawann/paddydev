"use client";

import Image from "next/image";
import { AlertTriangle } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { formatTime } from "@/lib/format";
import { LEAVE_TYPE_LABELS } from "@/types/hris";
import { GeofenceBadge } from "./geofence-badge";
import { SelfiePhotoDialog } from "./selfie-photo-dialog";
import { ShortHoursBadge } from "./short-hours-badge";
import type { RosterEmployee, RosterStatus } from "../types";

export const STATUS_META: Record<RosterStatus, { label: string; className: string }> = {
  hadir: { label: "Hadir", className: "bg-green-100 text-green-700" },
  terlambat: { label: "Terlambat", className: "bg-yellow-100 text-yellow-700" },
  belum_absen: { label: "Belum Absen", className: "bg-gray-100 text-gray-600" },
  absen: { label: "Tidak Hadir", className: "bg-red-100 text-red-700" },
  cuti: { label: "Cuti/Izin", className: "bg-blue-100 text-blue-700" },
  libur_nasional: { label: "Libur Nasional", className: "bg-red-100 text-red-600" },
  libur: { label: "Libur", className: "bg-slate-100 text-slate-500" },
  tanpa_jadwal: { label: "Tanpa Jadwal", className: "bg-orange-100 text-orange-700" },
};

export function RosterRow({ emp, isToday }: { emp: RosterEmployee; isToday: boolean }) {
  const meta = STATUS_META[emp.status];
  const overdue = emp.is_overdue && isToday;
  return (
    <div
      className={`flex flex-wrap items-center gap-3 rounded-lg border p-3 ${
        overdue ? "border-red-300 bg-red-50" : "border-gray-100 bg-white"
      }`}
    >
      {emp.photo_url ? (
        <Image
          src={emp.photo_url}
          alt={emp.full_name}
          width={36}
          height={36}
          unoptimized
          className="w-9 h-9 rounded-full object-cover shrink-0"
        />
      ) : (
        <div className="w-9 h-9 rounded-full bg-gradient-to-br from-blue-500 to-indigo-600 flex items-center justify-center text-sm font-bold text-white shrink-0">
          {emp.full_name.charAt(0).toUpperCase()}
        </div>
      )}
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-gray-900">{emp.full_name}</p>
        <p className="truncate text-xs text-gray-500">
          {[emp.nip, emp.department_name, emp.job_title].filter(Boolean).join(" · ") || "—"}
        </p>
      </div>
      {emp.attendance ? (
        <div className="text-right text-xs text-gray-600">
          <p>
            Masuk <span className="font-medium">{formatTime(emp.attendance.clock_in, "—")}</span>
            {emp.attendance.is_late && (
              <span className="ml-1 text-yellow-700">(+{emp.attendance.late_minutes} mnt)</span>
            )}
          </p>
          <p>
            Pulang <span className="font-medium">{formatTime(emp.attendance.clock_out, "—")}</span>
          </p>
          <div className="mt-0.5 flex justify-end gap-2">
            <SelfiePhotoDialog
              path={emp.attendance.clock_in_photo_url}
              label="masuk"
              employeeName={emp.full_name}
              time={formatTime(emp.attendance.clock_in, "—")}
              showLabel
              emptyFallback={null}
            />
            <SelfiePhotoDialog
              path={emp.attendance.clock_out_photo_url}
              label="pulang"
              employeeName={emp.full_name}
              time={formatTime(emp.attendance.clock_out, "—")}
              showLabel
              emptyFallback={null}
            />
          </div>
          <div className="mt-1 flex flex-wrap justify-end gap-1">
            <GeofenceBadge status={emp.attendance.clock_in_geofence} distanceM={emp.attendance.clock_in_distance_m} prefix="Masuk" />
            <GeofenceBadge status={emp.attendance.clock_out_geofence} distanceM={emp.attendance.clock_out_distance_m} prefix="Pulang" />
            <ShortHoursBadge isShort={emp.attendance.is_short_hours} minHours={emp.attendance.min_work_hours} />
          </div>
          {emp.attendance.notes &&
            (emp.attendance.clock_in_geofence === "invalid" || emp.attendance.clock_out_geofence === "invalid") && (
              <p className="mt-0.5 max-w-xs whitespace-pre-line text-right text-[11px] text-gray-500">“{emp.attendance.notes}”</p>
            )}
        </div>
      ) : emp.leave ? (
        <p className="text-xs text-gray-500">
          {(emp.leave.leave_type &&
            LEAVE_TYPE_LABELS[emp.leave.leave_type as keyof typeof LEAVE_TYPE_LABELS]) ||
            "Cuti"}
        </p>
      ) : null}
      <div className="flex items-center gap-1.5 shrink-0">
        {overdue && (
          <Badge className="bg-red-100 text-red-700">
            <AlertTriangle className="w-3 h-3" /> Lewat toleransi
          </Badge>
        )}
        <Badge className={meta.className}>{meta.label}</Badge>
      </div>
    </div>
  );
}
