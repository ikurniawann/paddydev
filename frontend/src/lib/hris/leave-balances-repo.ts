/**
 * Saldo cuti per karyawan per tahun (fallback lama, tidak terjangkau).
 *
 * EPIC-068: backend Go yang melayani saldo cuti (jatah default R1, penyesuaian,
 * sisa bawaan, ubah manual). Rute Go ada di go-routes.generated.json, jadi
 * proxy tidak meneruskan ke sini; hitungan prorata bulan di bawah SUDAH TIDAK
 * dipakai Go. Jangan dikembangkan; hapus bersama sisa fallback saldo lain.
 */

import { z } from "zod";
import { ApiError } from "@/lib/api/auth";
import { createServerPgClient } from "@/lib/pg/create-client";
import { isLineManagerRole } from "./onboarding-access";
import type { WorkforceActor } from "./workforce-auth";
import { unwrap } from "./workforce-route";

const ANNUAL_LEAVE_DEFAULT = 12;

export const leaveBalanceQuerySchema = z.object({
  year: z.coerce.number().int().min(2000).max(2100).optional(),
});

/**
 * Kuota cuti tahunan: 12 hari; karyawan yang masuk di tahun berjalan
 * mendapat 12 × sisa bulan / 12, dibulatkan ke bawah (bulan masuk dihitung).
 */
export function proratedAnnualQuota(joinDate: string | null, year: number): number {
  if (!joinDate) return ANNUAL_LEAVE_DEFAULT;
  const joined = new Date(joinDate);
  if (joined.getFullYear() !== year) return ANNUAL_LEAVE_DEFAULT;
  const monthsRemaining = 12 - joined.getMonth();
  return Math.max(0, Math.floor((monthsRemaining / 12) * ANNUAL_LEAVE_DEFAULT));
}

const BALANCE_SELECT = `
  *,
  employee:employees(
    id, full_name, nip, photo_url,
    department:departments(name), job_title:positions(title)
  )`;

async function initializeLeaveBalance(employeeId: string, year: number) {
  const db = await createServerPgClient();
  const { data: employee } = await db
    .from("employees")
    .select("join_date, employment_status")
    .eq("id", employeeId)
    .maybeSingle();
  if (!employee) throw ApiError.notFound("Employee not found");

  return unwrap(
    await db
      .from("leave_balances")
      .insert({
        employee_id: employeeId,
        year,
        annual_leave_total: proratedAnnualQuota(employee.join_date, year),
        annual_leave_used: 0,
        sick_leave_used: 0,
        unpaid_leave_used: 0,
        maternity_leave_used: 0,
        paternity_leave_used: 0,
      })
      .select(`
        *,
        employee:employees( id, full_name, nip, department:departments(name), job_title:positions(title) )
      `)
      .single()
  );
}

/** Pemilik, HRD, atau manajer; baris tahun itu dibuat bila belum ada. */
export async function getLeaveBalance(actor: WorkforceActor, employeeId: string, year: number) {
  const isOwner = actor.employeeId !== null && employeeId === actor.employeeId;
  if (!isOwner && !isLineManagerRole(actor.role)) {
    throw ApiError.forbidden("Forbidden: Can only view own leave balance");
  }

  const db = await createServerPgClient();
  const { data, error } = await db
    .from("leave_balances")
    .select(BALANCE_SELECT)
    .eq("employee_id", employeeId)
    .eq("year", year)
    .single();
  if (data) return data;
  if (error?.code === "PGRST116") return initializeLeaveBalance(employeeId, year);
  throw new Error(error?.message ?? "Gagal memuat saldo cuti");
}
