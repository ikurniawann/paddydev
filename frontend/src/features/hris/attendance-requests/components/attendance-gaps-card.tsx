"use client";

import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ATTENDANCE_GAP_LABEL, gapHint } from "@/lib/hris/attendance-requests";
import { shortDateId } from "@/lib/hris/placements";
import { useMyAttendanceGaps } from "../queries";
import type { AttendanceRequestPrefill } from "./attendance-request-dialog";

/**
 * "Absen terlewat" (MOM AB-06, K-9 digabung dengan AB-04): hari kerja di periode
 * absensi yang masih terbuka tanpa absen lengkap, masing-masing dengan tombol
 * "Ajukan koreksi". Tidak dirender bila tidak ada.
 */
export function AttendanceGapsCard({ onRequest }: { onRequest: (prefill: AttendanceRequestPrefill) => void }) {
  const days = useMyAttendanceGaps().data?.days ?? [];
  if (days.length === 0) return null;
  return (
    <Card className="border-amber-200 bg-amber-50/60">
      <CardContent className="min-w-0 space-y-2 p-4">
        <p className="flex items-center gap-1.5 text-sm font-semibold text-amber-800">
          <AlertTriangle className="h-4 w-4 shrink-0" />
          Absen terlewat ({days.length})
        </p>
        <p className="text-xs text-amber-700">
          Hari kerja di periode absensi yang masih terbuka tanpa absen lengkap. Ajukan koreksi dengan keterangan; HR yang
          menyetujui.
        </p>
        <ul className="divide-y divide-amber-100">
          {days.map((day) => (
            <li key={day.date} className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 py-2 text-sm">
              <span className="min-w-0 text-gray-800">
                <b>{shortDateId(day.date)}</b> · {ATTENDANCE_GAP_LABEL[day.kind]}
                {day.clock_in ? ` (masuk ${day.clock_in})` : ""}
              </span>
              <Button
                size="sm"
                variant="outline"
                className="shrink-0"
                onClick={() => onRequest({ date: day.date, hint: gapHint(day.kind, day.clock_in) })}
              >
                Ajukan koreksi
              </Button>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
