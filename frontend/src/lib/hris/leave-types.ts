/**
 * Katalog jenis cuti (EPIC-058). Cermin dari
 * backend/internal/modules/hris/domain/leave_types.go: ubah keduanya bersamaan.
 * Jenis dengan `selectable: false` sudah dihapus untuk pengajuan baru, tetapi
 * tetap dikenal supaya riwayat terbaca.
 */

export type LeaveTypeCode =
  | "annual" | "sick" | "personal" | "leader" | "maternity" | "paternity"
  | "pilgrimage" | "marriage" | "bereavement" | "emergency" | "unpaid" | "menstrual";

export interface LeaveTypeInfo {
  code: LeaveTypeCode;
  label: string;
  short: string;
  exportLabel: string;
  selectable: boolean;
  monthlyQuota?: number;
  singleDay?: boolean;
  leaderOnly?: boolean;
  /** Memotong 1 hari THP per hari izin (rule PIP payroll). */
  deductsThpDay?: boolean;
}

export const LEAVE_TYPES: readonly LeaveTypeInfo[] = [
  { code: "annual", label: "Cuti Tahunan", short: "Tahunan", exportLabel: "Cuti Tahunan", selectable: true },
  { code: "sick", label: "Sakit", short: "Sakit", exportLabel: "Cuti Sakit", selectable: true },
  { code: "personal", label: "Izin Pribadi", short: "Izin Pribadi", exportLabel: "Izin Pribadi", selectable: true,
    monthlyQuota: 3, singleDay: true, deductsThpDay: true },
  { code: "leader", label: "Izin Leader", short: "Izin Leader", exportLabel: "Izin Leader", selectable: true,
    monthlyQuota: 1, singleDay: true, leaderOnly: true },
  { code: "maternity", label: "Cuti Melahirkan", short: "Melahirkan", exportLabel: "Cuti Melahirkan", selectable: true },
  { code: "paternity", label: "Cuti Ayah", short: "Ayah", exportLabel: "Cuti Ayah", selectable: true },
  { code: "pilgrimage", label: "Cuti Ibadah Haji/Umrah", short: "Haji/Umrah", exportLabel: "Cuti Haji/Umrah", selectable: true },
  { code: "marriage", label: "Cuti Menikah", short: "Pernikahan", exportLabel: "Cuti Menikah", selectable: true },
  { code: "bereavement", label: "Cuti Duka", short: "Duka Cita", exportLabel: "Cuti Duka", selectable: true },
  // Dihapus untuk pengajuan baru (EPIC-058); riwayat tetap terbaca.
  { code: "emergency", label: "Izin Darurat", short: "Darurat", exportLabel: "Cuti Darurat", selectable: false },
  { code: "unpaid", label: "Izin Tanpa Gaji", short: "Tidak Dibayar", exportLabel: "Cuti Tanpa Upah", selectable: false },
  { code: "menstrual", label: "Cuti Haid", short: "Haid", exportLabel: "Cuti Haid", selectable: false },
];

const byCode = new Map<string, LeaveTypeInfo>(LEAVE_TYPES.map((t) => [t.code, t]));

export const LEAVE_TYPE_LABELS = Object.fromEntries(
  LEAVE_TYPES.map((t) => [t.code, t.label])
) as Record<LeaveTypeCode, string>;

export const SELECTABLE_LEAVE_CODES = LEAVE_TYPES.filter((t) => t.selectable).map((t) => t.code) as unknown as readonly [
  LeaveTypeCode,
  ...LeaveTypeCode[],
];

export const leaveTypeInfo = (code: string): LeaveTypeInfo | undefined => byCode.get(code);
export const leaveTypeLabel = (code: string): string => byCode.get(code)?.label ?? code;
export const leaveTypeShort = (code: string): string => byCode.get(code)?.short ?? code;
export const leaveTypeExportLabel = (code: string): string => byCode.get(code)?.exportLabel ?? code;

/** Opsi form pengajuan: hanya jenis yang berlaku; Izin Leader hanya untuk Supervisor ke atas. */
export function leaveTypeOptions({ leaderEligible }: { leaderEligible: boolean }) {
  return LEAVE_TYPES.filter((t) => t.selectable && (!t.leaderOnly || leaderEligible)).map((t) => ({
    value: t.code,
    label: t.label,
  }));
}
