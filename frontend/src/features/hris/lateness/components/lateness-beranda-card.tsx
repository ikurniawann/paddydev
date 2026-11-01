"use client";

import Link from "next/link";
import { AlarmClock, ChevronRight } from "lucide-react";
import { useMyLateness } from "../queries";
import { LatenessSummary } from "./lateness-summary";

/**
 * Kartu ringkas Beranda "Keterlambatan periode ini" (EPIC-077 R9), menaut ke
 * kartu lengkap di ESS → Absensi. Tidak dirender selama memuat atau gagal.
 */
export function LatenessBerandaCard() {
  const data = useMyLateness(0).data;
  if (!data) return null;
  return (
    <Link
      href="/dashboard/me/absensi#keterlambatan"
      className="block rounded-2xl border border-gray-200/70 bg-white p-4 shadow-sm transition hover:border-pink-200 hover:bg-pink-50/40 sm:p-5"
    >
      <span className="mb-3 flex items-center gap-2">
        <AlarmClock className="h-5 w-5 shrink-0 text-pink-600" />
        <span className="min-w-0 flex-1">
          <span className="block text-sm font-semibold text-gray-900">Keterlambatan periode ini</span>
          <span className="block text-xs text-gray-500">{data.period.label}</span>
        </span>
        <span className="flex shrink-0 items-center gap-0.5 text-xs font-medium text-pink-600">
          Detail <ChevronRight className="h-4 w-4" />
        </span>
      </span>
      <LatenessSummary stats={data} compact />
    </Link>
  );
}
