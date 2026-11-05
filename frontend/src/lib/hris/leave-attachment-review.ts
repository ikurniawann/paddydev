/**
 * Verifikasi lampiran cuti/izin oleh HR (EPIC-088, ES-11). Selama menunggu HR
 * status tetap "pending"; tahapnya di attachment_review.
 */
export type LeaveAttachmentReview = "awaiting" | "returned" | "verified";

/** Pesan server saat atasan menyetujui pengajuan berlampiran (R3). */
export const MANAGER_APPROVED_AWAITING_HR = "Disetujui atasan — menunggu verifikasi lampiran oleh HR";

export const LEAVE_REVIEW_STAGE_LABELS = {
  awaiting: "Menunggu verifikasi HR",
  returned: "Lampiran dikembalikan",
} as const;

export type LeaveReviewStage = keyof typeof LEAVE_REVIEW_STAGE_LABELS;

export const LEAVE_REVIEW_STAGE_CLASSES: Record<LeaveReviewStage, string> = {
  awaiting: "bg-sky-100 text-sky-800",
  returned: "bg-orange-100 text-orange-800",
};

/** Tahap verifikasi pengajuan pending (awaiting/returned); null selain itu. */
export function leaveReviewStage(leave: {
  status: string;
  attachment_review?: string | null;
}): LeaveReviewStage | null {
  if (leave.status !== "pending") return null;
  return leave.attachment_review === "awaiting" || leave.attachment_review === "returned"
    ? leave.attachment_review
    : null;
}
