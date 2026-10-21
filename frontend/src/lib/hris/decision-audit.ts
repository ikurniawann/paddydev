import { wibDateKey } from "./attendance-calendar";

/**
 * Nama pemutus untuk decided_by_name dan aktor audit: nama akun, "HRD" bila
 * kosong (sama dengan Go actorName; EPIC-063).
 */
export function decisionActorName(fullName: string | null | undefined): string {
  return fullName?.trim() || "HRD";
}

/**
 * Tanggal untuk label audit: YYYY-MM-DD kalender WIB, seperti Go. node-pg
 * mengembalikan kolom date sebagai Date (tengah malam lokal), jadi tidak
 * boleh dipotong lewat toISOString; string YYYY-MM-DD dipakai apa adanya.
 */
export function auditDateText(value: string | Date | null | undefined): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "string" && /^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
  return wibDateKey(value);
}
