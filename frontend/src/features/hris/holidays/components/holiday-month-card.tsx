import { PencilIcon, TrashIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { holidayDayLabel } from "@/lib/hris/holidays-view";
import { MONTH_NAMES_ID } from "@/lib/hris/month-label";
import type { HolidayRow } from "../types";
import { HolidayBadges } from "./holiday-badges";

interface HolidayMonthCardProps {
  /** 0-11 */
  month: number;
  holidays: HolidayRow[];
  onEdit: (holiday: HolidayRow) => void;
  onDelete: (holiday: HolidayRow) => void;
}

export function HolidayMonthCard({ month, holidays, onEdit, onDelete }: HolidayMonthCardProps) {
  return (
    <div className="rounded-xl border border-gray-200/70 bg-white p-4 shadow-sm">
      <p className="mb-3 text-xs font-semibold uppercase tracking-wider text-gray-400">
        {MONTH_NAMES_ID[month]}
      </p>
      <ul className="space-y-2.5">
        {holidays.map((holiday) => {
          const { day, weekday } = holidayDayLabel(holiday.holiday_date);
          return (
            <li key={holiday.id} className="group flex items-start gap-3">
              <div
                className={`flex w-11 shrink-0 flex-col items-center rounded-lg py-1 ${
                  holiday.status === "draft" ? "bg-gray-50 text-gray-400" : "bg-red-50 text-red-600"
                }`}
              >
                <span className="text-base font-bold leading-none">{day}</span>
                <span className="text-[10px] uppercase">{weekday}</span>
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium text-gray-900">{holiday.name}</p>
                <HolidayBadges
                  type={holiday.type}
                  deductsLeave={holiday.deducts_leave}
                  isDayOff={holiday.is_day_off}
                  status={holiday.status}
                />
                {holiday.note && (
                  <p className="mt-1 text-[11px] leading-snug text-gray-400">{holiday.note}</p>
                )}
              </div>
              <div className="flex shrink-0 gap-0.5 opacity-60 transition-opacity group-hover:opacity-100">
                <Button size="sm" variant="ghost" onClick={() => onEdit(holiday)}>
                  <PencilIcon className="h-3.5 w-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-red-600"
                  onClick={() => onDelete(holiday)}
                >
                  <TrashIcon className="h-3.5 w-3.5" />
                </Button>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
