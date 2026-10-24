/**
 * Label "Kurang jam / pulang lebih cepat" dan jam kerja minimum cabang
 * (EPIC-065, MOM AB-05). Murni, aman diimpor komponen klien. Label sendiri
 * dihitung dan disimpan server (trigger hris.attendance_short_hours).
 */

export const SHORT_HOURS_LABEL = "Kurang jam / pulang lebih cepat";
export const MIN_WORK_HOURS_ERROR = "Jam kerja minimum 1–24 jam (kelipatan 0,25)";

/** "9" / "8,5" dari numeric pg ("9.00") atau angka; "" bila kosong. */
export function formatHours(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === "") return "";
  const n = Number(value);
  if (!Number.isFinite(n)) return "";
  return n.toLocaleString("id-ID", { maximumFractionDigits: 2 });
}

/** Keterangan badge: "Hadir kurang dari 9 jam (jam minimum toko)". */
export function shortHoursDetail(min: string | number | null | undefined): string {
  const hours = formatHours(min);
  return hours ? `Hadir kurang dari ${hours} jam (jam minimum toko)` : SHORT_HOURS_LABEL;
}

function parseHours(input: string): number {
  return Number(input.trim().replace(",", "."));
}

/** "" bila valid; input kosong = tanpa aturan (sama dengan server). */
export function validateMinWorkHours(input: string): string {
  if (input.trim() === "") return "";
  const n = parseHours(input);
  if (!Number.isFinite(n) || n < 1 || n > 24 || Math.round(n * 4) !== n * 4) return MIN_WORK_HOURS_ERROR;
  return "";
}

/** Body PUT /api/hris/branches/{id}/min-work-hours. */
export function toMinWorkHoursPayload(input: string): { min_work_hours: number | null } {
  return { min_work_hours: input.trim() === "" ? null : parseHours(input) };
}
