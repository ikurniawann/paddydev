/**
 * Jejak audit server-side (port platform/audit NüHabit).
 *
 * `recordAudit(client, entry)` menulis satu baris ke audit.audit_log memakai
 * client yang sama dengan aksinya: di dalam `withTransaction` baris audit ikut
 * commit atau rollback bersama aksi. Tabelnya append-only di level database.
 *
 * configuration.activity_logs tidak dipakai: tabel itu log kandidat rekrutmen
 * (candidate_id NOT NULL) tanpa aktor maupun before/after.
 */

import { getPool } from "@/lib/db";

/** Entitas HRIS yang penghapusannya diaudit (EPIC-061; sama dengan Go hrisdel). */
export type HrisDeleteEntity =
  | "hris.employees"
  | "hris.employee_documents"
  | "hris.attendance"
  | "hris.leaves"
  | "hris.shifts"
  | "hris.public_holidays"
  | "hris.employment_contracts"
  | "hris.contract_signed_document"
  | "hris.announcements"
  | "hris.departments"
  | "hris.employment_statuses"
  | "hris.positions"
  | "hris.payroll_runs"
  | "hris.employee_salary"
  | "performance.kpi_targets"
  | "performance.feedback_assignments"
  | "performance.feedback_cycles"
  | "performance.feedback_responses"
  | "recruitment.candidates"
  | "recruitment.candidate_cv"
  | "recruitment.psikotes_questions"
  | "recruitment.job_openings";

export type AuditAction =
  | "stock.adjust"
  | "stock.opname_complete"
  | "stock.scrap"
  | "stock.transfer"
  | "po.approve"
  | "po.cancel"
  | "grn.post"
  | "ap_payment.create"
  | "ap_payment.void"
  | "vendor_credit.approve"
  | "vendor_credit.apply"
  | "purchase_return.revise"
  | "attendance.delete"
  | "leave.approve"
  | "leave.reject"
  | "overtime.approve"
  | "overtime.reject"
  | `${HrisDeleteEntity}.delete`;

export type AuditEntity =
  | "inventory"
  | "stock_opname"
  | "stock_transfer"
  | "purchase_order"
  | "grn"
  | "ap_payment"
  | "vendor_credit"
  | "purchase_return"
  | "hris.overtime_requests"
  | HrisDeleteEntity;

export interface AuditActor {
  id: string | null;
  name?: string | null;
}

export interface AuditEntry {
  actor: AuditActor;
  action: AuditAction;
  entity: AuditEntity;
  entityId: string | null;
  /** Nomor dokumen / label yang dibaca manusia. */
  entityLabel?: string | null;
  before?: unknown;
  after?: unknown;
  reason?: string | null;
  ip?: string | null;
  userAgent?: string | null;
}

export interface AuditQueryable {
  query: (text: string, params?: unknown[]) => Promise<unknown>;
}

function toJson(value: unknown): string | null {
  if (value === undefined || value === null) return null;
  return JSON.stringify(value);
}

export async function recordAudit(client: AuditQueryable, entry: AuditEntry): Promise<void> {
  await client.query(
    `INSERT INTO audit.audit_log
       (actor_id, actor_name, action, entity, entity_id, entity_label,
        before, after, reason, ip, user_agent)
     VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10, $11)`,
    [
      entry.actor.id,
      entry.actor.name ?? null,
      entry.action,
      entry.entity,
      entry.entityId,
      entry.entityLabel ?? null,
      toJson(entry.before),
      toJson(entry.after),
      entry.reason?.trim() || null,
      entry.ip ?? null,
      entry.userAgent ?? null,
    ]
  );
}

/**
 * Untuk alur lama yang belum transaksional (query builder per panggilan): tulis
 * sesudah aksi tersimpan. Kegagalan audit dicatat ke log, tidak membatalkan
 * respons atas aksi yang sudah commit.
 */
export async function recordAuditAfterCommit(entry: AuditEntry): Promise<void> {
  try {
    await recordAudit(getPool(), entry);
  } catch (error) {
    console.error(`[audit] gagal mencatat ${entry.action} ${entry.entityId ?? ""}:`, error);
  }
}

/** IP & user agent dari header permintaan (di balik proxy: x-forwarded-for). */
export function requestMeta(request: { headers?: Headers } | null | undefined): {
  ip: string | null;
  userAgent: string | null;
} {
  const headers = request?.headers;
  if (!headers) return { ip: null, userAgent: null };
  const forwarded = headers.get("x-forwarded-for");
  const ip = forwarded?.split(",")[0]?.trim() || headers.get("x-real-ip") || null;
  return { ip, userAgent: headers.get("user-agent") };
}
