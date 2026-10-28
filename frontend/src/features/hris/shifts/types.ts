import type { ShiftRow as BaseShiftRow } from "@/lib/hris/shifts-repo";
import type { ShiftPayload } from "@/lib/hris/shifts-view";

export type { ShiftPayload };

/** Baris shift; `employees_using` hanya ada bila daftar diminta dengan `with_usage=true`. */
export type ShiftRow = BaseShiftRow & { employees_using?: number };

/** Simpan penuh dari form, atau ubah status aktif saja (Nonaktifkan / Aktifkan). */
export type ShiftSavePayload = ShiftPayload | { is_active: boolean };
