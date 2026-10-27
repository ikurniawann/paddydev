/**
 * Data Area Karyawan (ESS) milik akun yang login: identitas, saldo cuti,
 * jadwal shift, dan ringkasan beranda /dashboard/me dalam satu round trip.
 */

import { query, queryOne } from "@/lib/db";
import { formatRupiah } from "@/lib/format";
import { addDaysIso } from "@/lib/payroll/period";
import {
  isoDayOfWeek,
  resolveScheduleRowForDate,
  WIB_OFFSET_HOURS,
  type EmployeeShiftRow,
} from "./shifts";

/** Baris jadwal + detail shift (join shifts). */
export type ScheduleRow = EmployeeShiftRow & {
  shift_name: string | null;
  start_time: string | null;
  end_time: string | null;
  late_tolerance_minutes: number | null;
};

export interface TodayShift {
  name: string;
  start_time: string | null;
  end_time: string | null;
  late_tolerance_minutes: number;
}

export interface DaySchedule {
  date: string;
  day_of_week: number; // 1=Senin … 7=Minggu
  is_today: boolean;
  status: "shift" | "libur" | "none";
  shift_name: string | null;
  start_time: string | null;
  end_time: string | null;
}

/** Waktu sekarang digeser ke WIB (baca komponen UTC-nya). */
export const nowWib = (): Date => new Date(Date.now() + WIB_OFFSET_HOURS * 3600_000);

/** Tanggal WIB hari ini (YYYY-MM-DD). */
export const todayWibIso = (): string => nowWib().toISOString().slice(0, 10);

/** Detail shift (nama/jam) untuk baris terpilih; null bila libur/tanpa jadwal. */
function shiftDetail(rows: ScheduleRow[], row: ScheduleRow | null): ScheduleRow | null {
  if (!row || row.shift_id === null) return null;
  return rows.find((r) => r.shift_id === row.shift_id && r.shift_name) ?? row;
}

/** Shift yang berlaku hari ini; null bila libur atau tanpa jadwal. */
export function todayShiftOf(rows: ScheduleRow[], today: string): TodayShift | null {
  const row = resolveScheduleRowForDate(rows, today);
  const detail = shiftDetail(rows, row);
  if (!row || row.shift_id === null || !detail?.shift_name) return null;
  return {
    name: detail.shift_name,
    start_time: detail.start_time,
    end_time: detail.end_time,
    late_tolerance_minutes: detail.late_tolerance_minutes ?? 0,
  };
}

/** Jadwal Senin–Minggu untuk minggu yang memuat `today`. */
export function weekScheduleOf(rows: ScheduleRow[], today: string): DaySchedule[] {
  const monday = addDaysIso(today, -(isoDayOfWeek(today) - 1));
  return Array.from({ length: 7 }, (_, i) => {
    const date = addDaysIso(monday, i);
    const row = resolveScheduleRowForDate(rows, date);
    const status: DaySchedule["status"] =
      row === null ? "none" : row.shift_id === null ? "libur" : "shift";
    const detail = status === "shift" ? shiftDetail(rows, row) : null;
    return {
      date,
      day_of_week: isoDayOfWeek(date),
      is_today: date === today,
      status,
      shift_name: detail?.shift_name ?? null,
      start_time: detail?.start_time ?? null,
      end_time: detail?.end_time ?? null,
    };
  });
}

interface RecentLeave {
  id: string;
  leave_type: string;
  start_date: string;
  end_date: string;
  status: string;
  created_at: string;
}
interface RecentOvertime {
  id: string;
  date: string;
  start_time: string;
  end_time: string;
  status: string;
  created_at: string;
}
interface RecentLoan {
  id: string;
  loan_type: string;
  principal_amount: string;
  status: string;
  created_at: string;
}

