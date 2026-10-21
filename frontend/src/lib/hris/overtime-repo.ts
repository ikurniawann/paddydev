/**
 * Pengajuan lembur (EPIC-008 Fase B). Karyawan mengajukan untuk dirinya
 * (source 'employee', diputuskan HRD/atasan); HRD menugaskan karyawan lain
 * (source 'company', dikonfirmasi karyawan ybs). Aturan otorisasi keputusan
 * ada di overtime-rules.ts.
 */

import { z } from "zod";
import { ApiError } from "@/lib/api/auth";
import { canonicalUuid } from "@/lib/api/canonical-uuid";
import { recordAuditAfterCommit } from "@/lib/audit";
import { overtimeHoursFromTimes, periodBounds } from "@/lib/payroll/period";
import { createServerPgClient } from "@/lib/pg/create-client";
import { auditDateText, decisionActorName } from "./decision-audit";
import { canDecideOvertime, decidedAs, OVERTIME_TOO_LATE, overtimeStartsInPast } from "./overtime-rules";
import type { WorkforceActor } from "./workforce-auth";
import { DATE_RE, unwrap, UUID_RE } from "./workforce-route";

const TIME_RE = /^([01]\d|2[0-3]):[0-5]\d$/;
const MAX_OVERTIME_HOURS = 12;
const DUPLICATE = "Sudah ada pengajuan lembur pending/approved di tanggal tersebut";

const OVERTIME_SELECT = `
  *,
  employee:employees!employee_id (
    id, full_name, nip, reporting_to,
    department:departments ( name )
  ),
  requester:employees!requested_by ( id, full_name ),
  decider:employees!decided_by ( id, full_name )
`;

export const overtimeListQuerySchema = z.object({
  status: z.string().optional(),
  month: z.string().optional(),
  year: z.string().optional(),
  employee_id: z.string().optional(),
  scope: z.string().optional(),
});

export const overtimeCreateSchema = z.object({
  // default: diri sendiri
  employee_id: z.union([z.literal("me"), z.string().regex(UUID_RE)]).optional(),
  date: z.string().regex(DATE_RE),
  start_time: z.string().regex(TIME_RE),
  end_time: z.string().regex(TIME_RE),
  reason: z.string().trim().min(5, "Alasan minimal 5 karakter"),
});

export const overtimeDecideSchema = z.object({
  overtime_id: z.string().uuid(),
  action: z.enum(["approve", "reject", "cancel"]),
  rejection_reason: z.string().optional(),
});

/** Filter bulan hanya bila bulan 1–12 dan tahun > 2000; selain itu diabaikan. */
function monthFilter(monthParam: string | undefined, yearParam: string | undefined) {
  const month = Number(monthParam);
  const year = Number(yearParam);
  const valid =
    Number.isInteger(month) && month >= 1 && month <= 12 && Number.isInteger(year) && year > 2000;
  return valid ? periodBounds(month, year) : null;
}

export async function listOvertime(actor: WorkforceActor, q: z.infer<typeof overtimeListQuerySchema>) {
  const db = await createServerPgClient();
  let query = db
    .from("overtime_requests")
    .select(OVERTIME_SELECT)
    .order("date", { ascending: false })
    .order("created_at", { ascending: false })
    .limit(200);

  if (q.scope === "approvals") {
    // Pengajuan yang menunggu keputusan SAYA sebagai atasan langsung
    if (!actor.employeeId) return [];
    const { data: subordinates } = await db
      .from("employees")
      .select("id")
      .eq("reporting_to", actor.employeeId)
      .neq("id", actor.employeeId); // reporting_to = diri sendiri bukan bawahan (EPIC-063 R2)
    const ids = ((subordinates ?? []) as { id: string }[]).map((row) => row.id);
    if (ids.length === 0) return [];
    query = query.in("employee_id", ids).eq("source", "employee");
  } else if (actor.isHr) {
    const target = q.employee_id === "me" ? actor.employeeId : q.employee_id;
    if (target) query = query.eq("employee_id", target);
  } else {
    // Non-HR hanya boleh melihat pengajuan miliknya sendiri
    if (!actor.employeeId) return [];
    query = query.eq("employee_id", actor.employeeId);
  }

  if (q.status) query = query.eq("status", q.status);
  const range = monthFilter(q.month, q.year);
  if (range) query = query.gte("date", range.start).lte("date", range.end);

  const { data, error } = await query;
  if (error) throw new Error(error.message);
  return data;
}

