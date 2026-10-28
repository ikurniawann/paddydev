/** Tanggal ISO (YYYY-MM-DD) untuk Jadwal Kerja; dihitung di UTC supaya tidak bergeser zona waktu. */
const DAY = 86_400_000;
const parse = (iso: string) => Date.parse(`${iso}T00:00:00Z`);
const iso = (ms: number) => new Date(ms).toISOString().slice(0, 10);

export function addDays(date: string, n: number): string {
  return iso(parse(date) + n * DAY);
}

/** Selisih hari (to − from). */
export function dayDiff(from: string, to: string): number {
  return Math.round((parse(to) - parse(from)) / DAY);
}

/** 0 = Senin … 6 = Minggu. */
export function weekdayIndex(date: string): number {
  return (new Date(parse(date)).getUTCDay() + 6) % 7;
}

export function mondayOf(date: string): string {
  return addDays(date, -weekdayIndex(date));
}

/** "04/11". */
export const dm = (date: string) => `${date.slice(8, 10)}/${date.slice(5, 7)}`;

export function rangeError(from: string, to: string): string | null {
  if (!from || !to) return "Pilih tanggal mulai dan selesai";
  const days = dayDiff(from, to) + 1;
  return days < 7 || days > 45 ? "Rentang tanggal 7–45 hari" : null;
}

/**
 * Rentang template & unggah Excel dari tampilan: tanggal lewat ditolak server,
 * jadi mulai paling awal hari ini, dan minimal 7 hari.
 */
export function uploadRange(visibleFrom: string, visibleTo: string, today: string): { from: string; to: string } {
  const from = visibleFrom > today ? visibleFrom : today;
  const minTo = addDays(from, 6);
  return { from, to: visibleTo > minTo ? visibleTo : minTo };
}

/** "4 Nov" atau "4 Nov 2026". */
export function dayLabel(date: string, withYear = false): string {
  return new Date(parse(date)).toLocaleDateString("id-ID", {
    day: "numeric",
    month: "short",
    ...(withYear ? { year: "numeric" } : {}),
    timeZone: "UTC",
  });
}
