"use client";

import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import type { HolidayRow } from "@/lib/hris/holidays";
import { shiftClock } from "@/lib/hris/attendance-calendar";
import { formatDateLong, formatTime } from "@/lib/format";
import { SHORT_HOURS_LABEL, shortHoursDetail } from "@/lib/hris/short-hours";
import type { CalendarAttendance, CalendarScheduleRow } from "@/features/hris/attendance/types";

export interface AttendanceDayInfo {
  date: string;
  attendance?: CalendarAttendance;
  scheduledShift: CalendarScheduleRow | null;
  isDayOff: boolean;
  holidays: HolidayRow[];
}

interface AttendanceDayDialogProps {
  info: AttendanceDayInfo | null;
  hasSchedule: boolean;
  onClose: () => void;
}

/** Popup detail hari, dipakai terutama di mobile (detail sel disembunyikan). */
export function AttendanceDayDialog({ info, hasSchedule, onClose }: AttendanceDayDialogProps) {
  const attendance = info?.attendance;

  return (
    <Dialog open={info !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-xs">
        <DialogHeader>
          <DialogTitle className="text-base">{info ? formatDateLong(info.date) : ""}</DialogTitle>
        </DialogHeader>
        {info && (
          <div className="space-y-3">
            {info.holidays.length > 0 && (
              <div className="rounded-lg border border-red-200 bg-red-50/70 p-3">
                <p className="text-[11px] font-semibold uppercase tracking-wide text-red-400">
                  Hari Libur
                </p>
                {info.holidays.map((holiday) => (
                  <p key={holiday.name} className="mt-1 text-sm font-semibold text-red-700">
                    {holiday.name}
                    {holiday.deducts_leave && (
                      <span className="ml-1 text-xs font-normal text-red-400">
                        (memotong jatah cuti)
                      </span>
                    )}
                  </p>
                ))}
              </div>
            )}

            <div className="rounded-lg border border-indigo-100 bg-indigo-50/60 p-3">
              <p className="text-[11px] font-semibold uppercase tracking-wide text-indigo-400">
                Jadwal Shift
              </p>
              {info.scheduledShift ? (
                <p className="mt-1 text-sm font-semibold text-indigo-800">
                  {info.scheduledShift.shift_name} · {shiftClock(info.scheduledShift.start_time)}–
                  {shiftClock(info.scheduledShift.end_time)}
                  {info.scheduledShift.is_overnight ? " (+1 hari)" : ""}
                </p>
              ) : info.isDayOff ? (
                <p className="mt-1 text-sm font-medium text-gray-500">Libur</p>
              ) : (
                <p className="mt-1 text-sm text-gray-400">
                  {hasSchedule ? "Tanpa jadwal" : "Jadwal belum diatur"}
                </p>
              )}
            </div>

            <div
              className={`rounded-lg border p-3 ${
                attendance
                  ? attendance.is_late
                    ? "border-amber-200 bg-amber-50/60"
                    : "border-emerald-200 bg-emerald-50/60"
                  : "border-gray-100 bg-gray-50/60"
              }`}
            >
              <p className="text-[11px] font-semibold uppercase tracking-wide text-gray-400">
                Absensi
              </p>
              {attendance ? (
                <div className="mt-1 space-y-0.5 text-sm">
                  <p className="font-semibold text-gray-800">
                    {formatTime(attendance.clock_in, "–")} – {formatTime(attendance.clock_out, "–")}
                  </p>
                  <p className={attendance.is_late ? "text-amber-600" : "text-emerald-600"}>
                    {attendance.is_late ? "Terlambat" : "Tepat waktu"}
                    {attendance.work_hours
                      ? ` · ${Number(attendance.work_hours).toFixed(1)} jam kerja`
                      : ""}
                  </p>
                  {attendance.is_short_hours && (
                    <p className="text-orange-600">
                      {SHORT_HOURS_LABEL} · {shortHoursDetail(attendance.min_work_hours)}
                    </p>
                  )}
                </div>
              ) : (
                <p className="mt-1 text-sm text-gray-400">Belum ada catatan absen</p>
              )}
            </div>
            <p className="text-center text-[10px] text-gray-300">Waktu dalam WIB</p>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
