/** Saldo cuti satu karyawan untuk satu tahun (EPIC-068 R12), bentuk yang sama untuk GET, daftar, dan PUT. */
export interface LeaveBalanceView {
  employee_id: string;
  nip: string | null;
  full_name: string;
  branch_name: string | null;
  year: number;
  annual_leave_total: number;
  annual_leave_adjustment: number;
  annual_leave_used: number;
  annual_leave_remaining: number;
  carry_over_days: number;
  carry_over_used: number;
  carry_over_remaining: number;
  /** YYYY-MM-DD; null = sisa bawaan tidak berlaku. */
  carry_over_expires_on: string | null;
  carry_over_active: boolean;
  /** Sisa tersedia hari ini (WIB): sisa jatah + sisa bawaan yang masih berlaku. */
  available: number;
  updated_at: string | null;
  /** false = baris belum tersimpan (tampil dari jatah default). */
  exists: boolean;
}

export interface LeaveBalanceListParams {
  year: number;
  branch_id: string;
  q: string;
}

/** Body PUT /api/hris/leave-balances/{employee_id} (R11). Terpakai tidak pernah dikirim. */
export interface LeaveBalancePayload {
  year: number;
  annual_leave_total: number;
  annual_leave_adjustment: number;
  carry_over_days: number;
  carry_over_expires_on: string | null;
  reason: string;
}

/** Aksi Saldo Cuti milik akun ini (GET /api/hris/me/leave-balance-access). */
export interface LeaveBalanceAccess {
  read: boolean;
  update: boolean;
  import: boolean;
  export: boolean;
}

/** GET/PUT /api/hris/leave-settings (R8). */
export interface LeaveSettings {
  carry_over_enabled: boolean;
  /** null = semua sisa dibawa. */
  carry_over_max_days: number | null;
  carry_over_expiry_month: number;
  carry_over_expiry_day: number;
  updated_at: string | null;
  updated_by_name: string | null;
  /** false sampai migrasi perhitungan ulang sisa bawaan terpasang; selama itu sisa bawaan tidak bisa dinyalakan. */
  carry_over_ready: boolean;
}

/** PUT adalah patch; carry_over_max_days: null menghapus batas. */
export type LeaveSettingsPatch = Partial<
  Pick<
    LeaveSettings,
    "carry_over_enabled" | "carry_over_max_days" | "carry_over_expiry_month" | "carry_over_expiry_day"
  >
>;

/** Satu masalah di file import: galat (menghalangi simpan) atau peringatan (perlu dicentang). */
export interface LeaveBalanceImportIssue {
  line: number;
  column: string;
  name: string;
  message: string;
}

/** Angka saldo satu karyawan di pratinjau import (R10). */
export interface LeaveBalanceImportFigures {
  annual_leave_total: number;
  annual_leave_adjustment: number;
  carry_over_days: number;
  carry_over_expires_on: string | null;
}

export interface LeaveBalanceImportChange {
  nip: string;
  name: string;
  before: LeaveBalanceImportFigures;
  after: LeaveBalanceImportFigures;
  /** true = baris saldo belum ada dan akan dibuat. */
  new: boolean;
  reason: string;
}

/** Isi `data` untuk step=preview; step=commit hanya mengembalikan `summary`. */
export interface LeaveBalanceImportPreview {
  summary: { employees: number; new: number; changed: number; same: number };
  errors?: LeaveBalanceImportIssue[];
  warnings?: LeaveBalanceImportIssue[];
  fingerprint?: string;
  /** Paling banyak 500. */
  changes?: LeaveBalanceImportChange[];
}

export interface LeaveBalanceImportScope {
  year: number;
  /** "" = semua cabang. */
  branch_id: string;
}
