"use client";

import { useMemo, useState, type ComponentType, type ReactNode } from "react";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { ChevronLeft, ChevronRight, Calendar as CalendarIcon } from "lucide-react";
import { holidaysOn, isHolidayDayOff } from "@/lib/hris/holidays";
import { monthYearLabel } from "@/lib/hris/month-label";
import {
  dateKey,
  indexByWibDate,
  monthBounds,
  monthGrid,
  resolveDaySchedule,
} from "@/lib/hris/attendance-calendar";
import {
  useCalendarAttendances,
  useEmployeeSchedule,
  useHolidayIndex,
} from "@/features/hris/attendance/queries";
import { AttendanceDayCell } from "./AttendanceDayCell";
import { AttendanceDayDialog, type AttendanceDayInfo } from "./AttendanceDayDialog";

const WEEK_DAYS = ["Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"];

function thisMonth() {
  const now = new Date();
  return { year: now.getFullYear(), monthIndex: now.getMonth() };
}

/**
 * Kalender absensi bulanan: jadwal shift, hari libur, dan realisasi absen.
 * `employeeId="me"` untuk karyawan yang login (ESS). Setelah clock-in/out,
 * invalidasi `attendanceQueryKeys.calendarAll()` untuk memuat ulang.
 */
export function AttendanceCalendar({
  employeeId,
  DayExtra,
  legendExtra,
}: {
  employeeId?: string;
  /** Isi tambahan per tanggal (dirender di tiap sel; `start`/`end` = bulan tampil). */
  DayExtra?: ComponentType<{ date: string; start: string; end: string }>;
  legendExtra?: ReactNode;
}) {
  const [{ year, monthIndex }, setMonth] = useState(thisMonth);
  // Popup detail hari (terutama mobile: di layar kecil detail sel disembunyikan)
  const [selectedDate, setSelectedDate] = useState<string | null>(null);

  const { start, end } = monthBounds(year, monthIndex);
  const attendanceQuery = useCalendarAttendances(employeeId, start, end);
  // Hari libur (EPIC-036): gagal memuat tidak memblokir kalender.
  const holidayIndex = useHolidayIndex(start, end).data;
  // Pola jadwal shift: sekali per karyawan (pola mingguan, bukan per bulan).
  const scheduleData = useEmployeeSchedule(employeeId).data;
  const schedule = scheduleData?.rows ?? [];
  const scheduleType = scheduleData?.scheduleType ?? null;

  const attendances = useMemo(
    () => indexByWibDate(attendanceQuery.data ?? []),
    [attendanceQuery.data]
  );

  // Legenda merah hanya bila ada tanggal merah yang benar-benar libur bagi karyawan ini.
  const hasDayOffHoliday = holidayIndex
    ? [...holidayIndex.values()].some((rows) => rows.some((h) => isHolidayDayOff(h, scheduleType)))
    : false;

  const { daysInMonth, firstWeekday, totalCells } = monthGrid(year, monthIndex);
  const now = new Date();
  const todayKey = dateKey(now.getFullYear(), now.getMonth(), now.getDate());

  const shiftMonth = (delta: number) => {
    const d = new Date(year, monthIndex + delta, 1);
    setMonth({ year: d.getFullYear(), monthIndex: d.getMonth() });
  };

  const dayInfo = (date: string): AttendanceDayInfo => {
    const { scheduled, isDayOff } = resolveDaySchedule(schedule, date);
    return {
      date,
      attendance: attendances[date],
      scheduledShift: scheduled,
      isDayOff,
      holidays: holidayIndex ? holidaysOn(holidayIndex, date) : [],
      scheduleType,
    };
  };

  return (
    <Card className="overflow-hidden">
      <CardHeader className="pb-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <p className="flex items-center gap-1.5 text-xs font-medium uppercase tracking-wider text-gray-400">
              <CalendarIcon className="h-3.5 w-3.5" /> Kalender Absensi
            </p>
            <h2 className="mt-1 text-xl font-bold tracking-tight text-gray-900">{monthYearLabel(monthIndex + 1, year)}</h2>
          </div>
          <div className="flex items-center gap-1">
            <Button variant="ghost" size="icon" onClick={() => shiftMonth(-1)} aria-label="Bulan sebelumnya">
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setMonth(thisMonth())}
              className="h-8 rounded-full px-3 text-xs"
            >
              Hari Ini
            </Button>
            <Button variant="ghost" size="icon" onClick={() => shiftMonth(1)} aria-label="Bulan berikutnya">
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="overflow-hidden rounded-xl ring-1 ring-gray-200">
          {/* header hari */}
          <div className="grid grid-cols-7 gap-px bg-gray-100">
            {WEEK_DAYS.map((day, i) => (
              <div
                key={day}
                className={`bg-gray-50 py-2 text-center text-[10px] font-semibold uppercase tracking-wider ${
                  i === 0 ? "text-red-400" : "text-gray-400"
                }`}
              >
                {day}
              </div>
            ))}
          </div>

          {attendanceQuery.isLoading ? (
            /* skeleton grid saat memuat */
            <div className="grid grid-cols-7 gap-px bg-gray-100">
              {Array.from({ length: 35 }).map((_, i) => (
                <div key={i} className="min-h-16 sm:min-h-24 animate-pulse bg-white p-2">
                  <div className="h-5 w-5 rounded-full bg-gray-100" />
                </div>
              ))}
            </div>
          ) : (
            <div key={`${year}-${monthIndex}`} className="grid grid-cols-7 gap-px bg-gray-100">
              {Array.from({ length: totalCells }).map((_, i) => {
                const day = i - firstWeekday + 1;
                if (day < 1 || day > daysInMonth) {
                  return <div key={`pad-${i}`} className="min-h-16 sm:min-h-24 bg-gray-50/60" />;
                }
                const date = dateKey(year, monthIndex, day);
                const info = dayInfo(date);
                return (
                  <AttendanceDayCell
                    key={day}
                    day={day}
                    isToday={date === todayKey}
                    isSunday={i % 7 === 0}
                    attendance={info.attendance}
                    scheduledShift={info.scheduledShift}
                    isDayOff={info.isDayOff}
                    holidays={info.holidays}
                    scheduleType={scheduleType}
                    extra={DayExtra ? <DayExtra date={date} start={start} end={end} /> : undefined}
                    onClick={() => setSelectedDate(date)}
                  />
                );
              })}
            </div>
          )}
        </div>

        {/* legenda */}
        <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-[11px] text-gray-500">
          <span className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-emerald-500" /> Hadir
          </span>
          <span className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-amber-500" /> Terlambat
          </span>
          {hasDayOffHoliday && (
            <span className="flex items-center gap-1.5">
              <span className="h-2 w-2 rounded-full bg-red-500" /> Hari libur
            </span>
          )}
          {schedule.length > 0 && (
            <>
              <span className="flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full bg-indigo-400" /> Jadwal shift
              </span>
              <span className="flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full bg-gray-300" /> Libur
              </span>
            </>
          )}
          {legendExtra}
          <span className="ml-auto hidden text-gray-300 sm:inline">Waktu dalam WIB</span>
        </div>
        <p className="mt-2 text-[11px] text-gray-400 sm:hidden">
          Ketuk tanggal untuk melihat jadwal & jam absen.
        </p>
      </CardContent>

      <AttendanceDayDialog
        info={selectedDate ? dayInfo(selectedDate) : null}
        hasSchedule={schedule.length > 0}
        onClose={() => setSelectedDate(null)}
      />
    </Card>
  );
}