/** Gabungan pengajuan terbaru (cuti/lembur/pinjaman), 6 teratas by created_at. */
export function mergeRecentRequests(
  leaves: RecentLeave[],
  overtime: RecentOvertime[],
  loans: RecentLoan[]
) {
  return [
    ...leaves.map((l) => ({
      kind: "cuti" as const,
      id: l.id,
      label: l.leave_type,
      detail: `${l.start_date} → ${l.end_date}`,
      status: l.status,
      created_at: l.created_at,
      href: "/dashboard/me/cuti",
    })),
    ...overtime.map((o) => ({
      kind: "lembur" as const,
      id: o.id,
      label: `Lembur ${o.date}`,
      detail: `${o.start_time?.slice(0, 5)}–${o.end_time?.slice(0, 5)}`,
      status: o.status,
      created_at: o.created_at,
      href: "/dashboard/me/lembur",
    })),
    ...loans.map((p) => ({
      kind: "pinjaman" as const,
      id: p.id,
      label: p.loan_type,
      detail: formatRupiah(p.principal_amount),
      status: p.status,
      created_at: p.created_at,
      href: "/dashboard/me/pinjaman",
    })),
  ]
    .sort((a, b) => (a.created_at < b.created_at ? 1 : -1))
    .slice(0, 6);
}

// ── Query bersama /me dan /me/beranda ──────────────────────────────────

function loadEmployeeCard(employeeId: string) {
  return queryOne<{
    id: string;
    full_name: string;
    nip: string | null;
    join_date: string | null;
    employment_status: string;
    photo_url: string | null;
    position_title: string | null;
    department_name: string | null;
  }>(
    `SELECT e.id, e.full_name, e.nip, e.join_date::text, e.employment_status, e.photo_url,
            p.title AS position_title, d.name AS department_name
     FROM hris.employees e
     LEFT JOIN hris.positions p ON p.id = e.job_title_id
     LEFT JOIN hris.departments d ON d.id = e.department_id
     WHERE e.id = $1`,
    [employeeId]
  );
}

interface LeaveBalanceRow {
  year: number;
  annual_leave_total: string;
  annual_leave_adjustment: string;
  annual_leave_used: string;
  annual_leave_remaining: string;
  carry_over_days: string;
  carry_over_used: string;
  carry_over_expires_on: string | null;
}

/** Baris saldo → bentuk yang sama dengan respons Go (EPIC-068 R12): angka, bawaan, dan `available` (R4). */
export function essLeaveBalanceOf(row: LeaveBalanceRow, today: string) {
  // Dihitung ulang bisa di bawah terpakai (tinjauan HR terbuka): tampil 0, bukan negatif.
  const carryRemaining = Math.max(Number(row.carry_over_days) - Number(row.carry_over_used), 0);
  const carryActive = !!row.carry_over_expires_on && row.carry_over_expires_on >= today;
  const remaining = Number(row.annual_leave_remaining);
  return {
    year: row.year,
    annual_leave_total: Number(row.annual_leave_total),
    annual_leave_adjustment: Number(row.annual_leave_adjustment),
    annual_leave_used: Number(row.annual_leave_used),
    annual_leave_remaining: remaining,
    carry_over_days: Number(row.carry_over_days),
    carry_over_used: Number(row.carry_over_used),
    carry_over_remaining: carryRemaining,
    carry_over_expires_on: row.carry_over_expires_on,
    carry_over_active: carryActive,
    available: remaining + (carryActive ? carryRemaining : 0),
  };
}

async function loadCurrentLeaveBalance(employeeId: string) {
  const row = await queryOne<LeaveBalanceRow>(
    `SELECT year, annual_leave_total, annual_leave_adjustment, annual_leave_used, annual_leave_remaining,
            carry_over_days, carry_over_used, carry_over_expires_on::text
     FROM hris.leave_balances
     WHERE employee_id = $1 AND year = date_part('year', now())::int`,
    [employeeId]
  );
  return row ? essLeaveBalanceOf(row, todayWibIso()) : null;
}

