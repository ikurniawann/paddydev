import type { LeaveQuota } from "@/features/hris/leaves/types";
import { leaveTypeInfo } from "./leave-types";

/** Catatan di bawah form untuk jenis berkuota (EPIC-058). Hari saja, tanpa rupiah. */
export function leaveFormNote(code: string, quota: LeaveQuota | null): string | null {
  const info = leaveTypeInfo(code);
  if (!info?.monthlyQuota || !quota) return null;
  const q = quota.types.find((t) => t.leave_type === code);
  if (!q) return null;
  const effect = info.deductsThpDay ? "Memotong 1 hari THP per hari izin." : "Tidak memotong gaji.";
  return `Sisa kuota ${info.label} bulan ${quota.month}: ${q.remaining} dari ${q.quota} hari. ${effect} Diajukan per satu hari.`;
}

/** Jenis satu hari: tanggal selesai mengikuti tanggal mulai. */
export function applySingleDay(code: string, start: string, end: string) {
  return leaveTypeInfo(code)?.singleDay ? { start, end: start } : { start, end };
}
