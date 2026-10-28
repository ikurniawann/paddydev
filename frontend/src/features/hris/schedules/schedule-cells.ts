import { addDays, dayDiff, weekdayIndex } from "./schedule-range";
import type { CellEdit, ScheduleGrid } from "./types";

/**
 * Logika murni grid Jadwal Kerja yang bisa diedit (EPIC-070): pola mingguan,
 * salin minggu lalu, isi kotak terpilih. Kotak "-" (tidak ditempatkan di
 * cabang pada tanggal itu) tidak pernah dikirim.
 */

export const cellKey = (employeeId: string, date: string) => `${employeeId}|${date}`;

export function parseCellKey(key: string): { employee_id: string; date: string } {
  const at = key.lastIndexOf("|");
  return { employee_id: key.slice(0, at), date: key.slice(at + 1) };
}

/** Batas grid lama: satu permintaan 7–45 hari. */
const GRID_MIN = 7;
const GRID_MAX = 45;

/** Potongan rentang (masing-masing 7–45 hari) untuk memuat grid sepanjang from..to. */
export function coverChunks(from: string, to: string): { from: string; to: string }[] {
  const out: { from: string; to: string }[] = [];
  let start = from;
  while (dayDiff(start, to) >= 0) {
    const end = dayDiff(start, to) + 1 > GRID_MAX ? addDays(start, GRID_MAX - 1) : to;
    out.push({ from: start, to: dayDiff(start, end) + 1 < GRID_MIN ? addDays(start, GRID_MIN - 1) : end });
    start = addDays(end, 1);
  }
  return out;
}

type GridRows = Pick<ScheduleGrid, "employees">;

/** Gabungkan kotak karyawan dari beberapa potongan grid. */
export function mergeGrids(grids: GridRows[]): GridRows {
  const byId = new Map<string, ScheduleGrid["employees"][number]>();
  for (const g of grids) {
    for (const e of g.employees) {
      const seen = byId.get(e.id);
      if (seen) seen.cells = [...seen.cells, ...e.cells];
      else byId.set(e.id, { ...e, cells: [...e.cells] });
    }
  }
  return { employees: [...byId.values()] };
}

function codeLookup(grid: GridRows): Map<string, string> {
  const map = new Map<string, string>();
  for (const e of grid.employees) for (const c of e.cells) map.set(cellKey(e.id, c.date), c.code);
  return map;
}

/** Edit terakhir untuk kunci yang sama menang; urutan kemunculan pertama dipertahankan. */
export function dedupeCells(cells: CellEdit[]): CellEdit[] {
  const map = new Map<string, CellEdit>();
  for (const c of cells) map.set(cellKey(c.employee_id, c.date), c);
  return [...map.values()];
}

/**
 * Pola mingguan (indeks 0 = Senin … 6 = Minggu) untuk from..to. Hanya kotak
 * yang ada di grid dan ditempatkan (bukan "-") yang dikirim.
 */
export function expandPattern(employeeIds: string[], pattern: string[], from: string, to: string, grid: GridRows): CellEdit[] {
  const codes = codeLookup(grid);
  const out: CellEdit[] = [];
  for (const id of employeeIds) {
    for (let date = from; dayDiff(date, to) >= 0; date = addDays(date, 1)) {
      const current = codes.get(cellKey(id, date));
      if (current === undefined || current === "-") continue;
      out.push({ employee_id: id, date, code: pattern[weekdayIndex(date)] ?? "" });
    }
  }
  return dedupeCells(out);
}

/**
 * Salin jadwal 7 hari sebelumnya ke minggu pertama grid sekarang, hari ke
 * hari yang sama. Dilewati: karyawan yang tidak ada di keduanya, kotak "-",
 * jadwal minggu lalu yang kosong, kotak yang sudah sama, dan tanggal lewat.
 */
export function copyWeek(prevGrid: GridRows, curGrid: GridRows, today = ""): CellEdit[] {
  const prevByEmployee = new Map(prevGrid.employees.map((e) => [e.id, e.cells]));
  const out: CellEdit[] = [];
  for (const e of curGrid.employees) {
    const prev = prevByEmployee.get(e.id);
    if (!prev) continue;
    const byWeekday = new Map(prev.map((c) => [weekdayIndex(c.date), c.code]));
    const week = e.cells.length > 0 ? addDays(e.cells[0].date, 6) : "";
    for (const c of e.cells) {
      if (dayDiff(c.date, week) < 0 || (today && c.date < today) || c.code === "-") continue;
      const code = byWeekday.get(weekdayIndex(c.date));
      if (!code || code === "-" || code === c.code) continue;
      out.push({ employee_id: e.id, date: c.date, code });
    }
  }
  return out;
}

/** Isi setiap kotak terpilih (kunci cellKey) dengan kode yang sama; "" = kosongkan. */
export function fillCells(selected: Iterable<string>, code: string): CellEdit[] {
  return dedupeCells([...selected].map((key) => ({ ...parseCellKey(key), code })));
}

/** Kunci kotak dalam satu baris di antara dua tanggal (inklusif, urutan bebas). */
export function rowRange(employeeId: string, dates: string[], a: string, b: string): string[] {
  const [lo, hi] = a <= b ? [a, b] : [b, a];
  return dates.filter((d) => d >= lo && d <= hi).map((d) => cellKey(employeeId, d));
}

const PALETTE = [
  "bg-sky-100 text-sky-800",
  "bg-emerald-100 text-emerald-800",
  "bg-violet-100 text-violet-800",
  "bg-rose-100 text-rose-800",
  "bg-teal-100 text-teal-800",
  "bg-indigo-100 text-indigo-800",
  "bg-orange-100 text-orange-800",
  "bg-lime-100 text-lime-800",
  "bg-fuchsia-100 text-fuchsia-800",
  "bg-cyan-100 text-cyan-800",
];

/** Warna kotak per kode shift (hash → palet kecil); L abu-abu, kosong kuning. */
export function shiftTone(code: string): string {
  if (code === "L") return "bg-gray-100 text-gray-500";
  if (code === "") return "bg-amber-50 text-amber-700";
  let h = 0;
  for (const ch of code) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return PALETTE[h % PALETTE.length];
}
