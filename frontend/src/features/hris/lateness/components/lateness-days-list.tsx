import { shortDateId } from "@/lib/hris/placements";
import { formatLateMinutes } from "@/lib/hris/lateness-view";
import type { LateDay } from "../types";

const WEEKDAYS = ["Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"];

function weekday(date: string): string {
  const [y, m, d] = date.split("-").map(Number);
  return WEEKDAYS[new Date(Date.UTC(y, m - 1, d)).getUTCDay()] ?? "";
}

/** Daftar hari terlambat: tanggal + shift di kiri, jam masuk & menit terlambat di kanan (wrap di HP). */
export function LatenessDaysList({ days }: { days: LateDay[] }) {
  return (
    <ul className="divide-y divide-gray-100 rounded-xl border border-gray-100 bg-white">
      {days.map((day) => (
        <li key={day.date} className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 px-3 py-2.5 text-sm">
          <span className="min-w-0">
            <span className="font-semibold text-gray-900">
              {weekday(day.date)}, {shortDateId(day.date)}
            </span>
            {day.shift_name ? <span className="block text-xs text-gray-500">{day.shift_name}</span> : null}
          </span>
          <span className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-600">
            <span className="tabular-nums">
              Masuk <b className="text-gray-900">{day.clock_in ?? "-"}</b>
              {day.scheduled_start ? <> / jadwal {day.scheduled_start}</> : null}
            </span>
            <span className="rounded-full bg-red-50 px-2 py-0.5 font-semibold text-red-700 tabular-nums">
              +{formatLateMinutes(day.late_minutes)}
            </span>
          </span>
        </li>
      ))}
    </ul>
  );
}
