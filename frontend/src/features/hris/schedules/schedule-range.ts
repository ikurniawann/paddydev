const DAY = 86_400_000;
const parse = (iso: string) => Date.parse(`${iso}T00:00:00Z`);
const iso = (ms: number) => new Date(ms).toISOString().slice(0, 10);

export function rangeError(from: string, to: string): string | null {
  if (!from || !to) return "Pilih tanggal mulai dan selesai";
  const days = Math.round((parse(to) - parse(from)) / DAY) + 1;
  return days < 7 || days > 45 ? "Rentang tanggal 7–45 hari" : null;
}

export function defaultRange(today: string): { from: string; to: string } {
  return { from: today, to: iso(parse(today) + 13 * DAY) };
}
