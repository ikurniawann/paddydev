/**
 * Definisi kolom & konversi form Pengaturan Payroll (payroll_settings +
 * payroll_tax_config). Kolom kosong = pakai default server, tidak dikirim.
 */

export interface FieldDef {
  key: string;
  label: string;
  suffix?: string;
}

export const BPJS_TK_FIELDS: FieldDef[] = [
  { key: "bpjs_tk_jht_employee", label: "JHT Karyawan", suffix: "%" },
  { key: "bpjs_tk_jht_employer", label: "JHT Perusahaan", suffix: "%" },
  { key: "bpjs_tk_jp_employee", label: "JP Karyawan", suffix: "%" },
  { key: "bpjs_tk_jp_employer", label: "JP Perusahaan", suffix: "%" },
  { key: "bpjs_tk_jkk", label: "JKK (Perusahaan)", suffix: "%" },
  { key: "bpjs_tk_jkm", label: "JKM (Perusahaan)", suffix: "%" },
];

export const BPJS_KES_FIELDS: FieldDef[] = [
  { key: "bpjs_kes_employee", label: "Karyawan", suffix: "%" },
  { key: "bpjs_kes_employer", label: "Perusahaan", suffix: "%" },
  { key: "bpjs_kes_max_upah", label: "Batas Upah Maks", suffix: "Rp" },
];

export const TAPERA_FIELDS: FieldDef[] = [
  { key: "tapera_employee", label: "Karyawan", suffix: "%" },
  { key: "tapera_employer", label: "Perusahaan", suffix: "%" },
];

export const LAINNYA_FIELDS: FieldDef[] = [
  { key: "overtime_multiplier", label: "Pengali Lembur (hari kerja)", suffix: "×" },
  // EPIC-036 Fase F — PP 35/2021 membedakan tarif lembur hari libur resmi.
  { key: "overtime_multiplier_holiday", label: "Pengali Lembur (hari libur)", suffix: "×" },
  { key: "overtime_hourly_divisor", label: "Pembagi Upah/Jam Lembur", suffix: "std 173" },
  { key: "payroll_day", label: "Tanggal Gajian", suffix: "tgl" },
  { key: "thr_eligible_months", label: "Min. Bulan Kerja THR", suffix: "bln" },
  // EPIC-055 — rule Odoo OT_PROD: tarif × (jam biasa + 2 × jam istirahat).
  { key: "overtime_production_rate", label: "Tarif Lembur Tim Produksi", suffix: "Rp/jam" },
];

export const LOAN_FIELDS: FieldDef[] = [
  { key: "loan_max_installment_percent", label: "Cicilan Maks dari Gaji Pokok", suffix: "%" },
  { key: "loan_max_active_per_employee", label: "Maks Pinjaman Aktif/Karyawan", suffix: "buah" },
];

export const LATE_MODE_OPTIONS = [
  { value: "off", label: "Nonaktif (tanpa potongan)" },
  { value: "per_minute", label: "Per menit keterlambatan" },
  { value: "flat", label: "Flat per kejadian terlambat" },
  { value: "tier", label: "Bertingkat per jumlah telat/bulan (rule Odoo)" },
];

/** Tier potongan telat (payroll_settings.late_tier_rules). max kosong = tanpa batas atas. */
export interface LateTierRule {
  min: number;
  max?: number;
  days: number;
  basis: "thp" | "base";
}

export const DEFAULT_LATE_TIERS: LateTierRule[] = [
  { min: 4, max: 6, days: 1, basis: "thp" },
  { min: 7, max: 9, days: 2, basis: "base" },
  { min: 10, days: 3, basis: "base" },
];

/** Baca late_tier_rules dari form state (JSON); rusak/kosong = default rule Odoo. */
export function parseLateTiers(value: string | undefined): LateTierRule[] {
  if (!value) return DEFAULT_LATE_TIERS;
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) && parsed.length > 0 ? (parsed as LateTierRule[]) : DEFAULT_LATE_TIERS;
  } catch {
    return DEFAULT_LATE_TIERS;
  }
}

export const PPH21_METHOD_OPTIONS = [
  { value: "annualized", label: "Disetahunkan (progresif Pasal 17 tiap bulan)" },
  { value: "ter", label: "TER bulanan PP 58/2023 + koreksi Desember" },
];

export const PPH21_BPJS_BASIS_OPTIONS = [
  { value: "both", label: "Iuran BPJS perusahaan + karyawan (seperti Odoo)" },
  { value: "employer", label: "Iuran BPJS perusahaan saja (standar DJP)" },
  { value: "employee", label: "Iuran BPJS karyawan saja" },
];

