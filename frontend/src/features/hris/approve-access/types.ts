/** Kapabilitas aksi `approve` akun ini per daun menu HR (EPIC-067 R14, GET /api/hris/me/approve-access). */
export interface HrApproveAccess {
  /** hris.workforce.attendance: pengajuan absensi + Perjalanan Dinas jalur HR. */
  attendance: boolean;
  /** hris.workforce.leaves: keputusan HR cadangan/override cuti. */
  leaves: boolean;
  /** hris.kepegawaian.overtime: keputusan HR cadangan/override lembur. */
  overtime: boolean;
}
