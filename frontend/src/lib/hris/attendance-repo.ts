import { z } from "zod";
import { ApiError } from "@/lib/api/auth";
import { query, queryOne } from "@/lib/db";
import { createServerPgClient } from "@/lib/pg/create-client";
import { savePrivateImage } from "@/lib/storage-private";
import {
  computeLateness,
  resolveShiftForDate,
  scheduledWindow,
  type EmployeeShiftRow,
} from "./shifts";
import type { AttendanceExportRecord } from "./attendance-report";
import type { WorkforceActor } from "./workforce-auth";
import { unwrap } from "./workforce-route";

/**
 * Absensi karyawan (hris.attendance): daftar, clock-in/out dengan selfie
 * wajib, koreksi HR, statistik bulanan, dan jadwal untuk kalender.
 * Non-HR selalu dibatasi ke record miliknya sendiri.
 */

/** Tanggal hari ini menurut WIB (jam kerja operasional). */
export function todayWib(now = Date.now()): string {
  return new Date(now + 7 * 3600_000).toISOString().split("T")[0];
}

const locationSchema = z.object({
  latitude: z.number(),
  longitude: z.number(),
  accuracy: z.number().optional(),
  address: z.string().optional(),
  ip_address: z.string().optional(),
});

// foto selfie (data URL) WAJIB utk clock-in & clock-out
const photoSchema = z
  .string()
  .regex(/^data:image\/(jpeg|png|webp);base64,/, "Foto selfie wajib disertakan");

export const clockInSchema = z.object({
  employee_id: z.string().uuid().optional(),
  date: z.string().optional(),
  photo: photoSchema,
  clock_in_location: locationSchema.optional(),
  notes: z.string().optional(),
});

export const clockOutSchema = z.object({
  attendance_id: z.string().uuid(),
  photo: photoSchema,
  clock_out_location: locationSchema.optional(),
  notes: z.string().optional(),
});

const MAX_PHOTO_BYTES = 2 * 1024 * 1024;

/** Simpan selfie (data URL) ke storage private; return path relatif. */
async function saveSelfie(dataUrl: string, employeeId: string): Promise<string> {
  const match = /^data:(image\/(?:jpeg|png|webp));base64,(.+)$/.exec(dataUrl);
  if (!match) throw ApiError.badRequest("Format foto tidak valid");
  const buffer = Buffer.from(match[2], "base64");
  if (buffer.length > MAX_PHOTO_BYTES) throw ApiError.badRequest("Ukuran foto maksimal 2 MB");
  const saved = await savePrivateImage(buffer, match[1], `attendance/${employeeId}`);
  if (!saved.path) throw ApiError.badRequest(saved.error ?? "Foto selfie tidak valid");
  return saved.path;
}

interface ScheduleJoinRow extends EmployeeShiftRow {
  name: string | null;
  start_time: string | null;
  end_time: string | null;
  late_tolerance_minutes: number | null;
  is_overnight: boolean | null;
}

/** Snapshot shift + keterlambatan utk sebuah clock-in. */
async function shiftSnapshot(employeeId: string, dateIso: string, clockIn: Date) {
  const rows = await query<ScheduleJoinRow>(
    `SELECT es.day_of_week, es.shift_id,
            es.effective_from::text, es.effective_to::text,
            s.name, s.start_time::text, s.end_time::text,
            s.late_tolerance_minutes, s.is_overnight
     FROM hris.employee_shifts es
     LEFT JOIN hris.shifts s ON s.id = es.shift_id
     WHERE es.employee_id = $1`,
    [employeeId]
  );
  const active = resolveShiftForDate(rows, dateIso);
  const detail = active ? rows.find((row) => row.shift_id === active.shift_id && row.name) : null;
  if (!active || !detail?.start_time || !detail.end_time) {
    // tanpa jadwal / libur: absen tetap tercatat, tanpa penilaian terlambat
    return { shift_id: null, scheduled_start: null, scheduled_end: null, is_late: false, late_minutes: 0 };
  }
  const shift = {
    start_time: detail.start_time,
    end_time: detail.end_time,
    is_overnight: detail.is_overnight ?? false,
    late_tolerance_minutes: detail.late_tolerance_minutes ?? 0,
  };
  const window = scheduledWindow(dateIso, shift);
  return {
    shift_id: active.shift_id,
    scheduled_start: window.start.toISOString(),
    scheduled_end: window.end.toISOString(),
    ...computeLateness(clockIn, dateIso, shift),
  };
}

