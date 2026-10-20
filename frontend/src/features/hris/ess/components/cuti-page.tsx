"use client";

import { useState } from "react";
import { CalendarDaysIcon, ClockIcon, PaperAirplaneIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { formatDate } from "@/lib/format";
import { useEssMe, useMyLeaves } from "../queries";
import { leaveTypeLabel } from "@/lib/hris/leave-types";
import { CutiRequestDialog } from "./cuti-request-dialog";
import { EssLoading, EssNotLinked } from "./ess-states";

/**
 * ESS → Izin & Cuti (/dashboard/me/cuti): kuota tahunan, riwayat pengajuan,
 * dan form pengajuan mandiri (server memaksa employee_id = diri sendiri).
 */

const LEAVE_STATUS_BADGES: Record<string, { label: string; className: string }> = {
  pending: { label: "Menunggu", className: "bg-amber-100 text-amber-700" },
  approved: { label: "Disetujui", className: "bg-green-100 text-green-700" },
  rejected: { label: "Ditolak", className: "bg-red-100 text-red-700" },
  cancelled: { label: "Dibatalkan", className: "bg-gray-100 text-gray-600" },
};

export function EssCutiPage() {
  const { data: me, isLoading } = useEssMe();
  const leaves = useMyLeaves().data ?? [];
  const [leaveDialog, setLeaveDialog] = useState(false);

  if (isLoading) return <EssLoading />;
  if (!me?.employee) return <EssNotLinked feature="Pengajuan izin/cuti" />;

  const balance = me.leave_balance;

  return (
    <div className="space-y-6">
      <div className="border-b border-gray-200/70 pb-4">
        <h1 className="text-2xl font-bold text-gray-900">Izin & Cuti</h1>
        <p className="text-sm text-gray-500">Ajukan dan pantau izin/cuti Anda</p>
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-800">
            <CalendarDaysIcon className="h-4 w-4 text-pink-500" /> Kuota Cuti Tahunan{" "}
            {balance?.year ?? new Date().getFullYear()}
          </h3>
          <p className="mt-3 text-3xl font-bold text-gray-900">
            {balance ? Number(balance.annual_leave_remaining) : "12"}
            <span className="ml-1 text-base font-normal text-gray-500">hari tersisa</span>
          </p>
          <p className="mt-1 text-xs text-gray-500">
            Terpakai {balance ? Number(balance.annual_leave_used) : 0} dari{" "}
            {balance ? Number(balance.annual_leave_total) : 12} hari
          </p>
          <Button className="mt-4 w-full gap-2" onClick={() => setLeaveDialog(true)}>
            <PaperAirplaneIcon className="h-4 w-4" /> Ajukan Izin / Cuti
          </Button>
        </div>

        <div className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm lg:col-span-2">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-800">
            <ClockIcon className="h-4 w-4 text-pink-500" /> Riwayat Pengajuan
          </h3>
          {leaves.length === 0 ? (
            <p className="mt-4 py-6 text-center text-sm text-gray-400">Belum ada pengajuan.</p>
          ) : (
            <ul className="mt-3 divide-y divide-gray-100">
              {leaves.map((leave) => {
                const badge = LEAVE_STATUS_BADGES[leave.status] ?? LEAVE_STATUS_BADGES.pending;
                return (
                  <li key={leave.id} className="flex items-center justify-between gap-3 py-2.5">
                    <div className="min-w-0">
                      <p className="text-sm font-medium text-gray-900">
                        {leaveTypeLabel(leave.leave_type)} · {leave.total_days} hari
                      </p>
                      <p className="truncate text-xs text-gray-500">
                        {formatDate(leave.start_date, "—")} — {formatDate(leave.end_date, "—")}
                        {leave.status === "rejected" && leave.rejection_reason
                          ? ` · Alasan ditolak: ${leave.rejection_reason}`
                          : ""}
                      </p>
                    </div>
                    <span
                      className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}
                    >
                      {badge.label}
                    </span>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </div>

      <CutiRequestDialog open={leaveDialog} onOpenChange={setLeaveDialog} balance={balance} />
    </div>
  );
}