export async function createOvertime(actor: WorkforceActor, input: z.infer<typeof overtimeCreateSchema>) {
  // Id dibandingkan dalam bentuk kanonik (huruf kecil), seperti Go (P10): UUID_RE menerima huruf besar.
  let targetId: string | null = actor.employeeId;
  if (input.employee_id && input.employee_id !== "me") {
    targetId = canonicalUuid(input.employee_id);
    if (!targetId) throw ApiError.badRequest("ID karyawan tidak valid");
  }
  if (!targetId) throw ApiError.badRequest("Akun ini tidak tertaut ke data karyawan");

  const isSelf = targetId === actor.employeeId;
  // Administrasi lembur = himpunan HRD (ruling C2), sama dengan keputusannya.
  if (!isSelf && !actor.isHrd) {
    throw ApiError.forbidden("Hanya HRD yang bisa membuat penugasan lembur untuk karyawan lain");
  }

  // MOM AP-02 (EPIC-063 R3): pengajuan sendiri sebelum lembur dimulai.
  if (isSelf && overtimeStartsInPast(input.date, input.start_time)) {
    throw ApiError.badRequest(OVERTIME_TOO_LATE);
  }

  const db = await createServerPgClient();
  const { data: target } = await db
    .from("employees")
    .select("id, full_name, is_active")
    .eq("id", targetId)
    .maybeSingle();
  if (!target || target.is_active === false) {
    throw ApiError.notFound("Karyawan tidak ditemukan/nonaktif");
  }

  const { data: existing } = await db
    .from("overtime_requests")
    .select("id, status")
    .eq("employee_id", targetId)
    .eq("date", input.date)
    .in("status", ["pending", "approved"])
    .limit(1);
  if (existing && existing.length > 0) throw ApiError.badRequest(DUPLICATE);

  if (input.start_time === input.end_time) {
    throw ApiError.badRequest("Jam mulai dan selesai tidak boleh sama");
  }
  const hours = overtimeHoursFromTimes(input.start_time, input.end_time);
  if (hours > MAX_OVERTIME_HOURS) throw ApiError.badRequest("Durasi lembur maksimal 12 jam");

  // 'company' = penugasan HRD untuk orang lain, menunggu KONFIRMASI karyawan;
  // 'employee' menunggu approval HRD/atasan.
  const source = isSelf ? "employee" : "company";
  const { data, error } = await db
    .from("overtime_requests")
    .insert({
      employee_id: targetId,
      date: input.date,
      start_time: input.start_time,
      end_time: input.end_time,
      hours,
      source,
      status: "pending",
      reason: input.reason,
      requested_by: actor.employeeId,
    })
    .select(OVERTIME_SELECT)
    .single();
  // Unique index (employee_id, date) untuk status aktif: balapan duplikat
  if (error?.code === "23505") throw ApiError.badRequest(DUPLICATE);
  unwrap({ data, error });

  return {
    data,
    message:
      source === "company"
        ? "Penugasan lembur dibuat — menunggu konfirmasi karyawan"
        : "Pengajuan lembur terkirim — menunggu persetujuan",
  };
}

const DECISION_STATUS = { approve: "approved", reject: "rejected", cancel: "cancelled" } as const;
const DECISION_MESSAGE = {
  approved: "Lembur disetujui",
  rejected: "Lembur ditolak",
  cancelled: "Pengajuan dibatalkan",
} as const;

export async function decideOvertime(actor: WorkforceActor, input: z.infer<typeof overtimeDecideSchema>) {
  const db = await createServerPgClient();
  const { data: overtime } = await db
    .from("overtime_requests")
    .select(`*, employee:employees!employee_id ( id, full_name, reporting_to )`)
    .eq("id", input.overtime_id)
    .maybeSingle();
  if (!overtime) throw ApiError.notFound("Pengajuan lembur tidak ditemukan");
  if (overtime.status !== "pending") {
    throw ApiError.badRequest(`Pengajuan sudah ${overtime.status}`, {
      current_status: overtime.status,
    });
  }

  const decision = canDecideOvertime(
    { employeeId: actor.employeeId, isHr: actor.isHrd },
    {
      employee_id: overtime.employee_id,
      requested_by: overtime.requested_by,
      source: overtime.source,
      reporting_to: overtime.employee?.reporting_to ?? null,
    },
    input.action
  );
  if (!decision.allowed) throw ApiError.forbidden(decision.reason);

  if (input.action === "reject" && !input.rejection_reason?.trim()) {
    throw ApiError.badRequest("Alasan penolakan wajib diisi");
  }

  const status = DECISION_STATUS[input.action];
  // Kapasitas hanya untuk keputusan atas pengajuan karyawan (Q-APV-3); sudah lolos canDecideOvertime.
  const capacity =
    overtime.source === "employee" && input.action !== "cancel"
      ? decidedAs(
          { employeeId: actor.employeeId, isHr: actor.isHrd },
          overtime.employee_id,
          overtime.employee?.reporting_to ?? null
        )
      : null;
  const rejection = input.action === "reject" ? (input.rejection_reason ?? null) : null;
  const now = new Date().toISOString();
  const { data, error } = await db
    .from("overtime_requests")
    .update({
      status,
      decided_by: actor.employeeId,
      decided_at: now,
      rejection_reason: rejection,
      ...(capacity ? { decided_as: capacity, decided_by_name: decisionActorName(actor.fullName) } : {}),
      updated_at: now,
    })
    .eq("id", input.overtime_id)
    .eq("status", "pending")
    .select(`
      *,
      employee:employees!employee_id ( id, full_name, nip ),
      decider:employees!decided_by ( id, full_name )
    `)
    .single();
  // 0 baris ter-update = sudah diputuskan proses lain (guard status pending)
  if (error?.code === "PGRST116") throw ApiError.conflict("Pengajuan sudah diproses oleh orang lain");
  unwrap({ data, error });

  if (capacity) {
    await recordAuditAfterCommit({
      actor: { id: actor.userId, name: decisionActorName(actor.fullName) },
      action: input.action === "approve" ? "overtime.approve" : "overtime.reject",
      entity: "hris.overtime_requests",
      entityId: input.overtime_id,
      entityLabel: `${overtime.employee?.full_name ?? ""} · ${auditDateText(overtime.date)}`,
      before: { status: "pending" },
      after: { status, decided_as: capacity, rejection_reason: rejection },
    });
  }
  return { data, message: DECISION_MESSAGE[status] };
}
