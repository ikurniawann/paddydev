/**
 * Pengajuan cuti: daftar ber-scope, pengajuan, detail, pembatalan/edit HR,
 * hapus, dan approve/reject. Status + potong/kembalikan kuota tahunan selalu
 * dalam SATU transaksi.
 */

import { z } from "zod";
import { ApiError } from "@/lib/api/auth";
import { recordAudit } from "@/lib/audit";
import { withTransaction } from "@/lib/db";
import { createPgClient, createServerPgClient } from "@/lib/pg/create-client";
import { buildWaLink } from "@/lib/recruitment/wa";
import { describeLeaveDays } from "./holidays";
import { loadHolidayIndex } from "./holidays-db";
import { SELECTABLE_LEAVE_CODES } from "./leave-types";
import { notifyLeaveRequestWa } from "./leave-wa";
import { canApproveLeaf, HRIS_APPROVE_LEAF } from "./approve-access";
import { auditDateText, decisionActorName } from "./decision-audit";
import { decidedAs } from "./overtime-rules";
import type { WorkforceActor } from "./workforce-auth";
import { DATE_RE, requireLinkedEmployee, unwrap } from "./workforce-route";

export const leaveListQuerySchema = z.object({
  employee_id: z.string().optional(),
  status: z.string().optional(),
  leave_type: z.string().optional(),
  start_date: z.string().optional(),
  end_date: z.string().optional(),
  page: z.coerce.number().int().min(1).default(1),
  limit: z.coerce.number().int().min(1).max(500).default(20),
});

export const leaveRequestSchema = z.object({
  employee_id: z.string().uuid().optional(),
  leave_type: z.enum(SELECTABLE_LEAVE_CODES),
  start_date: z.string().regex(DATE_RE, "Tanggal mulai harus berformat YYYY-MM-DD"),
  end_date: z.string().regex(DATE_RE, "Tanggal selesai harus berformat YYYY-MM-DD"),
  reason: z.string().min(10, "Reason must be at least 10 characters"),
  attachment_url: z.string().optional(),
});

export const leaveUpdateSchema = z.object({
  status: z.string().optional(),
  reason: z.string().nullish(),
  attachment_url: z.string().nullish(),
});

export const leaveApprovalSchema = z.object({
  leave_id: z.string().uuid(),
  action: z.enum(["approve", "reject"]),
  rejection_reason: z.string().optional(),
});

const EMPLOYEE_WITH_DEPT = "id, full_name, nip, department:departments(name)";

export async function listLeaves(actor: WorkforceActor, q: z.infer<typeof leaveListQuerySchema>) {
  // Non-HR hanya boleh melihat pengajuan cutinya sendiri
  const employeeId = actor.isHr ? q.employee_id : requireLinkedEmployee(actor);

  let query = createPgClient()
    .from("leaves")
    .select(
      `*,
       employee:employees!employee_id( ${EMPLOYEE_WITH_DEPT} ),
       approver:employees!approved_by( id, full_name, nip )`,
      { count: "exact" }
    );
  if (employeeId) query = query.eq("employee_id", employeeId);
  if (q.status) query = query.eq("status", q.status);
  if (q.leave_type) query = query.eq("leave_type", q.leave_type);
  if (q.start_date && q.end_date) {
    query = query.gte("start_date", q.start_date).lte("end_date", q.end_date);
  }

  const from = (q.page - 1) * q.limit;
  const { data, error, count } = await query
    .range(from, from + q.limit - 1)
    .order("created_at", { ascending: false });
  if (error) throw new Error(error.message);

  return {
    data: data || [],
    pagination: {
      page: q.page,
      limit: q.limit,
      total: count || 0,
      totalPages: Math.ceil((count || 0) / q.limit),
    },
  };
}

/** Kalimat penolakan untuk rentang yang seluruhnya libur/akhir pekan. */
function noWorkingDayMessage(excludedHolidays: { name: string }[]): string {
  const alasan =
    excludedHolidays.length > 0
      ? `sudah hari libur (${excludedHolidays.map((h) => h.name).join(", ")})`
      : "jatuh pada akhir pekan";
  return `Rentang tanggal ini ${alasan} — tidak perlu mengajukan cuti`;
}

