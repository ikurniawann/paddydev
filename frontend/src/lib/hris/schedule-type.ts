/** Tipe jadwal karyawan (Spec 2026-10-19 §2.2). */
export type ScheduleType = "shift" | "non_shift";

export const SCHEDULE_TYPE_OPTIONS: { value: ScheduleType; label: string }[] = [
  { value: "shift", label: "Shift" },
  { value: "non_shift", label: "Non-Shift" },
];

export function scheduleTypeLabel(t: ScheduleType | null | undefined): string {
  return SCHEDULE_TYPE_OPTIONS.find((o) => o.value === t)?.label ?? "—";
}

/** "2026-10-21" → "21/10/2026". */
export function dmy(iso: string): string {
  const [y, m, d] = iso.slice(0, 10).split("-");
  return y && m && d ? `${d}/${m}/${y}` : iso;
}

export function scheduleTypeConfirmText(to: ScheduleType, from: string, futureDays: number): string {
  if (to === "non_shift") {
    return futureDays > 0
      ? `${futureDays} jadwal per tanggal mulai ${dmy(from)} akan dihapus dan diganti jadwal tetap Senin–Jumat 09.00–18.00.`
      : `Jadwal tetap Senin–Jumat 09.00–18.00 berlaku mulai ${dmy(from)}.`;
  }
  return `Jadwal tetap berhenti mulai ${dmy(from)}. Karyawan perlu ditugaskan di Jadwal Kerja.`;
}

