/**
 * Laporan HRIS bulanan (dashboard Insights): headcount, absensi, cuti.
 * buildHrisReport murni dan teruji; loadHrisReport hanya memuat baris.
 */

import type { PgClient } from "@/lib/pg/create-client";
import { leaveTypeShort } from "./leave-types";

export interface ReportEmployeeRow {
  id: string;
  employment_status: string;
  is_active: boolean;
  department_id: string | null;
  join_date: string;
  end_date: string | null;
}

export interface ReportDepartmentRow {
  id: string;
  name: string;
}

export interface ReportAttendanceRow {
  status: string;
  is_late: boolean | null;
  work_hours: number | null;
  date: string;
}

export interface ReportLeaveRow {
  leave_type: string;
  status: string;
  total_days: number | null;
}

export interface HrisReportRows {
  employees: ReportEmployeeRow[];
  departments: ReportDepartmentRow[];
  attendance: ReportAttendanceRow[];
  leaves: ReportLeaveRow[];
}

const pad2 = (n: number) => String(n).padStart(2, "0");
const firstOfMonth = (d: Date) => `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-01`;

/** [awal bulan, awal bulan berikutnya) dalam ISO. */
export function monthRange(month: number, year: number): { start: string; end: string } {
  return {
    start: `${year}-${pad2(month)}-01`,
    end: month === 12 ? `${year + 1}-01-01` : `${year}-${pad2(month + 1)}-01`,
  };
}

/** Persentase satu desimal (sebagai number), 0 bila penyebut 0. */
const pct1 = (part: number, whole: number) =>
  whole > 0 ? parseFloat(((part / whole) * 100).toFixed(1)) : 0;

function countBy<T>(rows: T[], key: (row: T) => string, amount: (row: T) => number = () => 1) {
  const out: Record<string, number> = {};
  for (const row of rows) out[key(row)] = (out[key(row)] || 0) + amount(row);
  return out;
}

export function buildHrisReport(rows: HrisReportRows, month: number, year: number) {
  const { start, end } = monthRange(month, year);
  const { employees, departments, attendance, leaves } = rows;

  // ── Headcount ──
  const active = employees.filter((e) => e.is_active);
  const totalActive = active.length;
  const newHires = employees.filter((e) => e.join_date >= start && e.join_date < end).length;
  const yearStart = `${year}-01-01`;
  const yearEnd = `${year + 1}-01-01`;
  const turnoverCount = employees.filter(
    (e) =>
      (e.employment_status === "resigned" || e.employment_status === "terminated") &&
      e.end_date &&
      e.end_date >= yearStart &&
      e.end_date < yearEnd
  ).length;
  const turnoverRate = totalActive > 0 ? pct1(turnoverCount, totalActive + turnoverCount) : 0;

  const byDepartment = departments
    .map((d) => ({ name: d.name, count: active.filter((e) => e.department_id === d.id).length }))
    .filter((d) => d.count > 0);

  // Tren headcount 6 bulan sampai bulan terpilih
  const monthlyTrend = Array.from({ length: 6 }, (_, i) => {
    const d = new Date(year, month - 1 - (5 - i), 1);
    const mStart = firstOfMonth(d);
    const mEnd = firstOfMonth(new Date(d.getFullYear(), d.getMonth() + 1, 1));
    return {
      month: d.toLocaleDateString("id-ID", { month: "short", year: "2-digit" }),
      count: employees.filter((e) => e.join_date < mEnd && (!e.end_date || e.end_date >= mStart))
        .length,
    };
  });

  // ── Absensi bulan terpilih ──
  const totalRecords = attendance.length;
  const presentCount = attendance.filter((a) => a.status === "present").length;
  const absentCount = attendance.filter((a) => a.status === "absent").length;
  const lateCount = attendance.filter((a) => a.is_late).length;
  const avgWorkHours =
    totalRecords > 0
      ? parseFloat(
          (attendance.reduce((s, a) => s + (a.work_hours || 0), 0) / totalRecords).toFixed(1)
        )
      : 0;

  const daily: Record<string, { present: number; absent: number; late: number }> = {};
  for (const a of attendance) {
    const day = (daily[a.date] ??= { present: 0, absent: 0, late: 0 });
    if (a.status === "present") day.present++;
    if (a.status === "absent") day.absent++;
    if (a.is_late) day.late++;
  }
  // Label grafik "04 Okt" (hari + bulan singkat, tanpa tahun)
  const dailyTrend = Object.entries(daily)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([date, counts]) => ({
      date: new Date(date).toLocaleDateString("id-ID", { day: "2-digit", month: "short" }),
      ...counts,
    }));

  // ── Cuti bulan terpilih ──
  const approved = leaves.filter((l) => l.status === "approved");
  const leaveByType = countBy(approved, (l) => l.leave_type, (l) => l.total_days || 0);

  return {
    period: { month, year },
    headcount: {
      total_active: totalActive,
      new_hires: newHires,
      turnover_count: turnoverCount,
      turnover_rate: turnoverRate,
      by_status: Object.entries(countBy(active, (e) => e.employment_status)).map(
        ([status, count]) => ({ status, count })
      ),
      by_department: byDepartment,
      monthly_trend: monthlyTrend,
    },
    attendance: {
      total_records: totalRecords,
      present_count: presentCount,
      absent_count: absentCount,
      late_count: lateCount,
      present_rate: pct1(presentCount, totalRecords),
      late_rate: pct1(lateCount, presentCount),
      avg_work_hours: avgWorkHours,
      daily_trend: dailyTrend,
    },
    leaves: {
      approved_count: approved.length,
      pending_count: leaves.filter((l) => l.status === "pending").length,
      total_days: approved.reduce((s, l) => s + (l.total_days || 0), 0),
      by_type: Object.entries(leaveByType)
        .map(([type, days]) => ({ type: leaveTypeShort(type), days }))
        .sort((a, b) => b.days - a.days),
    },
  };
}

export async function loadHrisReport(db: PgClient, month: number, year: number) {
  const { start, end } = monthRange(month, year);
  const [employees, departments, attendance, leaves] = await Promise.all([
    db
      .from("employees")
      .select("id, employment_status, is_active, department_id, join_date, end_date")
      .order("full_name"),
    db.from("departments").select("id, name").eq("is_active", true).order("name"),
    db
      .from("attendance")
      .select("status, is_late, work_hours, employee_id, date")
      .gte("date", start)
      .lt("date", end),
    db
      .from("leaves")
      .select("leave_type, status, total_days, employee_id")
      .gte("start_date", start)
      .lt("start_date", end),
  ]);
  return buildHrisReport(
    {
      employees: (employees.data ?? []) as ReportEmployeeRow[],
      departments: (departments.data ?? []) as ReportDepartmentRow[],
      attendance: (attendance.data ?? []) as ReportAttendanceRow[],
      leaves: (leaves.data ?? []) as ReportLeaveRow[],
    },
    month,
    year
  );
}
