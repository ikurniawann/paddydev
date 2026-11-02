import type { HolidayType } from "@/lib/hris/holidays";
import type { HolidayStatus, holidayPayload } from "@/lib/hris/holidays-view";

export interface HolidayRow {
  id: string;
  /** "YYYY-MM-DD" */
  holiday_date: string;
  name: string;
  type: HolidayType;
  deducts_leave: boolean;
  /** Cuti bersama yang ditetapkan libur oleh perusahaan (EPIC-079). */
  is_day_off: boolean;
  status: HolidayStatus;
  source: "manual" | "impor";
  note: string | null;
}

/** Satu baris preview impor ICS (EPIC-036 Fase E). */
export interface HolidayImportRow {
  source_ref: string;
  holiday_date: string;
  name: string;
  type: HolidayType;
  deducts_leave: boolean;
  status: HolidayStatus;
  /** Dicentang otomatis? false = kandidat yang bukan tanggal merah. */
  suggested: boolean;
  reason?: string;
  already_imported: boolean;
}

export type HolidayPayload = ReturnType<typeof holidayPayload>;

export type HolidayImportItem = Pick<
  HolidayImportRow,
  "source_ref" | "holiday_date" | "name" | "type" | "deducts_leave" | "status"
>;
