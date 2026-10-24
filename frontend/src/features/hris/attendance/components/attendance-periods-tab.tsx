"use client";

import { useState } from "react";
import { Lock, LockOpen } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { formatDateTime } from "@/lib/format";
import { shortDateId } from "@/lib/hris/attendance-period";
import { AttendancePeriodSettingsCard } from "./attendance-period-settings-card";
import { ClosePeriodDialog, ReopenPeriodDialog } from "./attendance-period-dialogs";
import { useAttendancePeriods } from "../queries";
import type { AttendancePeriod, AttendancePeriodEvent } from "../types";

const RUN_STATUS: Record<string, { label: string; className: string }> = {
  draft: { label: "Draft", className: "bg-gray-100 text-gray-700" },
  processing: { label: "Diproses", className: "bg-yellow-100 text-yellow-700" },
  completed: { label: "Disetujui", className: "bg-blue-100 text-blue-700" },
  paid: { label: "Dibayar", className: "bg-green-100 text-green-700" },
};

const EVENT_LABEL: Record<AttendancePeriodEvent["action"], string> = {
  close: "Closing manual",
  auto_close: "Closing otomatis",
  reopen: "Dibuka ulang",
  closing_date: "Tanggal closing diubah",
};

function StatusBadge({ period }: { period: AttendancePeriod }) {
  if (period.status === "closed") {
    return (
      <Badge className="gap-1 bg-slate-100 text-slate-700">
        <Lock className="h-3 w-3" />
        Closed{period.close_mode === "auto" ? " (otomatis)" : ""}
      </Badge>
    );
  }
  return (
    <Badge className="gap-1 bg-emerald-100 text-emerald-700">
      <LockOpen className="h-3 w-3" />
      {period.is_current ? "Berjalan" : "Terbuka"}
    </Badge>
  );
}

/**
 * Tab "Periode Absensi": pengaturan cut-off + daftar periode setahun dengan
 * status closing, siapa/kapan, payroll terkait, dan riwayat buka ulang.
 */
export function AttendancePeriodsTab() {
  const [year, setYear] = useState<number | undefined>(undefined);
  const query = useAttendancePeriods(year);
  const data = query.data;
  const shownYear = year ?? data?.year ?? new Date().getFullYear();
  const [closing, setClosing] = useState<AttendancePeriod | null>(null);
  const [reopening, setReopening] = useState<AttendancePeriod | null>(null);
  const years = Array.from({ length: 4 }, (_, i) => (data?.year ?? new Date().getFullYear()) - i);

  return (
    <div className="space-y-4">
      <AttendancePeriodSettingsCard />

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3">
          <CardTitle className="text-base">Periode Absensi {shownYear}</CardTitle>
          <Select value={String(shownYear)} onValueChange={(v) => setYear(Number(v))}>
            <SelectTrigger className="w-28">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {years.map((y) => (
                <SelectItem key={y} value={String(y)}>
                  {y}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CardHeader>
        <CardContent className="p-0">
          {query.isLoading ? (
            <div className="flex justify-center py-10">
              <div className="h-6 w-6 animate-spin rounded-full border-2 border-gray-300 border-t-pink-500" />
            </div>
          ) : query.error ? (
            <p className="py-10 text-center text-sm text-red-500">{query.error.message}</p>
          ) : (data?.periods ?? []).length === 0 ? (
            <p className="py-10 text-center text-sm text-gray-400">Belum ada periode pada tahun ini.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-gray-600">
                    <th className="p-3">Periode</th>
                    <th className="p-3">Status</th>
                    <th className="p-3">Closing</th>
                    <th className="p-3">Absensi</th>
                    <th className="p-3">Payroll</th>
                    <th className="p-3">Riwayat</th>
                    <th className="p-3 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {(data?.periods ?? []).map((p) => (
                    <tr key={p.id} className="border-b border-gray-50 align-top">
                      <td className="p-3">
                        <p className="font-medium text-gray-900">{p.name}</p>
                        <p className="text-xs text-gray-500">{p.label}</p>
                      </td>
                      <td className="p-3">
                        <StatusBadge period={p} />
                      </td>
                      <td className="whitespace-nowrap p-3 text-xs text-gray-600">
                        {p.status === "closed" ? (
                          <>
                            <p>{formatDateTime(p.closed_at)}</p>
                            <p className="text-gray-400">
                              oleh {p.close_mode === "auto" ? "Sistem" : p.closed_by_name ?? "-"}
                            </p>
                          </>
                        ) : (
                          <>
                            <p>Otomatis {shortDateId(p.closing_date)}</p>
                            <p className="text-gray-400">
                              {p.days_until_closing <= 0 ? "hari ini terakhir" : `${p.days_until_closing} hari lagi`}
                            </p>
                          </>
                        )}
                      </td>
                      <td className="whitespace-nowrap p-3 text-xs text-gray-600">
                        <p>{p.stats?.rows ?? 0} baris · {p.stats?.employees ?? 0} karyawan</p>
                        {(p.stats?.without_clock_out ?? 0) > 0 && (
                          <p className="text-amber-700">{p.stats?.without_clock_out} tanpa jam pulang</p>
                        )}
                        {(p.stats?.pending_requests ?? 0) > 0 && (
                          <p className="text-amber-700">{p.stats?.pending_requests} pengajuan absensi menunggu</p>
                        )}
                        {(p.stats?.adjusted ?? 0) > 0 && (
                          <p className="text-violet-700">{p.stats?.adjusted} disesuaikan manual</p>
                        )}
                      </td>
                      <td className="p-3">
                        {(p.payroll_runs ?? []).length === 0 ? (
                          <span className="text-xs text-gray-400">—</span>
                        ) : (
                          (p.payroll_runs ?? []).map((r) => (
                            <Badge key={r.id} className={RUN_STATUS[r.status]?.className ?? "bg-gray-100 text-gray-700"}>
                              {RUN_STATUS[r.status]?.label ?? r.status}
                            </Badge>
                          ))
                        )}
                      </td>
                      <td className="max-w-xs p-3 text-xs text-gray-600">
                        {(p.events ?? []).length === 0 ? (
                          <span className="text-gray-400">—</span>
                        ) : (
                          <ul className="space-y-1">
                            {(p.events ?? []).slice(0, 4).map((e) => (
                              <li key={e.id}>
                                <span className="font-medium text-gray-700">{EVENT_LABEL[e.action]}</span> ·{" "}
                                {e.actor_name ?? "-"} · {formatDateTime(e.created_at)}
                                {e.reason && <span className="block text-gray-500">“{e.reason}”</span>}
                              </li>
                            ))}
                          </ul>
                        )}
                      </td>
                      <td className="whitespace-nowrap p-3 text-right">
                        {p.status === "open" && data?.permissions.can_manage && p.has_ended && (
                          <Button size="sm" variant="outline" onClick={() => setClosing(p)}>
                            Closing
                          </Button>
                        )}
                        {p.status === "closed" && data?.permissions.can_reopen && (
                          <Button size="sm" variant="ghost" onClick={() => setReopening(p)}>
                            Buka ulang
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {closing && (
        <ClosePeriodDialog period={closing} open onOpenChange={(open) => !open && setClosing(null)} />
      )}
      {reopening && (
        <ReopenPeriodDialog
          period={reopening}
          open
          onOpenChange={(open) => !open && setReopening(null)}
          offsetDays={data?.settings.closing_offset_days}
        />
      )}
    </div>
  );
}
