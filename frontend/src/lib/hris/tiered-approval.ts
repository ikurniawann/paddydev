/**
 * Approval berjenjang cuti/izin & perjalanan dinas (EPIC-092): SPV dulu, lalu
 * Manager bila ada. Selama tahap Manager status tetap "pending"; tahap SPV
 * yang sudah lewat tercatat di spv_approved_at/spv_approved_name.
 */
export interface SpvStageFields {
  status: string;
  spv_approved_at?: string | null;
  spv_approved_name?: string | null;
}

export const SPV_APPROVED_LABEL = "Disetujui SPV — menunggu Manager";
/** Pesan server saat SPV (atau HR mewakili) menyetujui tahap SPV. */
export const SPV_APPROVED_MESSAGE = "Disetujui SPV — menunggu persetujuan Manager";
export const SPV_APPROVED_CLASS = "bg-indigo-100 text-indigo-800";

/** Pending dan tahap SPV sudah lewat: kini giliran Manager. */
export function spvStageDone(row: SpvStageFields): boolean {
  return row.status === "pending" && !!row.spv_approved_at;
}

/** Siapa yang menyetujui tahap SPV (untuk Detail); null bila belum ada. */
export function spvApprovedNote(row: Omit<SpvStageFields, "status">): string | null {
  if (!row.spv_approved_at) return null;
  return `Disetujui SPV oleh ${row.spv_approved_name || "SPV"}`;
}
