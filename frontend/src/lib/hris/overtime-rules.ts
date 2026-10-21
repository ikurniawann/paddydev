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
