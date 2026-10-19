/**
 * Logika murni form master shift (halaman HRIS Shift Kerja) dan pemotong jam
 * kolom `time` Postgres, dipakai juga daftar lembur.
 */

/** "08:00:00" → "08:00"; kosong → fallback. */
export function hhmm(time: string | null | undefined, fallback = "—"): string {
  return time ? time.slice(0, 5) : fallback;
}

interface ShiftForm {
  code: string;
  name: string;
  start_time: string;
  end_time: string;
  break_minutes: string;
  late_tolerance_minutes: string;
  is_overnight: boolean;
}

export interface ShiftPayload {
  code: string;
  name: string;
  start_time: string;
  end_time: string;
  break_minutes: number;
  late_tolerance_minutes: number;
  is_overnight: boolean;
}

export const EMPTY_SHIFT_FORM: ShiftForm = {
  code: "",
  name: "",
  start_time: "08:00",
  end_time: "16:00",
  break_minutes: "60",
  late_tolerance_minutes: "10",
  is_overnight: false,
};

export function shiftFormFrom(shift: {
  code?: string | null;
  name: string;
  start_time: string;
  end_time: string;
  break_minutes: number;
  late_tolerance_minutes: number;
  is_overnight: boolean;
}): ShiftForm {
  return {
    code: shift.code ?? "",
    name: shift.name,
    start_time: hhmm(shift.start_time),
    end_time: hhmm(shift.end_time),
    break_minutes: String(shift.break_minutes),
    late_tolerance_minutes: String(shift.late_tolerance_minutes),
    is_overnight: shift.is_overnight,
  };
}

export function shiftPayload(form: ShiftForm): ShiftPayload {
  return {
    code: form.code.trim().toUpperCase(),
    name: form.name.trim(),
    start_time: form.start_time,
    end_time: form.end_time,
    break_minutes: Number(form.break_minutes) || 0,
    late_tolerance_minutes: Number(form.late_tolerance_minutes) || 0,
    is_overnight: form.is_overnight,
  };
}

/** Kode shift dipakai di file upload Jadwal Kerja: 1–12 huruf/angka, tanpa spasi. */
export function shiftCodeError(code: string): string | null {
  return /^[A-Z0-9]{1,12}$/.test(code.trim().toUpperCase()) ? null : "Kode 1–12 huruf/angka tanpa spasi";
}
