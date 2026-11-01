import { computeLoanTerms } from "@/lib/payroll/loans";
import { monthYearLabel } from "@/lib/hris/month-label";
import { privateFileHref } from "@/lib/hris/attachments";

/**
 * Logika murni halaman HRIS Pinjaman (EPIC-008 Fase D): form pengajuan,
 * pratinjau cicilan, ringkasan header, dan progres pelunasan.
 */

/** Jenis yang bisa diajukan (EPIC-060: Pinjaman Darurat dihapus). */
export const LOAN_TYPE_OPTIONS = [
  { value: "kasbon", label: "Kasbon (Salary Advance)" },
  { value: "loan", label: "Pinjaman Karyawan" },
];

/** Label semua jenis, termasuk yang sudah tidak bisa diajukan (riwayat). */
const LOAN_TYPE_LABELS: Record<string, string> = {
  ...Object.fromEntries(LOAN_TYPE_OPTIONS.map((o) => [o.value, o.label])),
  emergency: "Pinjaman Darurat",
};

export function loanTypeLabel(value: string): string {
  return LOAN_TYPE_LABELS[value] ?? value;
}

/**
 * Prosedur kasbon & Pinjaman Karyawan 01-11-2026 (EPIC-076); cermin Go
 * domain.loanRules. Hanya PKWT/PKWTT; probation tidak dapat mengajukan.
 */
export const LOAN_RULES: Record<string, { minServiceMonths: number; maxTenor: number; maxThpPercent: number }> = {
  kasbon: { minServiceMonths: 3, maxTenor: 2, maxThpPercent: 50 },
  loan: { minServiceMonths: 12, maxTenor: 6, maxThpPercent: 200 },
};

const ruleOf = (loanType: string) => LOAN_RULES[loanType] ?? LOAN_RULES.kasbon;

/** Pilihan cicilan: kasbon 1/2 (default 2 = 50/50), Pinjaman Karyawan 1–6. */
export function loanTenorOptions(loanType: string): { value: string; label: string }[] {
  return Array.from({ length: ruleOf(loanType).maxTenor }, (_, i) => {
    const n = i + 1;
    return { value: String(n), label: loanType === "kasbon" && n === 2 ? "2 kali (50% / 50%)" : `${n} kali` };
  });
}

/** Tenor yang masih sah setelah jenis berganti; di luar batas → cicilan maksimum jenis itu. */
export function tenorForType(loanType: string, tenor: string): string {
  const n = Number(tenor);
  const max = ruleOf(loanType).maxTenor;
  return Number.isInteger(n) && n >= 1 && n <= max ? tenor : String(max);
}

/** Ringkasan syarat yang tampil di form pengajuan. */
export function loanTermsText(loanType: string): string {
  const r = ruleOf(loanType);
  const doc = loanType === "kasbon" ? "dokumen pendukung" : "jaminan (surat berharga)";
  return `Hanya PKWT/PKWTT dengan masa kerja minimal ${r.minServiceMonths} bulan (probation tidak dapat mengajukan) · maks ${r.maxThpPercent}% THP · ${r.maxTenor === 2 ? "1–2" : `1–${r.maxTenor}`} kali cicilan dipotong dari payroll · wajib lampirkan ${doc}`;
}

/**
 * Payroll pertama yang memotong pengajuan bertanggal date (YYYY-MM-DD, WIB):
 * tanggal 1–20 → bulan itu, 21–akhir → bulan berikutnya (periode 21–20).
 * Cermin Go domain.LoanPayrollPeriod; saat disetujui bisa mundur bila run
 * bulan itu sudah selesai.
 */
export function loanPayrollPeriodLabel(date: string): string {
  const [y, m, d] = date.slice(0, 10).split("-").map(Number);
  if (!y || !m || !d) return "";
  const next = d > 20;
  const month = next ? (m % 12) + 1 : m;
  const year = next && m === 12 ? y + 1 : y;
  return monthYearLabel(month, year);
}

/** Tanggal WIB (YYYY-MM-DD) dari timestamp ISO. */
export function wibDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-CA", { timeZone: "Asia/Jakarta" });
}

/** Bulan potong pertama: yang tersimpan saat disetujui, selain itu perkiraan dari tanggal pengajuan. */
export function loanDeductionLabel(row: {
  first_installment_month: number | null;
  first_installment_year: number | null;
  created_at?: string | null;
}): string {
  if (row.first_installment_month && row.first_installment_year) {
    return monthYearLabel(row.first_installment_month, row.first_installment_year);
  }
  return row.created_at ? `${loanPayrollPeriodLabel(wibDate(row.created_at))} (perkiraan)` : "—";
}

/** Lampiran pinjaman privat: pemohon, Manager approver, dan HR (EPIC-076 R9). */
export const loanAttachmentHref = (path: string) => privateFileHref("/api/hris/loans/attachment", path);

export const MAX_LOAN_ATTACHMENTS = 3;

/** Syarat jumlah & cicilan & lampiran sebelum dikirim (server memeriksa ulang semuanya). */
export function validateLoanRequest(form: {
  loan_type: string;
  principal_amount: string;
  tenor_months: string;
  attachments: string[];
  collateral_attachments: string[];
}): string | null {
  if (!Number(form.principal_amount) || !Number(form.tenor_months)) {
    return "Jumlah pinjaman dan tenor wajib diisi";
  }
  if (tenorForType(form.loan_type, form.tenor_months) !== form.tenor_months) {
    return form.loan_type === "kasbon" ? "Cicilan kasbon hanya 1 atau 2 kali" : "Cicilan Pinjaman Karyawan 1 sampai 6 kali";
  }
  if (form.loan_type === "kasbon" && form.attachments.length === 0) return "Lampirkan minimal 1 dokumen pendukung kasbon";
  if (form.loan_type !== "kasbon" && form.collateral_attachments.length === 0) {
    return "Lampirkan minimal 1 dokumen jaminan (surat berharga)";
  }
  return null;
}

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
  attachments: string[];
  collateral_attachments: string[];
}

export const EMPTY_LOAN_FORM: LoanForm = {
  employee_id: "",
  loan_type: "kasbon",
  principal_amount: "",
  interest_rate: "0",
  tenor_months: "2",
  purpose: "",
  attachments: [],
  collateral_attachments: [],
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
  return validateLoanRequest(form);
}

export function loanPayload(form: LoanForm) {
  return {
    employee_id: form.employee_id,
    loan_type: form.loan_type,
    principal_amount: Number(form.principal_amount),
    interest_rate: Number(form.interest_rate) || 0,
    tenor_months: Number(form.tenor_months),
    purpose: form.purpose || undefined,
    attachments: form.attachments,
    collateral_attachments: form.loan_type === "kasbon" ? [] : form.collateral_attachments,
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
