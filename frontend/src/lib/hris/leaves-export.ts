/**
 * Ekspor CSV pengajuan cuti (HR). Label jenis (`leaveTypeExportLabel` di
 * katalog leave-types.ts, bukan label tampilan) dan label status khusus
 * ekspor ini dipertahankan karena berkasnya dibaca di luar aplikasi.
 */

import { z } from "zod";
import type { PgClient } from "@/lib/pg/create-client";
import { leaveTypeExportLabel } from "./leave-types";

const STATUS_LABELS: Record<string, string> = {
  pending: "Menunggu Persetujuan",
  approved: "Disetujui",
  rejected: "Ditolak",
  cancelled: "Dibatalkan",
};

const HEADERS = [
  "ID Cuti", "NIP", "Nama Karyawan", "Departemen", "Jabatan", "Jenis Cuti",
  "Tanggal Mulai", "Tanggal Selesai", "Total Hari", "Alasan", "Status",
  "Disetujui Oleh", "Tanggal Disetujui", "Alasan Penolakan", "Tanggal Pengajuan",
];

export interface LeaveExportRow {
  id: string;
  leave_type: string;
  status: string;
  start_date: string;
  end_date: string;
  total_days: number | null;
  reason: string | null;
  approved_at: string | null;
  rejection_reason: string | null;
  created_at: string;
  employee: {
    full_name: string | null;
    nip: string | null;
    department: { name: string | null } | null;
    job_title: { title: string | null } | null;
  } | null;
  approver: { full_name: string; nip: string | null } | null;
}

export const leaveExportQuerySchema = z.object({
  employee_id: z.string().optional(),
  status: z.string().optional(),
  leave_type: z.string().optional(),
  start_date: z.string().optional(),
  end_date: z.string().optional(),
});

/** Tanggal-jam WIB gaya id-ID ("4/10/2026, 14.30.00"), format lama berkas ekspor. */
const wibDateTime = (value: string) =>
  new Date(value).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" });

const csvCell = (field: unknown) => `"${String(field).replace(/"/g, '""')}"`;

export function leavesToCsv(rows: LeaveExportRow[]): string {
  const lines = rows.map((r) =>
    [
      r.id,
      r.employee?.nip || "-",
      r.employee?.full_name || "-",
      r.employee?.department?.name || "-",
      r.employee?.job_title?.title || "-",
      leaveTypeExportLabel(r.leave_type),
      r.start_date,
      r.end_date,
      r.total_days || 0,
      r.reason || "-",
      STATUS_LABELS[r.status] || r.status,
      r.approver ? `${r.approver.full_name} (${r.approver.nip})` : "-",
      r.approved_at ? wibDateTime(r.approved_at) : "-",
      r.rejection_reason || "-",
      wibDateTime(r.created_at),
    ]
      .map(csvCell)
      .join(",")
  );
  return [HEADERS.join(","), ...lines].join("\n");
}

export async function loadLeavesForExport(
  db: PgClient,
  q: z.infer<typeof leaveExportQuerySchema>
): Promise<LeaveExportRow[]> {
  let query = db.from("leaves").select(`
    *,
    employee:employees!employee_id(
      full_name, nip, department:departments(name), job_title:positions(title)
    ),
    approver:employees!leaves_approved_by_fkey( full_name, nip )
  `);
  if (q.employee_id) query = query.eq("employee_id", q.employee_id);
  if (q.status) query = query.eq("status", q.status);
  if (q.leave_type) query = query.eq("leave_type", q.leave_type);
  if (q.start_date && q.end_date) {
    query = query.gte("start_date", q.start_date).lte("end_date", q.end_date);
  }
  const { data, error } = await query.order("created_at", { ascending: false });
  if (error) throw new Error(error.message);
  return (data ?? []) as LeaveExportRow[];
}
