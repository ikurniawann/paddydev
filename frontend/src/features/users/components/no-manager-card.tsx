"use client";

import Link from "next/link";
import { useState } from "react";
import { UserMinusIcon } from "@heroicons/react/24/outline";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { NO_MANAGER_REASON_LABELS } from "@/lib/hris/reporting-to";
import { useEmployeesWithoutManager } from "../queries";

/**
 * Kualitas data atasan langsung (EPIC-063 R9): karyawan aktif yang cuti dan
 * lemburnya hanya bisa diputus HR karena atasannya kosong, menunjuk dirinya
 * sendiri, atau nonaktif. Baca saja; perbaikan lewat edit karyawan.
 */
export function NoManagerCard() {
  const { data } = useEmployeesWithoutManager();
  const [open, setOpen] = useState(false);
  if (!data || data.length === 0) return null;

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex w-full items-center gap-3 rounded-xl border border-amber-200 bg-amber-50/70 p-4 text-left text-sm text-amber-900 shadow-sm hover:bg-amber-50"
      >
        <UserMinusIcon className="h-5 w-5 shrink-0 text-amber-600" />
        <span>
          <strong>{data.length} karyawan aktif tanpa atasan langsung yang valid</strong> — cuti & lembur mereka hanya
          bisa diputus HR · Lihat daftar →
        </span>
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Karyawan tanpa atasan langsung</DialogTitle>
          </DialogHeader>
          <ul className="max-h-[60vh] divide-y divide-gray-100 overflow-y-auto">
            {data.map((row) => (
              <li key={row.id} className="flex items-center justify-between gap-3 py-2 text-sm">
                <div className="min-w-0">
                  <Link href={`/dashboard/employees/${row.id}`} className="font-medium text-gray-900 hover:underline">
                    {row.full_name}
                  </Link>
                  <p className="truncate text-xs text-gray-500">
                    {[row.nip, row.job_title, row.department].filter(Boolean).join(" · ") || "—"}
                  </p>
                </div>
                <span className="shrink-0 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800">
                  {NO_MANAGER_REASON_LABELS[row.reason]}
                </span>
              </li>
            ))}
          </ul>
        </DialogContent>
      </Dialog>
    </>
  );
}