export interface AttendanceListParams {
  employeeId: string | null;
  date: string | null;
  startDate: string | null;
  endDate: string | null;
  status: string | null;
  lateOnly: boolean;
  page: number;
  limit: number;
}

const MAX_LIST_LIMIT = 1000;

/**
 * Filter daftar absensi. employee_id=me → record sendiri; non-HR selalu
 * dipaksa ke record sendiri. Return null bila "me" tapi akun tak tertaut
 * karyawan (daftar kosong).
 */
export function resolveAttendanceListParams(
  params: URLSearchParams,
  actor: WorkforceActor
): AttendanceListParams | null {
  let employeeId = params.get("employee_id");
  if (employeeId === "me") {
    if (!actor.employeeId) return null;
    employeeId = actor.employeeId;
  }
  if (!actor.isHr) {
    if (!actor.employeeId) throw ApiError.forbidden("Akun ini tidak terhubung ke data karyawan");
    employeeId = actor.employeeId;
  }
  const page = parseInt(params.get("page") || "1") || 1;
  const limit = parseInt(params.get("limit") || "20") || 20;
  return {
    employeeId,
    date: params.get("date"),
    startDate: params.get("start_date"),
    endDate: params.get("end_date"),
    status: params.get("status"),
    // kolom status selalu 'present'; rekap "Terlambat" lewat is_late
    lateOnly: params.get("is_late") === "true",
    page: Math.max(1, page),
    limit: Math.min(MAX_LIST_LIMIT, Math.max(1, limit)),
  };
}

export async function listAttendance(params: AttendanceListParams) {
  const db = await createServerPgClient();
  let builder = db.from("attendance").select(
    `id, employee_id, date, clock_in, clock_out,
      clock_in_location, clock_out_location, clock_in_photo_url, clock_out_photo_url,
      shift_id, scheduled_start, scheduled_end, work_hours, break_minutes,
      status, is_late, late_minutes, notes, created_at, updated_at,
      shift:shifts(id, name),
      employee:employees!attendance_employee_id_fkey(
        id, full_name, nip, photo_url, department_id, job_title_id
      )`,
    { count: "exact" }
  );
  if (params.employeeId) builder = builder.eq("employee_id", params.employeeId);
  if (params.date) builder = builder.eq("date", params.date);
  if (params.startDate && params.endDate) {
    builder = builder.gte("date", params.startDate).lte("date", params.endDate);
  }
  if (params.status) builder = builder.eq("status", params.status);
  if (params.lateOnly) builder = builder.eq("is_late", true);

  const from = (params.page - 1) * params.limit;
  const { data, error, count } = await builder
    .range(from, from + params.limit - 1)
    .order("date", { ascending: false });
  unwrap({ data, error });
  return { rows: data ?? [], total: count || 0 };
}

export type ClockInResult =
  | { status: "created"; data: unknown }
  | { status: "already"; attendanceId: string };

export async function clockIn(
  actor: WorkforceActor,
  input: z.infer<typeof clockInSchema>
): Promise<ClockInResult> {
  // non-HR hanya boleh clock-in untuk dirinya sendiri
  const employeeId = actor.isHr && input.employee_id ? input.employee_id : actor.employeeId;
  if (!employeeId) throw ApiError.notFound("Akun ini tidak terhubung ke data karyawan");

  const db = await createServerPgClient();
  const date = input.date || todayWib();
  const { data: existing } = await db
    .from("attendance")
    .select("id")
    .eq("employee_id", employeeId)
    .eq("date", date)
    .single();
  if (existing) return { status: "already", attendanceId: existing.id };

  const photoPath = await saveSelfie(input.photo, employeeId);
  const clockInAt = new Date();
  const snapshot = await shiftSnapshot(employeeId, date, clockInAt);

  const data = unwrap(
    await db
      .from("attendance")
      .insert({
        employee_id: employeeId,
        date,
        clock_in: clockInAt.toISOString(),
        clock_in_location: input.clock_in_location || null,
        clock_in_photo_url: photoPath,
        ...snapshot,
        notes: input.notes || null,
        status: "present",
      })
      .select()
      .single()
  );
  return { status: "created", data };
}

