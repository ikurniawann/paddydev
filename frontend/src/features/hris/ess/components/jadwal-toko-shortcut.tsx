"use client";

import Link from "next/link";
import { CalendarDays, ChevronRight } from "lucide-react";
import { useScheduleAccess } from "@/features/hris/schedule-access/queries";

/**
 * Pintasan Jadwal Toko di Beranda ESS (EPIC-073 R4): di HP menu Area Karyawan
 * tersembunyi di "Lainnya", jadi pengatur jadwal toko dan HR Jadwal Kerja
 * mendapat kartu langsung. Tidak dirender untuk karyawan lain.
 */
export function JadwalTokoShortcut() {
  const data = useScheduleAccess().data;
  if (!data) return null;
  const hr = data.hr === true;
  const delegate = data.is_delegate && data.branches.length > 0;
  if (!hr && !delegate) return null;
  const sub = hr
    ? "Mode HR: atur jadwal kerja semua toko"
    : `Atur jadwal kerja${data.can_place ? " dan penempatan" : ""} karyawan di ${data.branches.length} toko Anda`;
  return (
    <Link
      href="/dashboard/me/jadwal-toko"
      className="flex items-center gap-3 rounded-xl border border-gray-200/70 bg-white p-4 shadow-sm transition hover:border-pink-200 hover:bg-pink-50/40"
    >
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-pink-50">
        <CalendarDays className="h-5 w-5 text-pink-600" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-semibold text-gray-900">Jadwal Toko</span>
        <span className="block break-words text-xs text-gray-500">{sub}</span>
      </span>
      <ChevronRight className="h-4 w-4 shrink-0 text-gray-400" />
    </Link>
  );
}
