/** Jenis surat yang diterbitkan HR (EPIC-094 R9). */
export type LetterType = "teguran" | "sp1" | "sp2" | "sp3";

export const LETTER_TYPES: { value: LetterType; label: string }[] = [
  { value: "teguran", label: "Surat Teguran" },
  { value: "sp1", label: "Surat Peringatan 1" },
  { value: "sp2", label: "Surat Peringatan 2" },
  { value: "sp3", label: "Surat Peringatan 3" },
];

/** Satu baris GET /api/hris/disciplinary-letters. */
export interface DisciplinaryLetter {
  id: string;
  employee_id: string;
  employee_name: string;
  nip: string | null;
  letter_type: LetterType;
  label: string;
  /** YYYY-MM-DD */
  issued_date: string;
  notes: string | null;
  attachment_path: string | null;
  created_by_name: string | null;
  created_at: string;
  /** Terisi = surat dibatalkan; tidak tampil lagi di ESS karyawan. */
  revoked_at: string | null;
  revoked_by_name: string | null;
  revoke_reason: string | null;
}

export interface IssueLetterPayload {
  employee_id: string;
  letter_type: LetterType;
  issued_date: string;
  notes: string | null;
  attachment_path: string | null;
}

/** Keterlambatan periode berjalan sebagai bantuan HR (R11). */
export interface LetterLateness {
  count: number;
  level: string;
  label: string;
  period_start: string;
  period_end: string;
}

export interface LetterListParams {
  employeeId?: string;
  includeRevoked: boolean;
}