export async function clockOut(actor: WorkforceActor, input: z.infer<typeof clockOutSchema>) {
  const db = await createServerPgClient();
  const { data: attendance, error } = await db
    .from("attendance")
    .select("*")
    .eq("id", input.attendance_id)
    .single();
  if (error || !attendance) throw ApiError.notFound("Attendance record not found");
  // non-HR hanya boleh clock-out absensinya sendiri
  if (!actor.isHr && attendance.employee_id !== actor.employeeId) {
    throw ApiError.forbidden("Tidak boleh mengubah absensi karyawan lain");
  }
  if (attendance.clock_out) throw ApiError.badRequest("Sudah clock-out untuk absensi ini");

  const photoPath = await saveSelfie(input.photo, attendance.employee_id);
  // work_hours dihitung trigger database
  return unwrap(
    await db
      .from("attendance")
      .update({
        clock_out: new Date().toISOString(),
        clock_out_location: input.clock_out_location || null,
        clock_out_photo_url: photoPath,
        notes: input.notes ? `${attendance.notes || ""}\n${input.notes}`.trim() : attendance.notes,
      })
      .eq("id", input.attendance_id)
      .select()
      .single()
  );
}

/** Detail absensi; non-HR hanya record miliknya (404 agar id lain tidak bocor). */
export async function getAttendance(id: string, actor: WorkforceActor) {
  const db = await createServerPgClient();
  const { data, error } = await db
    .from("attendance")
    .select(
      `*,
        employee:employees!inner(
          id, full_name, nip, photo_url, email, department_id, job_title_id
        )`
    )
    .eq("id", id)
    .single();
  if (error || !data) throw ApiError.notFound("Attendance not found");
  if (!actor.isHr && data.employee_id !== actor.employeeId) {
    throw ApiError.notFound("Attendance not found");
  }
  return data;
}

export const attendanceUpdateSchema = z.object({
  status: z.string().max(30).optional(),
  notes: z.string().max(5000).nullable().optional(),
  validation_notes: z.string().max(5000).nullable().optional(),
  validated: z.boolean().optional(),
});

export async function updateAttendance(
  id: string,
  body: z.infer<typeof attendanceUpdateSchema>,
  actor: WorkforceActor
) {
  const { validated, ...fields } = body;
  const update: Record<string, unknown> = { ...fields };
  if (validated === true) {
    // FK validated_by → hris.employees(id): pakai record karyawan validator;
    // null bila akun tidak tertaut karyawan (mis. super admin)
    update.validated_by = actor.employeeId;
    update.validated_at = new Date().toISOString();
  }
  const db = await createServerPgClient();
  return unwrap(await db.from("attendance").update(update).eq("id", id).select().single());
}

export async function deleteAttendance(id: string) {
  const db = await createServerPgClient();
  const { error } = await db.from("attendance").delete().eq("id", id);
  unwrap({ data: null, error });
}

/** Bulan/tahun statistik; default bulan berjalan WIB. */
export function resolveStatsPeriod(params: URLSearchParams, now = Date.now()) {
  const wib = new Date(now + 7 * 3600_000);
  return {
    month: Number(params.get("month")) || wib.getUTCMonth() + 1,
    year: Number(params.get("year")) || wib.getUTCFullYear(),
  };
}

/**
 * Statistik halaman rekap: hadir hari ini, terlambat bulan ini, di luar
 * jadwal, rata-rata jam kerja. Non-HR dibatasi datanya sendiri.
 */