function loadScheduleRows(employeeId: string) {
  return query<ScheduleRow>(
    `SELECT es.day_of_week, es.shift_id,
            es.effective_from::text, es.effective_to::text,
            s.name AS shift_name, s.start_time::text, s.end_time::text,
            s.late_tolerance_minutes
     FROM hris.employee_shifts es
     LEFT JOIN hris.shifts s ON s.id = es.shift_id
     WHERE es.employee_id = $1`,
    [employeeId]
  );
}

/** GET /api/hris/me: identitas + kuota cuti tahun berjalan + shift hari ini. */
export async function loadMe(employeeId: string) {
  const [employee, leaveBalance, scheduleRows] = await Promise.all([
    loadEmployeeCard(employeeId),
    loadCurrentLeaveBalance(employeeId),
    loadScheduleRows(employeeId),
  ]);
  return {
    employee,
    leave_balance: leaveBalance,
    today_shift: todayShiftOf(scheduleRows, todayWibIso()),
    has_schedule: scheduleRows.length > 0,
  };
}

/**
 * GET /api/hris/me/beranda: ringkasan beranda ESS (identitas, saldo cuti,
 * shift hari ini + minggu ini, absensi bulan berjalan, slip terbaru, pinjaman
 * aktif, pengajuan terbaru, pengumuman, ringkasan KPI terakhir).
 */