/** Saklar payroll (boolean). */
export const TOGGLE_FIELDS: { key: string; label: string; help: string }[] = [
  {
    key: "overtime_requires_clockout",
    label: "Lembur wajib ada clock-out",
    help: "Matikan agar lembur approved tetap dibayar walau absensi hari itu tanpa clock-out.",
  },
  {
    key: "overtime_rest_from_schedule",
    label: "Hari tanpa shift di jadwal = hari istirahat",
    help: "Lembur pada hari tanpa shift di jadwal karyawan dibayar tarif libur (selain hari yang ditandai hari istirahat).",
  },
  {
    key: "personal_leave_deduction",
    label: "Potong gaji untuk Izin Pribadi",
    help: "1 hari THP (gaji pokok + transport + makan + jabatan) ÷ hari kerja terjadwal × hari izin. Tunjangan per hari tidak dipotong dua kali; intern tidak dipotong.",
  },
  {
    key: "sick_leave_deduction",
    label: "Potong transport & makan saat Sakit / Cuti Melahirkan",
    help: "Hanya tunjangan flat: (transport + makan) ÷ pembagi hari × hari sakit & melahirkan. Tunjangan per hari sudah tidak dibayar saat tidak hadir.",
  },
];

export const PTKP_FIELDS: FieldDef[] = [
  { key: "ptkp_tk_0", label: "TK/0", suffix: "Rp" },
  { key: "ptkp_tk_1", label: "TK/1", suffix: "Rp" },
  { key: "ptkp_tk_2", label: "TK/2", suffix: "Rp" },
  { key: "ptkp_tk_3", label: "TK/3", suffix: "Rp" },
  { key: "ptkp_k_0", label: "K/0", suffix: "Rp" },
  { key: "ptkp_k_1", label: "K/1", suffix: "Rp" },
  { key: "ptkp_k_2", label: "K/2", suffix: "Rp" },
  { key: "ptkp_k_3", label: "K/3", suffix: "Rp" },
];

export const BRACKET_FIELDS: FieldDef[] = [
  { key: "bracket_1_limit", label: "Batas Lapisan 1", suffix: "Rp" },
  { key: "bracket_1_rate", label: "Tarif Lapisan 1", suffix: "%" },
  { key: "bracket_2_limit", label: "Batas Lapisan 2", suffix: "Rp" },
  { key: "bracket_2_rate", label: "Tarif Lapisan 2", suffix: "%" },
  { key: "bracket_3_limit", label: "Batas Lapisan 3", suffix: "Rp" },
  { key: "bracket_3_rate", label: "Tarif Lapisan 3", suffix: "%" },
  { key: "bracket_4_limit", label: "Batas Lapisan 4", suffix: "Rp" },
  { key: "bracket_4_rate", label: "Tarif Lapisan 4", suffix: "%" },
  { key: "bracket_5_rate", label: "Tarif Lapisan 5", suffix: "%" },
  { key: "jabatan_expense_percentage", label: "Biaya Jabatan", suffix: "%" },
  { key: "jabatan_expense_max", label: "Biaya Jabatan Maks/Thn", suffix: "Rp" },
];

export const SETTINGS_KEYS = [
  ...[
    ...BPJS_TK_FIELDS,
    ...BPJS_KES_FIELDS,
    ...TAPERA_FIELDS,
    ...LAINNYA_FIELDS,
    ...LOAN_FIELDS,
  ].map((f) => f.key),
  "late_deduction_mode",
  "late_deduction_amount",
  "late_min_minutes",
  "late_tier_rules",
  "pph21_method",
  "pph21_bpjs_basis",
  ...TOGGLE_FIELDS.map((f) => f.key),
];

/** Kolom pengaturan bertipe teks — dikirim apa adanya, bukan angka. */
const STRING_SETTING_KEYS = new Set(["late_deduction_mode", "pph21_method", "pph21_bpjs_basis"]);
/** Kolom boolean ("true" / "false" di form state). */
const BOOLEAN_SETTING_KEYS = new Set(TOGGLE_FIELDS.map((f) => f.key));
/** Kolom JSON (teks JSON di form state). */
const JSON_SETTING_KEYS = new Set(["late_tier_rules"]);

export const TAX_KEYS = [...PTKP_FIELDS, ...BRACKET_FIELDS].map((f) => f.key);

export type FormState = Record<string, string>;

export function rowToFormState(
  row: Record<string, unknown> | null,
  keys: string[]
): FormState {
  const state: FormState = {};
  for (const key of keys) {
    const value = row?.[key];
    state[key] =
      value === null || value === undefined
        ? ""
        : typeof value === "object"
          ? JSON.stringify(value)
          : String(value);
  }
  return state;
}

export function formStateToPayload(
  state: FormState
): Record<string, number | string | boolean | LateTierRule[]> {
  const payload: Record<string, number | string | boolean | LateTierRule[]> = {};
  for (const [key, value] of Object.entries(state)) {
    if (value === "") continue;
    if (BOOLEAN_SETTING_KEYS.has(key)) {
      payload[key] = value === "true";
      continue;
    }
    if (JSON_SETTING_KEYS.has(key)) {
      payload[key] = parseLateTiers(value);
      continue;
    }
    if (STRING_SETTING_KEYS.has(key)) {
      payload[key] = value;
      continue;
    }
    const n = Number(value);
    if (Number.isFinite(n)) payload[key] = n;
  }
  return payload;
}
