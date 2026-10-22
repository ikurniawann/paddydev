/**
 * Aturan otorisasi keputusan pengajuan lembur (EPIC-008 Fase B) — murni &
 * teruji, dipakai POST /api/hris/overtime/decide.
 *
 * source 'employee' : HRD atau atasan langsung yang memutuskan;
 *                     pengaju TIDAK boleh memutuskan pengajuannya sendiri
 *                     sekalipun ber-role HR (jam lembur masuk gaji).
 * source 'company'  : hanya karyawan yang ditugaskan yang mengonfirmasi.
 * cancel            : pembuat pengajuan (atau HR utk penugasan company),
 *                     selama masih pending.
 */

export interface OvertimeDecisionActor {
  employeeId: string | null;
  isHr: boolean;
}

export interface OvertimeDecisionRequest {
  employee_id: string;
  requested_by: string | null;
  source: "employee" | "company";
  reporting_to: string | null;
}

export type OvertimeDecisionAction = "approve" | "reject" | "cancel";

export function canDecideOvertime(
  actor: OvertimeDecisionActor,
  request: OvertimeDecisionRequest,
  action: OvertimeDecisionAction
): { allowed: boolean; reason?: string } {
  const isTargetEmployee =
    actor.employeeId !== null && request.employee_id === actor.employeeId;
  const isRequester =
    actor.employeeId !== null && request.requested_by === actor.employeeId;
  const isDirectManager =
    actor.employeeId !== null && request.reporting_to === actor.employeeId;

  if (action === "cancel") {
    const canCancel = isRequester || (request.source === "company" && actor.isHr);
    return canCancel
      ? { allowed: true }
      : { allowed: false, reason: "Hanya pembuat pengajuan yang bisa membatalkan" };
  }

  if (request.source === "company") {
    return isTargetEmployee
      ? { allowed: true }
      : {
          allowed: false,
          reason: "Hanya karyawan yang ditugaskan yang bisa mengonfirmasi penugasan ini",
        };
  }

  // source 'employee'
  if (isTargetEmployee) {
    return {
      allowed: false,
      reason: "Tidak bisa memutuskan pengajuan lembur sendiri",
    };
  }
  if (!actor.isHr && !isDirectManager) {
    return {
      allowed: false,
      reason: "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini",
    };
  }
  return { allowed: true };
}

/* ── EPIC-063: approval atasan & lembur sebelum dimulai ─────────────── */

/** K-13: Weekday = regular (1,5×), Weekend / Libur = rest_day (2×); tarif dari setting payroll. */
export type OvertimeDayType = "regular" | "rest_day";

export const OVERTIME_DAY_TYPE_OPTIONS: readonly { value: OvertimeDayType; label: string }[] = [
  { value: "regular", label: "Weekday" },
  { value: "rest_day", label: "Weekend / Libur" },
];

export function overtimeDayTypeLabel(dayType: string): string {
  return dayType === "rest_day" ? "Weekend / Libur" : "Weekday";
}

export const OVERTIME_TOO_LATE = "Lembur wajib diajukan sebelum dimulai — jam mulai sudah lewat";

/**
 * MOM AP-02 (EPIC-063 R3): tanggal + jam mulai (jam dinding WIB) sebelum awal
 * menit berjalan WIB. Sama dengan Go domain.OvertimeStartsInPast; format atau
 * tanggal kalender yang tidak dikenal = false (diserahkan ke validasi input).
 */
export function overtimeStartsInPast(date: string, start: string, now: Date = new Date()): boolean {
  const d = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  const t = /^(\d{2}):(\d{2})$/.exec(start);
  if (!d || !t) return false;
  const [year, month, day, hour, minute] = [d[1], d[2], d[3], t[1], t[2]].map(Number);
  const calendar = new Date(Date.UTC(year, month - 1, day));
  if (
    calendar.getUTCFullYear() !== year ||
    calendar.getUTCMonth() !== month - 1 ||
    calendar.getUTCDate() !== day ||
    hour > 23 ||
    minute > 59
  ) {
    return false;
  }
  const startUtc = Date.UTC(year, month - 1, day, hour - 7, minute);
  const minuteNow = Math.floor(now.getTime() / 60_000) * 60_000;
  return startUtc < minuteNow;
}

/**
 * Kapasitas pemutus cuti/lembur (Q-APV-3, EPIC-063 R1/R2): atasan langsung →
 * "manager", HR → "hr" (cadangan & override), null = tidak boleh. Pemohon
 * tidak pernah memutus pengajuannya sendiri. Sama dengan Go domain.DecideAs.
 * `isHr` di sini adalah himpunan HRD (super_admin/admin/hrd, ruling C1/P3).
 */