export async function loadAttendanceStats(actor: WorkforceActor, month: number, year: number) {
  const employeeFilter = actor.isHr ? null : actor.employeeId;
  if (!actor.isHr && !employeeFilter) {
    throw ApiError.forbidden("Akun ini tidak terhubung ke data karyawan");
  }

  const [monthly, todayRow, activeEmployees] = await Promise.all([
    queryOne<{
      total_records: string;
      late_count: string;
      off_schedule: string;
      avg_work_hours: string | null;
    }>(
      `SELECT count(*) AS total_records,
              count(*) FILTER (WHERE is_late) AS late_count,
              count(*) FILTER (WHERE shift_id IS NULL) AS off_schedule,
              round(avg(work_hours), 1) AS avg_work_hours
       FROM hris.attendance
       WHERE date_part('month', date) = $1 AND date_part('year', date) = $2
         AND ($3::uuid IS NULL OR employee_id = $3)`,
      [month, year, employeeFilter]
    ),
    queryOne<{ present_today: string; late_today: string }>(
      `SELECT count(*) AS present_today,
              count(*) FILTER (WHERE is_late) AS late_today
       FROM hris.attendance
       WHERE date = (now() + interval '7 hours')::date
         AND ($1::uuid IS NULL OR employee_id = $1)`,
      [employeeFilter]
    ),
    actor.isHr
      ? queryOne<{ count: string }>(
          `SELECT count(*) FROM hris.employees e
           WHERE e.is_active
             AND NOT EXISTS (
               SELECT 1 FROM configuration.users u
               WHERE u.id = e.user_id AND u.role = 'super_admin')`
        )
      : Promise.resolve(null),
  ]);

  return {
    month,
    year,
    present_today: Number(todayRow?.present_today ?? 0),
    late_today: Number(todayRow?.late_today ?? 0),
    active_employees: activeEmployees ? Number(activeEmployees.count) : null,
    month_records: Number(monthly?.total_records ?? 0),
    month_late: Number(monthly?.late_count ?? 0),
    month_off_schedule: Number(monthly?.off_schedule ?? 0),
    avg_work_hours: monthly?.avg_work_hours ? Number(monthly.avg_work_hours) : null,
  };
}

/** Pola jadwal (baris employee_shifts + detail shift) untuk kalender absensi. */
export async function listScheduleRows(employeeId: string) {
  return query(
    `SELECT es.day_of_week, es.shift_id,
            es.effective_from::text, es.effective_to::text,
            s.name AS shift_name, s.start_time::text, s.end_time::text,
            s.is_overnight
     FROM hris.employee_shifts es
     LEFT JOIN hris.shifts s ON s.id = es.shift_id
     WHERE es.employee_id = $1
     ORDER BY es.effective_from DESC, es.day_of_week ASC`,
    [employeeId]
  );
}

export interface AttendanceExportFilters {
  employeeId: string | null;
  startDate: string | null;
  endDate: string | null;
  status: string | null;
}

/** Baris ekspor rekap (CSV/Excel/PDF), terbaru dulu. */
export async function listAttendanceForExport(
  filters: AttendanceExportFilters
): Promise<AttendanceExportRecord[]> {
  const db = await createServerPgClient();
  let builder = db.from("attendance").select(
    `id, employee_id, date, clock_in, clock_out,
      clock_in_location, clock_out_location, work_hours, break_minutes,
      status, is_late, late_minutes, is_short_hours, min_work_hours, notes,
      clock_in_photo_url, clock_out_photo_url, created_at,
      employee:employees!attendance_employee_id_fkey(
        full_name, nip,
        department:departments(name),
        job_title:positions(title)
      )`
  );
  if (filters.employeeId) builder = builder.eq("employee_id", filters.employeeId);
  if (filters.startDate && filters.endDate) {
    builder = builder.gte("date", filters.startDate).lte("date", filters.endDate);
  }
  if (filters.status) builder = builder.eq("status", filters.status);

  return (unwrap(await builder.order("date", { ascending: false })) ?? []) as AttendanceExportRecord[];
}

/** Nama perusahaan pertama untuk kop laporan. */
export async function loadCompanyName(): Promise<string> {
  const company = await queryOne<{ name: string }>(
    "SELECT name FROM configuration.companies ORDER BY created_at LIMIT 1"
  );
  return company?.name ?? "Arkiv OS";
}