/**
 * Ajukan cuti. Hari yang memotong jatah: akhir pekan dan libur nasional
 * dikecualikan, cuti bersama TETAP memotong (SKB, EPIC-036 Fase D).
 */
export async function createLeave(actor: WorkforceActor, input: z.infer<typeof leaveRequestSchema>) {
  // Non-HR hanya boleh mengajukan cuti untuk dirinya sendiri
  const employeeId = actor.isHr && input.employee_id ? input.employee_id : actor.employeeId;
  if (!employeeId) throw ApiError.notFound("Akun ini tidak terhubung ke data karyawan");

  const db = createPgClient();
  const { data: employee } = await db
    .from("employees")
    .select("id, full_name, employment_status, is_active")
    .eq("id", employeeId)
    .single();
  if (!employee || !employee.is_active) throw ApiError.notFound("Employee not found or inactive");

  if (input.end_date < input.start_date) {
    throw ApiError.badRequest("Tanggal selesai tidak boleh sebelum tanggal mulai");
  }

  // Sengaja TANPA backfill: cuti yang sudah disetujui memakai total_days lama.
  const holidayIndex = await loadHolidayIndex(input.start_date, input.end_date);
  const { totalDays, excludedHolidays } = describeLeaveDays(
    input.start_date,
    input.end_date,
    holidayIndex
  );
  // 0 hari kerja ditolak: memotong jatah untuk hari yang sudah libur itu bug.
  if (totalDays === 0) throw ApiError.badRequest(noWorkingDayMessage(excludedHolidays));

// EPIC-068: fallback lama, tidak terjangkau. Pengajuan/persetujuan/pembatalan
// cuti dilayani backend Go (go-routes.generated.json), yang juga mengenal
// penyesuaian dan sisa bawaan (R4-R6); jalur saldo tahunan di sini tidak.
  if (input.leave_type === "annual") {
    const { data: balance } = await db
      .from("leave_balances")
      .select("annual_leave_remaining")
      .eq("employee_id", employeeId)
      .eq("year", new Date(input.start_date).getFullYear())
      .single();
    if (balance && balance.annual_leave_remaining < totalDays) {
      throw ApiError.badRequest("Insufficient annual leave balance", {
        remaining: balance.annual_leave_remaining,
        requested: totalDays,
      });
    }
  }

  const data = unwrap(
    await db
      .from("leaves")
      .insert({
        employee_id: employeeId,
        leave_type: input.leave_type,
        start_date: input.start_date,
        end_date: input.end_date,
        total_days: totalDays,
        reason: input.reason,
        attachment_url: input.attachment_url || null,
        status: "pending",
      })
      .select("*")
      .single()
  );

  // Notifikasi WA ke atasan langsung; gagal kirim tidak menggagalkan pengajuan.
  void notifyLeaveRequestWa({
    leaveId: String(data.id),
    employeeId,
    employeeName: employee.full_name,
    leaveType: input.leave_type,
    startDate: input.start_date,
    endDate: input.end_date,
    totalDays,
    reason: input.reason ?? null,
  });

  return {
    message: "Leave request submitted successfully",
    data,
    // Menjelaskan kenapa total_days lebih kecil dari rentang kalendernya
    meta: { total_days: totalDays, excluded_holidays: excludedHolidays },
  };
}

/** Detail cuti: HR, pemilik, atau atasan langsung karyawan ybs. */
export async function getLeave(actor: WorkforceActor, id: string) {
  const db = await createServerPgClient();
  const { data } = await db
    .from("leaves")
    .select(`
      *,
      employee:employees!employee_id(
        id, full_name, nip, photo_url, email, phone, reporting_to,
        department:departments(name), job_title:positions(title)
      ),
      approver:employees!leaves_approved_by_fkey( id, full_name, nip, email )
    `)
    .eq("id", id)
    .maybeSingle();
  if (!data) throw ApiError.notFound("Leave request not found");

  const own = actor.employeeId !== null && data.employee_id === actor.employeeId;
  const manager = actor.employeeId !== null && data.employee?.reporting_to === actor.employeeId;
  if (!actor.isHr && !own && !manager) throw ApiError.forbidden("Insufficient permissions");
  return data;
}

