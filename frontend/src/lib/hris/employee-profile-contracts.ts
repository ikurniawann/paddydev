import { formatRupiah } from "@/lib/format";

/**
 * Form kontrak di tab "Kontrak" detail karyawan: konversi baris kontrak ↔
 * nilai form, dan payload yang dikirim ke API kontrak.
 */

export type ContractType = "pkwt" | "pkwtt";

export interface ContractFormValues {
  contract_type: ContractType;
  start_date: string;
  end_date: string;
  probation_end_date: string;
  work_location: string;
  base_salary: string;
  notes: string;
}

export const EMPTY_CONTRACT_FORM: ContractFormValues = {
  contract_type: "pkwt",
  start_date: "",
  end_date: "",
  probation_end_date: "",
  work_location: "",
  base_salary: "",
  notes: "",
};

export interface ContractAdminFormValues {
  signed_at: string;
  kemnaker_registered_at: string;
  compensation_paid_at: string;
}

interface ContractRowFields {
  contract_type: ContractType;
  start_date: string;
  end_date: string | null;
  probation_end_date: string | null;
  work_location: string | null;
  /** Tidak dikirim API untuk akun tanpa akses Penggajian. */
  base_salary?: string | null;
  notes: string | null;
  signed_at: string | null;
  kemnaker_registered_at: string | null;
  compensation_paid_at: string | null;
}

/** Nilai untuk <input type="date"> (YYYY-MM-DD, kalender WIB). */
export function toDateInput(value: string | null | undefined): string {
  if (!value) return "";
  // tanggal kalender dipakai apa adanya: lewat new Date() bisa bergeser sehari
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleDateString("en-CA", { timeZone: "Asia/Jakarta" });
}

export function contractFormFromRow(row: ContractRowFields): ContractFormValues {
  return {
    contract_type: row.contract_type,
    start_date: toDateInput(row.start_date),
    end_date: toDateInput(row.end_date),
    probation_end_date: toDateInput(row.probation_end_date),
    work_location: row.work_location ?? "",
    base_salary: row.base_salary ? String(Number(row.base_salary)) : "",
    notes: row.notes ?? "",
  };
}

/** PKWT punya tanggal berakhir tanpa probation; PKWTT sebaliknya. String kosong → null. */
export function contractFormPayload(form: ContractFormValues) {
  const isPkwt = form.contract_type === "pkwt";
  return {
    start_date: form.start_date,
    end_date: isPkwt ? form.end_date || null : null,
    probation_end_date: isPkwt ? null : form.probation_end_date || null,
    work_location: form.work_location || null,
    base_salary: form.base_salary ? Number(form.base_salary) : null,
    notes: form.notes || null,
  };
}

export function contractAdminFormFromRow(row: ContractRowFields): ContractAdminFormValues {
  return {
    signed_at: toDateInput(row.signed_at),
    kemnaker_registered_at: toDateInput(row.kemnaker_registered_at),
    compensation_paid_at: toDateInput(row.compensation_paid_at),
  };
}

/** String kosong mengosongkan tanggal di server (null). */
export function contractAdminPayload(form: ContractAdminFormValues) {
  return {
    signed_at: form.signed_at || null,
    kemnaker_registered_at: form.kemnaker_registered_at || null,
    compensation_paid_at: form.compensation_paid_at || null,
  };
}

/** Pesan toast aksi kontrak; akhiri/putus PKWT menyertakan uang kompensasi. */
export function contractActionMessage(res: {
  message: string;
  compensation_amount?: number | null;
}): string {
  const amount = res.compensation_amount;
  const compensation =
    amount != null && amount > 0 ? ` — uang kompensasi ${formatRupiah(amount)}` : "";
  return `${res.message}${compensation}`;
}
