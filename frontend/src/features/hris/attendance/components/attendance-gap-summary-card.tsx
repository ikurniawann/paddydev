"use client";

import { useState } from "react";
import { AlertTriangle, CheckCircle2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { todayWib } from "@/lib/dates";
import { addDays } from "@/lib/hris/attendance-period";
import { shortDateId } from "@/lib/hris/placements";
import { useAttendanceGapSummary } from "../queries";

/**
 * "Absen terlewat kemarin" (EPIC-067): ringkasan untuk HR dari
 * /api/hris/attendance/gap-summary — aturan yang sama dengan pemindaian ESS
 * (hari libur, cuti, dinas, dan pengajuan yang menunggu tidak dihitung).
 * Karyawan tanpa jadwal ditampilkan terpisah dan tidak dihitung belum absen.
 */
export function AttendanceGapSummaryCard() {
  const yesterday = addDays(todayWib(), -1);
  const [date, setDate] = useState(yesterday);
  const [open, setOpen] = useState(false);
  const query = useAttendanceGapSummary(date);
  const data = query.data;

  const title = date === yesterday ? "Absen terlewat kemarin" : `Absen terlewat ${shortDateId(date)}`;
  const total = data ? data.missing + data.no_clock_out : 0;

  return (
    <Card className="border-amber-200 bg-amber-50/60">
      <CardContent className="min-w-0 space-y-2 p-4">
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
          <p className="flex min-w-0 items-center gap-1.5 text-sm font-semibold text-amber-800">
            <AlertTriangle className="h-4 w-4 shrink-0" />
            {title}
          </p>
          <div className="flex items-center gap-2">
            <Label htmlFor="gap-summary-date" className="sr-only">
              Tanggal absen terlewat
            </Label>
            <Input
              id="gap-summary-date"
              type="date"
              className="h-8 w-auto bg-white"
              value={date}
              max={todayWib()}
              onChange={(e) => {
                if (e.target.value) {
                  setDate(e.target.value);
                  setOpen(false);
                }
              }}
            />
          </div>
        </div>

        {query.isLoading ? (
          <p className="text-sm text-amber-700">Memuat ringkasan…</p>
        ) : query.isError || !data ? (
          <p className="text-sm text-red-600">{query.error?.message || "Gagal memuat ringkasan absen terlewat"}</p>
        ) : (
          <>
            {total === 0 ? (
              <p className="flex items-center gap-1.5 text-sm text-gray-700">
                <CheckCircle2 className="h-4 w-4 shrink-0 text-green-600" />
                Tidak ada absen terlewat
              </p>
            ) : (
              <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm text-gray-800">
                <span className="font-semibold">{data.missing} belum absen</span>
                <span className="font-semibold">{data.no_clock_out} belum absen pulang</span>
                <Button size="sm" variant="outline" onClick={() => setOpen((v) => !v)}>
                  {open ? "Sembunyikan daftar" : "Lihat daftar"}
                </Button>
              </div>
            )}
            {data.unscheduled > 0 && (
              <p className="text-xs text-amber-700">{data.unscheduled} karyawan tanpa jadwal (tidak dihitung)</p>
            )}
            {data.period_closed && <p className="text-xs text-amber-700">Periode sudah ditutup</p>}
            {open && total > 0 && (
              <ul className="divide-y divide-amber-100">
                {data.employees.map((emp) => (
                  <li
                    key={`${emp.employee_id}-${emp.kind}`}
                    className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 py-2 text-sm"
                  >
                    <span className="min-w-0 text-gray-800">
                      <b>{emp.name}</b>
                      {emp.nip ? ` · ${emp.nip}` : ""}
                    </span>
                    <span className="shrink-0 text-gray-600">
                      {emp.kind === "no_clock_out"
                        ? `Belum absen pulang${emp.clock_in ? ` (masuk ${emp.clock_in})` : ""}`
                        : "Belum absen"}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
