import type { LeaveQuota } from "@/features/hris/leaves/types";
import { leaveTypeInfo, type LeaveTypeInfo } from "./leave-types";

/** Catatan di bawah form untuk jenis berkuota (EPIC-058). Hari saja, tanpa rupiah. */
export function leaveFormNote(code: string, quota: LeaveQuota | null): string | null {
  const info = leaveTypeInfo(code);
  if (!info?.monthlyQuota || !quota) return null;
  const q = quota.types.find((t) => t.leave_type === code);
  if (!q) return null;
  const deducts = q.deducts_thp ?? info.deductsThpDay;
  const effect = deducts
    ? "Memotong 1 hari THP per hari izin."
    : info.deductsThpDay
      ? "Tidak memotong THP (status Magang)."
      : "Tidak memotong gaji.";
  return `Sisa kuota ${info.label} bulan ${quota.month}: ${q.remaining} dari ${q.quota} hari. ${effect} Diajukan per satu hari.`;
}

/** Jenis satu hari: tanggal selesai mengikuti tanggal mulai. */
export function applySingleDay(code: string, start: string, end: string) {
  return leaveTypeInfo(code)?.singleDay ? { start, end: start } : { start, end };
}

/** Jenis yang tersimpan di form; jatuh ke "annual" bila sudah tidak ada di opsi (mis. Izin Leader). */
export function effectiveLeaveType(code: string, options: { value: string }[]): string {
  return options.some((o) => o.value === code) ? code : "annual";
}

/** Teks khusus per jenis (TnC 03-11, EPIC-084); aturan angka dari katalog. */
const RULE_NOTES: Partial<Record<string, string>> = {
  leader: "1x per bulan, hangus bila tidak dipakai, tanpa potong gaji",
  birthday: "1 hari dalam bulan ulang tahun",
  personal: "Hanya bila belum punya hak cuti atau cuti tahunan habis",
  maternity: "upah penuh bila masa kerja ≥ 1 tahun",
  miscarriage: "upah penuh bila masa kerja ≥ 1 tahun",
  pilgrimage: "lewat 14 hari tidak dibayar bila masa kerja < 1 tahun",
};

const ATTACHMENT_HINTS: Partial<Record<string, string>> = {
  birthday: "wajib lampiran (mis. foto KTP)",
  miscarriage: "wajib lampiran (surat dokter)",
};

/** Ringkasan aturan pengajuan di bawah pilihan jenis (EPIC-084 R1/R2). Server tetap memutuskan. */
export function leaveRuleHint(info: LeaveTypeInfo): string | null {
  const parts: string[] = [];
  if (info.sameDayOnly) parts.push("Diajukan pada hari sakit");
  else if (info.leadDays) parts.push(`Diajukan paling lambat H-${info.leadDays}`);
  if (info.maxDays) parts.push(`maksimal ${info.maxDays} hari kerja`);
  if (info.maxSpanDays) parts.push(`maksimal ${info.maxSpanDays} hari kalender`);
  const note = RULE_NOTES[info.code];
  if (note) parts.push(note);
  if (info.attachmentRequired) parts.push(ATTACHMENT_HINTS[info.code] ?? "wajib lampiran");
  if (parts.length === 0) return null;
  const text = parts.join(" · ");
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** Tanggal mulai paling awal untuk pengajuan sendiri (H-n; Sakit = hari ini); null bila bebas. */
export function earliestStartDate(info: LeaveTypeInfo, today: string): string | null {
  if (info.sameDayOnly) return today;
  if (!info.leadDays) return null;
  const d = new Date(`${today}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + info.leadDays);
  return d.toISOString().slice(0, 10);
}

/** Pesan bila jenis wajib lampiran tetapi belum ada lampiran. */
export function leaveAttachmentError(code: string, hasAttachment: boolean): string | null {
  const info = leaveTypeInfo(code);
  return info?.attachmentRequired && !hasAttachment ? `Lampiran wajib untuk ${info.label}` : null;
}