export function decidedAs(
  actor: OvertimeDecisionActor,
  ownerId: string,
  reportingTo: string | null
): "manager" | "hr" | null {
  if (actor.employeeId !== null && actor.employeeId === ownerId) return null;
  if (actor.employeeId !== null && reportingTo === actor.employeeId) return "manager";
  return actor.isHr ? "hr" : null;
}

/* ── Input lembur setelah selesai (EPIC-066) ─────────────────────────── */

export const OVERTIME_ATTACHMENT_ACCEPT = "application/pdf,image/jpeg,image/png,image/webp";
export const MAX_OVERTIME_ATTACHMENTS = 3;
export const MAX_OVERTIME_ATTACHMENT_BYTES = 5 * 1024 * 1024;

function clockMinutes(s: string): number | null {
  const m = /^(\d{2}):(\d{2})/.exec(s);
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  return h <= 23 && min <= 59 ? h * 60 + min : null;
}

/** Durasi kotor (menit), lewat tengah malam bila selesai tidak setelah mulai. */
function grossMinutes(start: string, end: string): number {
  const s = clockMinutes(start);
  const e = clockMinutes(end);
  if (s === null || e === null) return 0;
  return e <= s ? e + 1440 - s : e - s;
}

/** Akhir lembur di jam dinding WIB; null bila input tidak valid. */
function overtimeEnd(date: string, start: string, end: string): Date | null {
  const s = clockMinutes(start);
  const gross = grossMinutes(start, end);
  if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || s === null || gross === 0) return null;
  const base = new Date(`${date}T00:00:00+07:00`);
  return Number.isNaN(base.getTime()) ? null : new Date(base.getTime() + (s + gross) * 60_000);
}

/** Server: Lembur baru bisa diinput setelah selesai (EPIC-066). */
export function overtimeNotFinished(date: string, start: string, end: string, now: Date = new Date()): boolean {
  const ends = overtimeEnd(date, start, end);
  return ends !== null && now.getTime() < ends.getTime();
}

/** "DD-MM-YYYY HH:MM" (WIB) akhir lembur, seperti pesan server. */
export function overtimeEndLabel(date: string, start: string, end: string): string {
  const ends = overtimeEnd(date, start, end);
  if (!ends) return "";
  const wibTime = new Date(ends.getTime() + 7 * 3600_000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(wibTime.getUTCDate())}-${pad(wibTime.getUTCMonth() + 1)}-${wibTime.getUTCFullYear()} ${pad(wibTime.getUTCHours())}:${pad(wibTime.getUTCMinutes())}`;
}

/** Jam dibayar = (durasi − istirahat) / 60, 2 desimal, tidak negatif. */
export function paidOvertimeHours(start: string, end: string, breakMinutes: number): number {
  const minutes = Math.max(0, grossMinutes(start, end) - (Number.isFinite(breakMinutes) ? breakMinutes : 0));
  return Math.round((minutes / 60) * 100) / 100;
}

/** Tautan dokumentasi lembur privat (pemilik, atasan langsung, HR). */
export function overtimeAttachmentHref(path: string): string {
  return `/api/hris/overtime/attachment/${path.split("/").map(encodeURIComponent).join("/")}`;
}

/** Jam & istirahat lembur yang dikoreksi atasan saat menyetujui ("HH:MM", menit). */
export interface OvertimeTimes {
  start_time: string;
  end_time: string;
  break_minutes: number;
}

/**
 * Hanya kolom yang BERBEDA dari nilai tersimpan (HH:MM tanpa detik; istirahat
 * sebagai angka). Server memeriksa kewenangan koreksi sebelum membandingkan,
 * jadi menyetujui tanpa perubahan tidak boleh mengirim jam sama sekali.
 */
export function overtimeTimeChanges(
  stored: { start_time: string; end_time: string; break_minutes?: number | string | null },
  next: OvertimeTimes
): Partial<OvertimeTimes> | undefined {
  const changes: Partial<OvertimeTimes> = {};
  if (next.start_time.slice(0, 5) !== stored.start_time.slice(0, 5)) changes.start_time = next.start_time;
  if (next.end_time.slice(0, 5) !== stored.end_time.slice(0, 5)) changes.end_time = next.end_time;
  if (next.break_minutes !== Number(stored.break_minutes ?? 0)) changes.break_minutes = next.break_minutes;
  return Object.keys(changes).length > 0 ? changes : undefined;
}