/**
 * Karyawan membatalkan pengajuannya yang masih pending; HR membatalkan cuti
 * (termasuk yang SUDAH disetujui, kuota tahunan dikembalikan dalam transaksi
 * yang sama) dan mengedit alasan/lampiran.
 */
export async function updateLeave(
  actor: WorkforceActor,
  id: string,
  input: z.infer<typeof leaveUpdateSchema>
) {
  const db = await createServerPgClient();
  const { data: leave } = await db.from("leaves").select("*").eq("id", id).maybeSingle();
  if (!leave) throw ApiError.notFound("Leave request not found");

  if (input.status === "cancelled") {
    const isOwner = actor.employeeId !== null && leave.employee_id === actor.employeeId;
    const cancellablePending = leave.status === "pending" && (isOwner || actor.isHr);
    const cancellableApproved = leave.status === "approved" && actor.isHr;
    if (!cancellablePending && !cancellableApproved) {
      throw ApiError.forbidden(
        leave.status === "approved"
          ? "Cuti yang sudah disetujui hanya bisa dibatalkan oleh HRD/admin"
          : "Pengajuan ini tidak bisa dibatalkan"
      );
    }

// EPIC-068: fallback lama, tidak terjangkau. Pengajuan/persetujuan/pembatalan
// cuti dilayani backend Go (go-routes.generated.json), yang juga mengenal
// penyesuaian dan sisa bawaan (R4-R6); jalur saldo tahunan di sini tidak.
    const refund = cancellableApproved && leave.leave_type === "annual";
    await withTransaction(async (client) => {
      await client.query(`UPDATE hris.leaves SET status = 'cancelled' WHERE id = $1`, [id]);
      if (refund) {
        await client.query(
          `UPDATE hris.leave_balances
           SET annual_leave_used = GREATEST(annual_leave_used - $3, 0),
               updated_at = now()
           WHERE employee_id = $1 AND year = $2`,
          [leave.employee_id, new Date(leave.start_date).getFullYear(), leave.total_days]
        );
      }
    });
    return {
      message: refund
        ? `Cuti dibatalkan — kuota ${leave.total_days} hari dikembalikan`
        : "Pengajuan dibatalkan",
    };
  }

  const update: { reason?: string | null; attachment_url?: string | null } = {};
  if (input.reason !== undefined) update.reason = input.reason;
  if (input.attachment_url !== undefined) update.attachment_url = input.attachment_url;
  if (!actor.isHr || Object.keys(update).length === 0) {
    throw ApiError.forbidden("No valid updates or insufficient permissions");
  }

  const data = unwrap(await db.from("leaves").update(update).eq("id", id).select().single());
  return { message: "Leave request updated successfully", data };
}

/** Hapus pengajuan; route memeriksa aksi "delete" di Cuti & Izin (EPIC-061). */
export async function deleteLeave(id: string) {
  const db = await createServerPgClient();
  unwrap(await db.from("leaves").delete().eq("id", id));
}

/**
 * Approve/reject oleh HRD, admin, atau ATASAN LANGSUNG (reporting_to).
 * approved_by ber-FK ke hris.employees: diisi record karyawan approver (null
 * untuk akun tanpa karyawan). Mengembalikan link WhatsApp untuk approver.
 */