export async function loadBeranda(emp: string) {
  const today = todayWibIso();
  const now = nowWib();
  const month = now.getUTCMonth() + 1;
  const year = now.getUTCFullYear();

  const [
    employee,
    leaveBalance,
    scheduleRows,
    attendance,
    payslip,
    loans,
    leaves,
    overtime,
    loanRequests,
    announcementRows,
    kpi,
  ] = await Promise.all([
    loadEmployeeCard(emp),
    loadCurrentLeaveBalance(emp),
    loadScheduleRows(emp),
    queryOne<{
      present: string;
      late: string;
      off_schedule: string;
      avg_work_hours: string | null;
      clocked_in_today: string;
    }>(
      `SELECT count(*) AS present,
              count(*) FILTER (WHERE is_late) AS late,
              count(*) FILTER (WHERE shift_id IS NULL) AS off_schedule,
              round(avg(work_hours), 1) AS avg_work_hours,
              count(*) FILTER (WHERE date = (now() + interval '7 hours')::date) AS clocked_in_today
       FROM hris.attendance
       WHERE employee_id = $1
         AND date_part('month', date) = $2 AND date_part('year', date) = $3`,
      [emp, month, year]
    ),
    queryOne<{
      net_salary: string;
      run_name: string | null;
      period_month: number;
      period_year: number;
      paid_at: string | null;
    }>(
      `SELECT pd.net_salary, pr.run_name, pr.period_month, pr.period_year, pr.paid_at::text
       FROM hris.payroll_details pd
       JOIN hris.payroll_runs pr ON pr.id = pd.payroll_run_id
       WHERE pd.employee_id = $1 AND pr.status = 'paid'
       ORDER BY pr.period_year DESC, pr.period_month DESC, pr.paid_at DESC NULLS LAST
       LIMIT 1`,
      [emp]
    ),
    queryOne<{ count: string; total_remaining: string; monthly_installment: string }>(
      `SELECT count(*) AS count,
              COALESCE(sum(remaining_balance), 0) AS total_remaining,
              COALESCE(sum(monthly_installment)
                       FILTER (WHERE status = 'approved' AND COALESCE(remaining_balance, 0) > 0), 0)
                AS monthly_installment
       FROM hris.loans
       WHERE employee_id = $1 AND is_active
         AND status IN ('pending', 'approved')
         AND (status = 'pending' OR COALESCE(remaining_balance, 0) > 0)`,
      [emp]
    ),
    query<RecentLeave>(
      `SELECT id, leave_type, start_date::text, end_date::text, status, created_at::text
       FROM hris.leaves WHERE employee_id = $1
       ORDER BY created_at DESC LIMIT 5`,
      [emp]
    ),
    query<RecentOvertime>(
      `SELECT id, date::text, start_time::text, end_time::text, status, source, created_at::text
       FROM hris.overtime_requests WHERE employee_id = $1
       ORDER BY created_at DESC LIMIT 5`,
      [emp]
    ),
    query<RecentLoan>(
      `SELECT id, loan_type, principal_amount, status, created_at::text
       FROM hris.loans WHERE employee_id = $1
       ORDER BY created_at DESC LIMIT 5`,
      [emp]
    ),
    query<{
      id: string;
      title: string;
      cover_image_url: string | null;
      tags: string[];
      is_pinned: boolean;
      publish_at: string | null;
      created_at: string;
      is_read: boolean;
    }>(
      `SELECT a.id, a.title, a.cover_image_url, a.tags, a.is_pinned,
              a.publish_at::text, a.created_at::text,
              (r.employee_id IS NOT NULL) AS is_read
       FROM hris.announcements a
       JOIN hris.employees e ON e.id = $1
       LEFT JOIN hris.announcement_reads r
         ON r.announcement_id = a.id AND r.employee_id = $1
       WHERE a.status = 'published'
         AND (a.publish_at IS NULL OR a.publish_at <= now())
         AND (a.expires_at IS NULL OR a.expires_at > now())
         AND (
           a.target_scope = 'global'
           OR EXISTS (
             SELECT 1 FROM hris.announcement_departments ad
             WHERE ad.announcement_id = a.id AND ad.department_id = e.department_id
           )
         )
       ORDER BY a.is_pinned DESC, COALESCE(a.publish_at, a.created_at) DESC
       LIMIT 20`,
      [emp]
    ),
    queryOne<{ kpi_count: string; avg_achievement: string | null; avg_score: string | null }>(
      `SELECT count(*) AS kpi_count,
              round(avg(achievement_percentage), 0) AS avg_achievement,
              round(avg(score)::numeric, 1) AS avg_score
       FROM hris.employee_kpis
       WHERE employee_id = $1
         AND review_id = (
           SELECT review_id FROM hris.employee_kpis
           WHERE employee_id = $1 ORDER BY created_at DESC LIMIT 1
         )`,
      [emp]
    ),
  ]);

  return {
    employee,
    leave_balance: leaveBalance,
    today_shift: todayShiftOf(scheduleRows, today),
    has_schedule: scheduleRows.length > 0,
    week_schedule: weekScheduleOf(scheduleRows, today),
    attendance: {
      month,
      year,
      present: Number(attendance?.present ?? 0),
      late: Number(attendance?.late ?? 0),
      off_schedule: Number(attendance?.off_schedule ?? 0),
      avg_work_hours: attendance?.avg_work_hours ? Number(attendance.avg_work_hours) : null,
      clocked_in_today: Number(attendance?.clocked_in_today ?? 0) > 0,
    },
    latest_payslip: payslip
      ? {
          net_salary: Number(payslip.net_salary),
          run_name: payslip.run_name,
          period_month: payslip.period_month,
          period_year: payslip.period_year,
          paid_at: payslip.paid_at,
        }
      : null,
    active_loans: {
      count: Number(loans?.count ?? 0),
      total_remaining: Number(loans?.total_remaining ?? 0),
      monthly_installment: Number(loans?.monthly_installment ?? 0),
    },
    recent_requests: mergeRecentRequests(leaves, overtime, loanRequests),
    announcements: {
      items: announcementRows.slice(0, 4),
      unread: announcementRows.filter((a) => !a.is_read).length,
      total: announcementRows.length,
    },
    kpi:
      kpi && Number(kpi.kpi_count) > 0
        ? {
            count: Number(kpi.kpi_count),
            avg_achievement: kpi.avg_achievement ? Number(kpi.avg_achievement) : null,
            avg_score: kpi.avg_score ? Number(kpi.avg_score) : null,
          }
        : null,
  };
}
