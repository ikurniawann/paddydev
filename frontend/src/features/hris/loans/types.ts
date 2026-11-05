import type { loanPayload } from "@/lib/hris/loans-view";

export interface LoanRow {
  id: string;
  employee_id: string;
  loan_type: string;
  principal_amount: string | number;
  interest_rate: string | number;
  tenor_months: number;
  monthly_installment: string | number;
  remaining_balance: string | number;
  paid_amount: string | number;
  first_installment_month: number | null;
  first_installment_year: number | null;
  status: string;
  purpose: string | null;
  /** Catatan pengajuan (EPIC-072: tampil di Detail). */
  notes?: string | null;
  rejection_reason: string | null;
  manager_approver_id?: string | null;
  /** Tahap SPV kasbon/pinjaman (EPIC-091) — terisi saat SPV (atau HR mewakili) menyetujui. */
  supervisor_approver_id?: string | null;
  supervisor_approved_at?: string | null;
  created_at: string;
  /** Dokumen pendukung & jaminan (EPIC-076 R8). */
  attachments?: string[];
  collateral_attachments?: string[];
  employee?: {
    id: string;
    full_name: string;
    nip: string | null;
    department?: { name: string } | null;
  } | null;
}

export type LoanPayload = ReturnType<typeof loanPayload>;

export interface LoanDecision {
  id: string;
  approved: boolean;
  rejection_reason?: string;
}
