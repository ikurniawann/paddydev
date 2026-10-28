import { hhmm } from "@/lib/hris/shifts-view";
import type { ShiftRow } from "./types";

export const DEFAULT_GROUP = "Bawaan";
const OTHER_GROUP = "Lainnya";

/** DEF dan NS dipakai jadwal bawaan & jadwal tetap Non-Shift: tidak bisa diubah kodenya, dihapus, atau dinonaktifkan. */
export const isProtectedShiftCode = (code: string) => code === "DEF" || code === "NS";

/** Awalan kode = huruf sebelum angka pertama ("PAGI8" → "PAGI"); DEF/NS masuk "Bawaan". */
export function shiftGroupName(code: string): string {
  if (isProtectedShiftCode(code)) return DEFAULT_GROUP;
  const prefix = code.match(/^[^0-9]*/)?.[0] ?? "";
  return prefix || OTHER_GROUP;
}

export interface ShiftGroup {
  name: string;
  shifts: ShiftRow[];
}

/** Bawaan paling atas, grup lain urut abjad; di dalam grup urut menurut angka pada kode. */
export function groupShifts(shifts: ShiftRow[]): ShiftGroup[] {
  const byName = new Map<string, ShiftRow[]>();
  for (const shift of shifts) {
    const name = shiftGroupName(shift.code);
    byName.set(name, [...(byName.get(name) ?? []), shift]);
  }
  const rank = (name: string) => (name === DEFAULT_GROUP ? 0 : 1);
  return [...byName.entries()]
    .map(([name, items]) => ({
      name,
      shifts: items.sort((a, b) => a.code.localeCompare(b.code, "id", { numeric: true })),
    }))
    .sort((a, b) => rank(a.name) - rank(b.name) || a.name.localeCompare(b.name, "id"));
}

/** Cocokkan kata kunci ke kode, nama, jam mulai/selesai, atau rentang jam. */
export function filterShifts(shifts: ShiftRow[], keyword: string): ShiftRow[] {
  const q = keyword.trim().toLowerCase();
  if (!q) return shifts;
  return shifts.filter((s) => {
    const start = hhmm(s.start_time);
    const end = hhmm(s.end_time);
    return [s.code, s.name, start, end, `${start}–${end}`, `${start}-${end}`].some((v) => v.toLowerCase().includes(q));
  });
}

const toMinutes = (time: string): number | null => {
  const m = /^(\d{1,2}):(\d{2})/.exec(time);
  return m ? Number(m[1]) * 60 + Number(m[2]) : null;
};

const fromMinutes = (total: number): string => {
  const day = ((total % 1440) + 1440) % 1440;
  return `${String(Math.floor(day / 60)).padStart(2, "0")}:${String(day % 60).padStart(2, "0")}`;
};

/** "Masuk 11:00 · terlambat setelah 11:10 · pulang 19:00"; null bila jam belum lengkap. */
export function shiftSummary(startTime: string, endTime: string, toleranceMinutes: string | number, overnight: boolean) {
  const start = toMinutes(startTime);
  const end = toMinutes(endTime);
  if (start === null || end === null) return null;
  const late = fromMinutes(start + (Number(toleranceMinutes) || 0));
  return `Masuk ${fromMinutes(start)} · terlambat setelah ${late} · pulang ${fromMinutes(end)}${
    overnight ? " (keesokan hari)" : ""
  }`;
}

/** Saran nama dari kode + jam, mis. "PAGI8 07:00-15:00"; kosong bila kode/jam belum ada. */
export function suggestShiftName(code: string, startTime: string, endTime: string): string {
  const c = code.trim().toUpperCase();
  if (!c || toMinutes(startTime) === null || toMinutes(endTime) === null) return "";
  return `${c} ${hhmm(startTime)}-${hhmm(endTime)}`;
}
