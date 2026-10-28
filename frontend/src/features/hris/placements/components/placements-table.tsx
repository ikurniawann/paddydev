"use client";

import Link from "next/link";
import { Button } from "@/components/ui/button";
import { placementStatus, shortDateId } from "@/lib/hris/placements";
import { cn } from "@/lib/utils";
import type { Placement, PlacementSearchRow } from "../types";

/** Penempatan yang belum berakhir pada `today`. */
export function runningPlacements(row: PlacementSearchRow, today: string): Placement[] {
  return row.placements.filter((p) => placementStatus(p, today) !== "berakhir");
}

/** Toko utama yang berjalan; tanpa toko utama, penempatan berjalan pertama. */
export function mainPlacement(row: PlacementSearchRow, today: string): Placement | null {
  const running = runningPlacements(row, today);
  return running.find((p) => p.is_primary) ?? running[0] ?? null;
}

/** "4 Nov" dari "2026-11-04". */
function dayMonth(date: string): string {
  return shortDateId(date).split(" ").slice(0, 2).join(" ");
}

const TONE = {
  ok: "border-green-200 bg-green-50 text-green-700",
  warn: "border-amber-200 bg-amber-50 text-amber-800",
  muted: "border-gray-200 bg-gray-50 text-gray-500",
};

/** Status jadwal: Non-Shift → Belum ditempatkan → Shift s/d tanggal / Belum ada jadwal. */
export function scheduleStatus(row: PlacementSearchRow, today: string): { label: string; tone: keyof typeof TONE } {
  if (row.schedule_type === "non_shift") return { label: "Non-Shift", tone: "muted" };
  if (runningPlacements(row, today).length === 0) return { label: "Belum ditempatkan", tone: "warn" };
  if (!row.has_schedule) return { label: "Belum ada jadwal", tone: "warn" };
  return { label: row.schedule_until ? `Shift · s/d ${dayMonth(row.schedule_until)}` : "Ada jadwal", tone: "ok" };
}

interface PlacementsTableProps {
  rows: PlacementSearchRow[];
  today: string;
  canManage: boolean;
  selected: Set<string>;
  onToggle: (row: PlacementSearchRow) => void;
  onTogglePage: () => void;
  onManage: (row: PlacementSearchRow) => void;
}

export function PlacementsTable({ rows, today, canManage, selected, onToggle, onTogglePage, onManage }: PlacementsTableProps) {
  const pageSelected = rows.length > 0 && rows.every((r) => selected.has(r.employee_id));
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs text-gray-500">
            {canManage && (
              <th className="w-10 p-3">
                <input
                  type="checkbox"
                  aria-label="Pilih semua di halaman ini"
                  checked={pageSelected}
                  onChange={onTogglePage}
                />
              </th>
            )}
            <th className="p-3">Karyawan</th>
            <th className="p-3">Toko utama</th>
            <th className="p-3">Sejak</th>
            <th className="p-3">Jadwal</th>
            <th className="p-3 text-right">Aksi</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const main = mainPlacement(row, today);
            const others = new Set(runningPlacements(row, today).map((p) => p.branch_id)).size - 1;
            const schedule = scheduleStatus(row, today);
            const subtitle = [row.nip, row.position_title].filter(Boolean).join(" · ");
            const checked = selected.has(row.employee_id);
            return (
              <tr
                key={row.employee_id}
                className={cn("border-b border-gray-50 align-top last:border-0", checked && "bg-primary/5")}
              >
                {canManage && (
                  <td className="p-3">
                    <input
                      type="checkbox"
                      aria-label={`Pilih ${row.full_name}`}
                      checked={checked}
                      onChange={() => onToggle(row)}
                    />
                  </td>
                )}
                <td className="p-3">
                  <Link
                    href={`/dashboard/employees/${row.employee_id}?tab=placements`}
                    className="font-medium text-gray-900 hover:underline"
                  >
                    {row.full_name}
                  </Link>
                  <p className="text-xs text-gray-400" title={row.department_name ?? undefined}>
                    {subtitle || "—"}
                  </p>
                </td>
                <td className="p-3">
                  {main ? (
                    <>
                      <p className={cn("text-gray-900", !main.is_primary && "text-gray-600")}>{main.branch_name}</p>
                      {others > 0 && <p className="text-xs text-gray-400">+{others} toko lain</p>}
                    </>
                  ) : (
                    <span className="text-gray-400">—</span>
                  )}
                </td>
                <td className="whitespace-nowrap p-3 text-gray-600">{main ? shortDateId(main.start_date) : "—"}</td>
                <td className="p-3">
                  <span className={cn("whitespace-nowrap rounded-full border px-2 py-0.5 text-xs", TONE[schedule.tone])}>
                    {schedule.label}
                  </span>
                </td>
                <td className="p-3 text-right">
                  <Button size="sm" variant="outline" onClick={() => onManage(row)}>
                    {canManage ? "Kelola" : "Lihat"}
                  </Button>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
