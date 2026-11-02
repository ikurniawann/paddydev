import type { HolidayType } from "./holidays";
import { defaultDeductsLeave } from "./holidays-input";

/**
 * Logika murni halaman HRIS Hari Libur (EPIC-036): form tambah/ubah, kartu
 * per bulan, dan pilihan awal pratinjau impor kalender.
 */

export type HolidayStatus = "draft" | "aktif";

interface HolidayForm {
  /** "YYYY-MM-DD" */
  holiday_date: string;
  name: string;
  type: HolidayType;
  deducts_leave: boolean;
  /** Hanya bermakna untuk cuti bersama (EPIC-079): ditetapkan libur oleh perusahaan. */
  is_day_off: boolean;
  status: HolidayStatus;
  note: string;
}

export function emptyHolidayForm(year: number): HolidayForm {
  return {
    holiday_date: `${year}-01-01`,
    name: "",
    type: "nasional",
    deducts_leave: false,
    is_day_off: false,
    status: "aktif",
    note: "",
  };
}

export function holidayFormFrom(
  row: Omit<HolidayForm, "note" | "is_day_off"> & { note: string | null; is_day_off?: boolean }
): HolidayForm {
  return {
    holiday_date: row.holiday_date,
    name: row.name,
    type: row.type,
    deducts_leave: row.deducts_leave,
    is_day_off: row.type === "cuti_bersama" && (row.is_day_off ?? false),
    status: row.status,
    note: row.note ?? "",
  };
}

/** Ganti tipe sekaligus default potong cuti mengikuti SKB; penetapan libur hanya milik cuti bersama. */
export function withHolidayType(form: HolidayForm, type: HolidayType): HolidayForm {
  return {
    ...form,
    type,
    deducts_leave: defaultDeductsLeave(type),
    is_day_off: type === "cuti_bersama" && form.is_day_off,
  };
}

export function holidayPayload(form: HolidayForm) {
  return {
    holiday_date: form.holiday_date,
    name: form.name.trim(),
    type: form.type,
    deducts_leave: form.deducts_leave,
    is_day_off: form.type === "cuti_bersama" && form.is_day_off,
    status: form.status,
    note: form.note.trim() || null,
  };
}

/** Indeks 0-11 berisi libur pada bulan itu, urutan masukan dipertahankan. */
export function groupHolidaysByMonth<T extends { holiday_date: string }>(rows: readonly T[]): T[][] {
  const groups: T[][] = Array.from({ length: 12 }, () => []);
  for (const row of rows) groups[Number(row.holiday_date.slice(5, 7)) - 1]?.push(row);
  return groups;
}

/** Chip kalender: "17" + "Min". Dihitung di UTC supaya tanggal kalender tidak bergeser. */
export function holidayDayLabel(dateIso: string): { day: string; weekday: string } {
  const date = new Date(`${dateIso}T00:00:00Z`);
  return {
    day: String(date.getUTCDate()).padStart(2, "0"),
    weekday: date.toLocaleDateString("id-ID", { weekday: "short", timeZone: "UTC" }),
  };
}

/** Pratinjau impor: yang disarankan dan belum pernah diimpor tercentang otomatis. */
export function defaultImportSelection(
  rows: readonly { source_ref: string; suggested: boolean; already_imported: boolean }[]
): Set<string> {
  return new Set(rows.filter((r) => r.suggested && !r.already_imported).map((r) => r.source_ref));
}

export function toggleInSet<T>(set: ReadonlySet<T>, value: T): Set<T> {
  const next = new Set(set);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}
