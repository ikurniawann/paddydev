"use client";

import { Users } from "lucide-react";
import { AttendanceMonitoringTab } from "@/features/hris/attendance/components/attendance-monitoring-tab";

/**
 * Area Karyawan → Absensi Tim (EPIC-074): atasan melihat presensi harian
 * seluruh timnya, langsung dan tidak langsung (rantai reporting_to). Hanya
 * baca; koreksi tetap lewat Pengajuan Absensi atau HR. Server
 * (/api/hris/me/team-attendance) yang membatasi isinya ke tim akun ini.
 */
export function EssAbsensiTimPage() {
  return (
    <div className="space-y-5">
      <div>
        <h1 className="flex items-center gap-2 text-2xl font-bold text-gray-900">
          <Users className="h-6 w-6 text-brand-text" />
          Absensi Tim
        </h1>
        <p className="mt-1 text-sm text-gray-500">
          Presensi anggota tim Anda, termasuk anggota di bawah koordinator. Koreksi absen tetap lewat
          Pengajuan Absensi atau HR.
        </p>
      </div>
      <AttendanceMonitoringTab scope="team" />
    </div>
  );
}
