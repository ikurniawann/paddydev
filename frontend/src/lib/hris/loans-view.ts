import { computeLoanTerms } from "@/lib/payroll/loans";

/**
 * Logika murni halaman HRIS Pinjaman (EPIC-008 Fase D): form pengajuan,
 * pratinjau cicilan, ringkasan header, dan progres pelunasan.
 */

/** Jenis yang bisa diajukan (EPIC-060: Pinjaman Darurat dihapus). */
export const LOAN_TYPE_OPTIONS = [
  { value: "kasbon", label: "Kasbon (Salary Advance)" },
  { value: "loan", label: "Pinjaman" },
];

/** Label semua jenis, termasuk yang sudah tidak bisa diajukan (riwayat). */
const LOAN_TYPE_LABELS: Record<string, string> = {
  ...Object.fromEntries(LOAN_TYPE_OPTIONS.map((o) => [o.value, o.label])),
  emergency: "Pinjaman Darurat",
};

export function loanTypeLabel(value: string): string {
  return LOAN_TYPE_LABELS[value] ?? value;
}

/** Tenor kasbon (MOM KP-02): 1 atau 2 bulan; default 2 (50/50). */
export const kasbonTenorOptions = [
  { value: "1", label: "1 bulan" },
  { value: "2", label: "2 bulan (50% / 50%)" },
];

export const LOAN_STATUS_LABELS: Record<string, string> = {
  pending: "Menunggu",
  manager_approved: "Disetujui Manager",
  approved: "Berjalan",
  rejected: "Ditolak",
  paid_off: "Lunas",
};

export interface EmployeeBrief {
  id: string;
  full_name: string;
  nip: string | null;
}

/** Opsi combobox karyawan: "Nama (NIP)". Dipakai juga halaman lembur. */
export function employeeComboOptions(employees: readonly EmployeeBrief[]): { value: string; label: string }[] {
  return employees.map((emp) => ({
    value: emp.id,
    label: emp.nip ? `${emp.full_name} (${emp.nip})` : emp.full_name,
  }));
}

interface LoanForm {
  employee_id: string;
  loan_type: string;
  principal_amount: string;
  interest_rate: string;
  tenor_months: string;
  purpose: string;
}

export const EMPTY_LOAN_FORM: LoanForm = {
  employee_id: "",
  loan_type: "kasbon",
  principal_amount: "",
  interest_rate: "0",
  tenor_months: "2",
  purpose: "",
};

/** Perkiraan cicilan bulanan (bunga flat), null bila pokok/tenor belum diisi. */
export function previewInstallment(form: LoanForm): number | null {
  const principal = Number(form.principal_amount);
  const tenorMonths = Number(form.tenor_months);
  if (!principal || !tenorMonths) return null;
  return computeLoanTerms({ principal, tenorMonths, ratePercent: Number(form.interest_rate) || 0 })
    .monthlyInstallment;
}

export function validateLoanForm(form: LoanForm): string | null {
  if (!form.employee_id) return "Pilih karyawan";
  if (!Number(form.principal_amount) || !Number(form.tenor_months)) {
    return "Jumlah pinjaman dan tenor wajib diisi";
  }
  if (form.loan_type === "kasbon" && !["1", "2"].includes(form.tenor_months)) return "Tenor kasbon hanya 1 atau 2 bulan";
  return null;
}

export function loanPayload(form: LoanForm) {
  return {
    employee_id: form.employee_id,
    loan_type: form.loan_type,
    principal_amount: Number(form.principal_amount),
    interest_rate: Number(form.interest_rate) || 0,
    tenor_months: Number(form.tenor_months),
    purpose: form.purpose || undefined,
  };
}

type Amount = string | number;

/**
 * Pengajuan yang menunggu keputusan HR: kasbon yang sudah disetujui manager,
 * atau pending tanpa manager (kasbon lama / pinjaman biasa).
 */
export function isAwaitingHr(row: { status: string; manager_approver_id?: string | null }): boolean {
  return row.status === "manager_approved" || (row.status === "pending" && !row.manager_approver_id);
}

/** Jumlah pengajuan menunggu HR dan total sisa pinjaman berjalan. */
export function loanSummary(
  rows: readonly { status: string; remaining_balance: Amount; manager_approver_id?: string | null }[]
) {
  let pendingCount = 0;
  let activeTotal = 0;
  for (const row of rows) {
    if (isAwaitingHr(row)) pendingCount += 1;
    if (row.status === "approved") activeTotal += Number(row.remaining_balance) || 0;
  }
  return { pendingCount, activeTotal };
}

/** Persentase terbayar (bulat, 0-100). */
export function loanPaidPercent(row: { paid_amount: Amount; remaining_balance: Amount }): number {
  const paid = Number(row.paid_amount) || 0;
  const total = paid + (Number(row.remaining_balance) || 0);
  return total > 0 ? Math.min(100, Math.round((paid / total) * 100)) : 0;
}
