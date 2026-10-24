"use client";

import type { ReactNode } from "react";
import type { HolidayRow } from "@/lib/hris/holidays";
import { shiftClock } from "@/lib/hris/attendance-calendar";
import { formatTime } from "@/lib/format";
import { shortHoursDetail } from "@/lib/hris/short-hours";
import type { CalendarAttendance, CalendarScheduleRow } from "@/features/hris/attendance/types";

interface AttendanceDayCellProps {
  day: number;
  isToday: boolean;
  isSunday: boolean;
  attendance?: CalendarAttendance;
  scheduledShift: CalendarScheduleRow | null;
  isDayOff: boolean;
  holidays: HolidayRow[];
  /** Isi tambahan di bawah sel (mis. jadwal treatment terapis spa). */
  extra?: ReactNode;
  onClick: () => void;
}

/** Satu sel kalender absensi: libur, jadwal shift, dan realisasi absen. */
export function AttendanceDayCell({
  day,
  isToday,
  isSunday,
  attendance,
  scheduledShift,
  isDayOff,
  holidays,
  extra,
  onClick,
}: AttendanceDayCellProps) {
  const isLate = attendance?.is_late ?? false;
  const isPublicHoliday = holidays.length > 0;

  return (
    <div
      onClick={onClick}
      className={`group relative flex min-h-16 sm:min-h-24 cursor-pointer flex-col gap-1 p-1.5 sm:p-2 transition-colors ${
        isToday
          ? "bg-blue-50/70"
          : isPublicHoliday
            ? "bg-red-50/60"
            : isDayOff
              ? "bg-gray-50/80"
              : "bg-white hover:bg-slate-50"
      }`}
    >
      {/* nomor tanggal */}
      <div className="flex items-start justify-between">
        <span
          className={`inline-flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold ${
            isToday
              ? "bg-blue-600 text-white shadow-sm"
              : isPublicHoliday
                ? "text-red-600"
                : isSunday
                  ? "text-red-400"
                  : isDayOff
                    ? "text-gray-400"
                    : "text-gray-700"
          }`}
        >
          {day}
        </span>
        {/* dot ringkas utk layar kecil */}
        <span className="flex gap-1 sm:hidden">
          {isPublicHoliday && <span className="h-1.5 w-1.5 rounded-full bg-red-500" />}
          {attendance && (
            <span
              className={`h-1.5 w-1.5 rounded-full ${isLate ? "bg-amber-500" : "bg-emerald-500"}`}
            />
          )}
          {scheduledShift && <span className="h-1.5 w-1.5 rounded-full bg-indigo-400" />}
          {attendance?.is_short_hours && <span className="h-1.5 w-1.5 rounded-full bg-orange-500" />}
        </span>
      </div>

      {/* hari libur: nama liburnya, bukan sekadar warna merah */}
      {isPublicHoliday && (
        <div className="hidden sm:block rounded-md border-l-2 border-red-400 bg-red-50 px-1.5 py-0.5">
          {holidays.map((holiday) => (
            <p
              key={holiday.name}
              title={holiday.name}
              className="truncate text-[10px] font-semibold leading-tight text-red-700"
            >
              {holiday.name}
            </p>
          ))}
        </div>
      )}

      {/* jadwal shift */}
      {scheduledShift && (
        <div className="hidden sm:block rounded-md border-l-2 border-indigo-400 bg-indigo-50/80 px-1.5 py-0.5">
          <p className="truncate text-[10px] font-semibold leading-tight text-indigo-700">
            {scheduledShift.shift_name}
          </p>
          <p className="text-[10px] leading-tight text-indigo-400">
            {shiftClock(scheduledShift.start_time)}–{shiftClock(scheduledShift.end_time)}
            {scheduledShift.is_overnight ? " +1" : ""}
          </p>
        </div>
      )}
      {isDayOff && (
        <p className="hidden sm:block text-[10px] font-medium uppercase tracking-wider text-gray-300">
          Libur
        </p>
      )}

      {/* realisasi absensi */}
      {attendance && (
        <div
          className={`hidden sm:block rounded-md border-l-2 px-1.5 py-0.5 ${
            isLate ? "border-amber-400 bg-amber-50" : "border-emerald-400 bg-emerald-50"
          }`}
        >
          <p
            className={`truncate text-[10px] font-semibold leading-tight ${
              isLate ? "text-amber-700" : "text-emerald-700"
            }`}
          >
            {formatTime(attendance.clock_in, "–")}–{formatTime(attendance.clock_out, "–")}
          </p>
          <p className={`text-[10px] leading-tight ${isLate ? "text-amber-500" : "text-emerald-500"}`}>
            {isLate
              ? "Terlambat"
              : attendance.work_hours
                ? `${Number(attendance.work_hours).toFixed(1)} jam`
                : "Hadir"}
          </p>
          {attendance.is_short_hours && (
            <p
              className="truncate text-[10px] font-semibold leading-tight text-orange-600"
              title={shortHoursDetail(attendance.min_work_hours)}
            >
              Kurang jam
            </p>
          )}
        </div>
      )}
      {extra}
    </div>
  );
}
