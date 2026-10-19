/**
 * Master shift kerja (hris.shifts). Semua karyawan ber-akun boleh membaca
 * (dropdown jadwal tim oleh atasan); tulis khusus HR kepegawaian.
 */

import { z } from "zod";
import { ApiError } from "@/lib/api/auth";
import { query, queryOne } from "@/lib/db";

const TIME_RE = /^([01]\d|2[0-3]):[0-5]\d(:[0-5]\d)?$/;

export interface ShiftRow {
  id: string;
  code: string;
  is_default?: boolean;
  name: string;
  start_time: string;
  end_time: string;
  break_minutes: number;
  late_tolerance_minutes: number;
  is_overnight: boolean;
  is_active: boolean;
  sort_order: number;
}

const SHIFT_COLUMNS = `id, name, start_time, end_time, break_minutes,
  late_tolerance_minutes, is_overnight, is_active, sort_order`;

const NAME_REQUIRED = "Nama shift wajib diisi";
const START_INVALID = "Jam mulai tidak valid (HH:MM)";
const END_INVALID = "Jam selesai tidak valid (HH:MM)";
const TOLERANCE_INVALID = "Toleransi terlambat harus angka ≥ 0";

export const createShiftSchema = z
  .object({
    name: z.string({ error: NAME_REQUIRED }).trim().min(1, NAME_REQUIRED),
    start_time: z.string({ error: START_INVALID }).regex(TIME_RE, START_INVALID),
    end_time: z.string({ error: END_INVALID }).regex(TIME_RE, END_INVALID),
    break_minutes: z.number().int().min(0).optional(),
    late_tolerance_minutes: z
      .number({ error: TOLERANCE_INVALID })
      .int(TOLERANCE_INVALID)
      .min(0, TOLERANCE_INVALID)
      .optional(),
    is_overnight: z.boolean().optional(),
    sort_order: z.number().int().optional(),
  })
  .refine((s) => s.is_overnight || s.end_time > s.start_time, {
    message:
      "Jam selesai harus setelah jam mulai — atau tandai sebagai shift malam (lewat tengah malam)",
  });

export const updateShiftSchema = z.object({
  name: z.string().optional(),
  start_time: z.string().regex(TIME_RE, "Jam mulai tidak valid").optional(),
  end_time: z.string().regex(TIME_RE, "Jam selesai tidak valid").optional(),
  break_minutes: z.number().int().min(0).optional(),
  late_tolerance_minutes: z.number().int().min(0).optional(),
  is_overnight: z.boolean().optional(),
  is_active: z.boolean().optional(),
  sort_order: z.number().int().optional(),
});

export function listShifts(): Promise<ShiftRow[]> {
  return query<ShiftRow>(`SELECT ${SHIFT_COLUMNS} FROM hris.shifts ORDER BY sort_order ASC, name ASC`);
}

export async function createShift(input: z.infer<typeof createShiftSchema>): Promise<ShiftRow | null> {
  return queryOne<ShiftRow>(
    `INSERT INTO hris.shifts
       (name, start_time, end_time, break_minutes, late_tolerance_minutes, is_overnight, sort_order)
     VALUES ($1,$2,$3,$4,$5,$6,$7)
     RETURNING ${SHIFT_COLUMNS}`,
    [
      input.name,
      input.start_time,
      input.end_time,
      input.break_minutes ?? 60,
      input.late_tolerance_minutes ?? 10,
      input.is_overnight ?? false,
      input.sort_order ?? 0,
    ]
  );
}

export async function updateShift(id: string, input: z.infer<typeof updateShiftSchema>): Promise<void> {
  const updated = await queryOne(
    `UPDATE hris.shifts SET
       name                   = COALESCE($2, name),
       start_time             = COALESCE($3::time, start_time),
       end_time               = COALESCE($4::time, end_time),
       break_minutes          = COALESCE($5, break_minutes),
       late_tolerance_minutes = COALESCE($6, late_tolerance_minutes),
       is_overnight           = COALESCE($7, is_overnight),
       is_active              = COALESCE($8, is_active),
       sort_order             = COALESCE($9, sort_order)
     WHERE id = $1 RETURNING id, name`,
    [
      id,
      input.name?.trim() ?? null,
      input.start_time ?? null,
      input.end_time ?? null,
      input.break_minutes ?? null,
      input.late_tolerance_minutes ?? null,
      input.is_overnight ?? null,
      input.is_active ?? null,
      input.sort_order ?? null,
    ]
  );
  if (!updated) throw ApiError.notFound("Shift tidak ditemukan");
}

/** Hapus bila belum pernah dipakai; bila dirujuk jadwal/absensi, nonaktifkan. */
export async function deleteShift(id: string): Promise<string> {
  const used = await queryOne<{ used: boolean }>(
    `SELECT EXISTS (
       SELECT 1 FROM hris.employee_shifts WHERE shift_id = $1
       UNION ALL
       SELECT 1 FROM hris.attendance WHERE shift_id = $1
     ) AS used`,
    [id]
  );
  if (used?.used) {
    await queryOne(`UPDATE hris.shifts SET is_active = false WHERE id = $1 RETURNING id`, [id]);
    return "Shift sudah dipakai jadwal/absensi — dinonaktifkan (tidak dihapus)";
  }
  const deleted = await queryOne<{ id: string }>(
    `DELETE FROM hris.shifts WHERE id = $1 RETURNING id`,
    [id]
  );
  if (!deleted) throw ApiError.notFound("Shift tidak ditemukan");
  return "Shift dihapus";
}
