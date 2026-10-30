"use client";

import Link from "next/link";
import { ChevronRight, Users } from "lucide-react";
import { useTeamAttendance } from "@/features/hris/attendance/queries";
import { todayWib } from "@/lib/dates";

/**
 * Kartu Beranda "Absensi Tim" hari ini untuk atasan (EPIC-074 R7): di HP menu
 * Area Karyawan tersembunyi di "Lainnya". Tidak dirender tanpa anggota tim.
 */
export function TeamAttendanceCard() {
  const data = useTeamAttendance(todayWib()).data;
  if (!data || data.employees.length === 0) return null;
  const s = data.summary;
  return (
    <Link
      href="/dashboard/me/absensi-tim"
      className="flex items-center gap-3 rounded-xl border border-gray-200/70 bg-white p-4 shadow-sm transition hover:border-pink-200 hover:bg-pink-50/40"
    >
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-indigo-50">
        <Users className="h-5 w-5 text-indigo-600" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-semibold text-gray-900">Absensi Tim hari ini</span>
        <span className="block break-words text-xs text-gray-500">
          {`${(s.hadir ?? 0) + (s.terlambat ?? 0)} hadir · ${s.terlambat ?? 0} terlambat · ${s.belum_absen ?? 0} belum absen`}
        </span>
      </span>
      <ChevronRight className="h-4 w-4 shrink-0 text-gray-400" />
    </Link>
  );
}