export async function decideLeave(actor: WorkforceActor, input: z.infer<typeof leaveApprovalSchema>) {
  const db = await createServerPgClient();
  const { data: leave } = await db
    .from("leaves")
    .select(`
      *,
      employee:employees!employee_id(
        id, full_name, email, phone, reporting_to,
        department:departments(id, name), job_title:positions(id, title)
      )
    `)
    .eq("id", input.leave_id)
    .maybeSingle();
  if (!leave) throw ApiError.notFound("Leave request not found");

  // Q-APV-3 (EPIC-063 R1/R2), sama dengan Go DecideLeave; HR = "approve" pada daun Cuti (EPIC-067 R11/R15).
  const capacity = decidedAs(
    { employeeId: actor.employeeId, isHr: await canApproveLeaf(actor, HRIS_APPROVE_LEAF.cuti) },
    leave.employee_id,
    leave.employee?.reporting_to ?? null
  );
  if (!capacity) {
    throw ApiError.forbidden(
      actor.employeeId !== null && actor.employeeId === leave.employee_id
        ? "Tidak bisa memutuskan pengajuan cuti sendiri"
        : "Hanya atasan langsung atau HRD yang bisa memproses pengajuan ini"
    );
  }
  if (leave.status !== "pending") {
    throw ApiError.badRequest(`Leave request already ${leave.status}`, {
      current_status: leave.status,
    });
  }
  if (input.action === "reject" && !input.rejection_reason) {
    throw ApiError.badRequest("Rejection reason is required");
  }

  const isApprove = input.action === "approve";
  const status = isApprove ? "approved" : "rejected";
  const rejection = isApprove ? null : (input.rejection_reason ?? null);
  await withTransaction(async (client) => {
    const updated = await client.query(
      `UPDATE hris.leaves
       SET status = $2, approved_by = $3, approved_at = now(),
           rejection_reason = $4, decided_as = $5, decided_by_name = $6
       WHERE id = $1 AND status = 'pending'`,
      [input.leave_id, status, actor.employeeId, rejection, capacity, decisionActorName(actor.fullName)]
    );
    if (updated.rowCount === 0) throw ApiError.conflict("Pengajuan sudah diproses oleh orang lain");
// EPIC-068: fallback lama, tidak terjangkau. Pengajuan/persetujuan/pembatalan
// cuti dilayani backend Go (go-routes.generated.json), yang juga mengenal
// penyesuaian dan sisa bawaan (R4-R6); jalur saldo tahunan di sini tidak.
    if (isApprove && leave.leave_type === "annual") {
      await client.query(
        `INSERT INTO hris.leave_balances (employee_id, year, annual_leave_total, annual_leave_used)
         VALUES ($1, $2, 12, $3)
         ON CONFLICT (employee_id, year)
         DO UPDATE SET annual_leave_used = leave_balances.annual_leave_used + $3,
                       updated_at = now()`,
        [leave.employee_id, new Date(leave.start_date).getFullYear(), leave.total_days]
      );
    }
    await recordAudit(client, {
      actor: { id: actor.userId, name: decisionActorName(actor.fullName) },
      action: isApprove ? "leave.approve" : "leave.reject",
      entity: "hris.leaves",
      entityId: input.leave_id,
      entityLabel: `${leave.employee?.full_name ?? ""} · ${auditDateText(leave.start_date)} s.d. ${auditDateText(leave.end_date)}`,
      before: { status: "pending" },
      after: { status, decided_as: capacity, rejection_reason: rejection },
    });
  });

  const { data } = await db
    .from("leaves")
    .select(`
      *,
      employee:employees!employee_id( id, full_name, email, department:departments(name) ),
      approver:employees!approved_by( id, full_name, email )
    `)
    .eq("id", input.leave_id)
    .single();

  // Notifikasi WhatsApp pola wa.me: link dibuka approver dari UI
  const name = leave.employee?.full_name;
  const rangeLabel = `${leave.start_date} s.d. ${leave.end_date} (${leave.total_days} hari)`;
  const waMessage = isApprove
    ? `Halo ${name}, pengajuan izin/cuti Anda ${rangeLabel} telah DISETUJUI. Selamat beristirahat!`
    : `Halo ${name}, mohon maaf pengajuan izin/cuti Anda ${rangeLabel} DITOLAK. Alasan: ${input.rejection_reason}. Silakan hubungi HRD untuk diskusi.`;

  return {
    message: `Leave request ${input.action}d successfully`,
    action: input.action,
    wa_link: buildWaLink(leave.employee?.phone, waMessage),
    data,
  };
}
