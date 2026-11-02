import type { LatenessCode, LatenessLevel, LateDay, MyLateness, TeamLatenessMember } from "../types";

const LEVELS: Record<LatenessCode, LatenessLevel> = {
  none: { code: "none", label: "Tidak ada keterlambatan", range: "0 kali", min: 0, max: 0 },
  teguran: { code: "teguran", label: "Teguran lisan dan tertulis", range: "1–2 kali", min: 1, max: 2 },
  sp1: { code: "sp1", label: "Surat Peringatan 1", range: "3–4 kali", min: 3, max: 4 },
  sp2: { code: "sp2", label: "Surat Peringatan 2", range: "5–6 kali", min: 5, max: 6 },
  sp3: { code: "sp3", label: "Surat Peringatan 3 dan/atau PHK", range: "Lebih dari 6 kali", min: 7, max: null },
};

export const level = (code: LatenessCode) => LEVELS[code];

export const lateDay = (date: string, minutes = 12): LateDay => ({
  date, shift_name: "Pagi", scheduled_start: "08:00", clock_in: "08:12", late_minutes: minutes,
});

const envelope = {
  period: { name: "Oktober 2026", label: "1–31 Okt 2026", start_date: "2026-10-01", end_date: "2026-10-31", status: "open", is_current: true },
  periods: [
    { back: 0, name: "Oktober 2026", label: "1–31 Okt 2026" },
    { back: 1, name: "September 2026", label: "1–30 Sep 2026" },
  ],
  rules: Object.values(LEVELS),
};

export function myLateness(code: LatenessCode, days: LateDay[], next: MyLateness["next"] = null): MyLateness {
  return { ...envelope, count: days.length, total_minutes: days.reduce((n, d) => n + d.late_minutes, 0), level: level(code), next, days };
}

export function member(over: Partial<TeamLatenessMember> & { employee_id: string; full_name: string }): TeamLatenessMember {
  return {
    position_title: null, direct: true, reporting_to_name: "Atasan Tim", count: 0, total_minutes: 0,
    level: level("none"), next: null, days: [], ...over,
  };
}

export const teamEnvelope = envelope;
