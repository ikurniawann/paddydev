import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { formatDate } from "@/lib/format";
import {
  OVERTIME_DAY_TYPE_OPTIONS,
  overtimeDayTypeLabel,
  type OvertimeDecisionAction,
} from "@/lib/hris/overtime-rules";
import { hhmm } from "@/lib/hris/shifts-view";
import { useState } from "react";
import type { OvertimeDayType, OvertimeRow } from "../types";
import { OvertimeAttachmentButtons } from "@/components/hris/overtime-attachment-preview";

const STATUS_BADGES: Record<string, { label: string; className: string }> = {
  pending: { label: "Menunggu", className: "bg-amber-100 text-amber-700" },
  approved: { label: "Disetujui", className: "bg-green-100 text-green-700" },
  rejected: { label: "Ditolak", className: "bg-red-100 text-red-700" },
  cancelled: { label: "Dibatalkan", className: "bg-gray-100 text-gray-600" },
};

interface OvertimeTableProps {
  rows: OvertimeRow[];
  loading: boolean;
  decidingId: string | null;
  onDecide: (id: string, action: OvertimeDecisionAction, dayType?: OvertimeDayType) => void;
  canCorrect: boolean;
  /** Pemegang aksi `approve` daun Lembur (EPIC-067 R14): hanya mereka melihat Setujui/Tolak, Batalkan penugasan perusahaan, dan boleh mengganti jenis hari (server menetapkan day_type hanya untuk mereka, R11). */
  canDecide: boolean;
  onCorrect: (row: OvertimeRow) => void;
}

export function OvertimeTable({ rows, loading, decidingId, onDecide, canCorrect, canDecide, onCorrect }: OvertimeTableProps) {
  return (
    <div className="overflow-x-auto rounded-xl border border-gray-200/70 bg-white shadow-sm">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b bg-gray-50 text-left text-xs uppercase tracking-wide text-gray-500">
            <th className="px-4 py-3">Karyawan</th>
            <th className="px-4 py-3">Tanggal</th>
            <th className="px-4 py-3">Jam</th>
            <th className="px-4 py-3">Durasi</th>
            <th className="px-4 py-3">Sumber</th>
            <th className="px-4 py-3">Alasan</th>
            <th className="px-4 py-3">Status</th>
            <th className="px-4 py-3 text-right">Aksi</th>
          </tr>
        </thead>
        <tbody>
          {loading ? (
            <tr>
              <td colSpan={8} className="py-10 text-center text-gray-400">
                <Loader2 className="mx-auto h-6 w-6 animate-spin" />
              </td>
            </tr>
          ) : rows.length === 0 ? (
            <tr>
              <td colSpan={8} className="py-10 text-center text-gray-400">
                Tidak ada pengajuan lembur pada filter ini.
              </td>
            </tr>
          ) : (
            rows.map((row) => (
              <OvertimeTableRow
                key={`${row.id}:${row.day_type ?? "regular"}`}
                row={row}
                deciding={decidingId === row.id}
                onDecide={onDecide}
                canCorrect={canCorrect}
                canDecide={canDecide}
                onCorrect={onCorrect}
              />
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

interface OvertimeTableRowProps {
  row: OvertimeRow;
  deciding: boolean;
  onDecide: (id: string, action: OvertimeDecisionAction, dayType?: OvertimeDayType) => void;
  canCorrect: boolean;
  canDecide: boolean;
  onCorrect: (row: OvertimeRow) => void;
}

function OvertimeTableRow({ row, deciding, onDecide, canCorrect, canDecide, onCorrect }: OvertimeTableRowProps) {
  const badge = STATUS_BADGES[row.status] ?? STATUS_BADGES.pending;
  const pending = row.status === "pending";
  // Koreksi jam lembur karyawan yang punya atasan langsung (aktif) adalah hak atasan (EPIC-066);
  // atasan nonaktif dihitung tanpa atasan (EPIC-063 R9).
  const managerOwnsCorrection =
    row.source === "employee" &&
    !!row.employee?.reporting_to &&
    row.employee.reporting_to !== row.employee_id &&
    row.employee.reporting_to_active === true;
  const [dayType, setDayType] = useState<OvertimeDayType>(row.day_type ?? "regular");
  return (
    <tr className="border-b last:border-0 hover:bg-gray-50/60">
      <td className="px-4 py-3 font-medium text-gray-900">{row.employee?.full_name ?? "—"}</td>
      <td className="px-4 py-3">
        {formatDate(row.date, "—")}
        {row.day_type === "rest_day" && (
          <span className="ml-1 rounded-full bg-violet-100 px-2 py-0.5 text-xs font-medium text-violet-700">
            {overtimeDayTypeLabel(row.day_type)}
          </span>
        )}
        {row.corrected_at && (
          <span
            className="ml-1 rounded-full bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700"
            title={`Dikoreksi ${row.corrected_by_name ?? "HR"}`}
          >
            Dikoreksi
          </span>
        )}
      </td>
      <td className="px-4 py-3">
        {hhmm(row.start_time)}–{hhmm(row.end_time)}
      </td>
      <td className="px-4 py-3">
        {Number(row.hours)} jam
        {row.source === "employee" && (
          <p className="text-xs text-gray-500">istirahat {row.break_minutes ?? 0} menit</p>
        )}
        <OvertimeAttachmentButtons paths={row.attachments ?? []} className="mt-1" />
      </td>
      <td className="px-4 py-3">
        {row.source === "company" ? (
          <span className="rounded-full bg-sky-100 px-2 py-0.5 text-xs font-medium text-sky-700">
            Perusahaan
          </span>
        ) : (
          <span className="rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600">
            Karyawan
          </span>
        )}
      </td>
      <td className="max-w-[220px] truncate px-4 py-3 text-gray-600" title={row.reason ?? ""}>
        {row.reason || "—"}
        {row.status === "rejected" && row.rejection_reason ? ` · Ditolak: ${row.rejection_reason}` : ""}
      </td>
      <td className="px-4 py-3">
        <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}>{badge.label}</span>
        {row.decided_as === "hr" && <span className="ml-1 text-xs text-gray-500">oleh HR</span>}
      </td>
      <td className="px-4 py-3 text-right">
        {pending && row.source === "employee" && canDecide && (
          <div className="flex justify-end gap-2">
            <select
              aria-label="Jenis hari"
              value={dayType}
              onChange={(e) => setDayType(e.target.value as OvertimeDayType)}
              className="rounded-md border border-gray-200 px-2 text-xs"
              disabled={deciding}
            >
              {OVERTIME_DAY_TYPE_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            <Button size="sm" disabled={deciding} onClick={() =>
                // Hanya kirim jenis bila pemegang approve mengubahnya di sini; jangan menimpa koreksi/nilai server.
                onDecide(row.id, "approve", dayType !== (row.day_type ?? "regular") ? dayType : undefined)
              }>
              Setujui
            </Button>
            <Button size="sm" variant="outline" disabled={deciding} onClick={() => onDecide(row.id, "reject")}>
              Tolak
            </Button>
          </div>
        )}
        {pending && row.source === "company" && (
          <div className="flex items-center justify-end gap-2">
            <span className="text-xs text-gray-400">Menunggu konfirmasi karyawan</span>
            {/* Pembatalan oleh HR butuh approve di Lembur (CanDecideOvertime, EPIC-067 R11). */}
            {canDecide && (
              <Button
                size="sm"
                variant="ghost"
                className="text-xs text-gray-500"
                disabled={deciding}
                onClick={() => onDecide(row.id, "cancel")}
              >
                Batalkan
              </Button>
            )}
          </div>
        )}
        {canCorrect && !managerOwnsCorrection && (row.status === "pending" || row.status === "approved") && (
          <Button size="sm" variant="ghost" className="ml-2 text-xs" onClick={() => onCorrect(row)}>
            Koreksi
          </Button>
        )}
      </td>
    </tr>
  );
}
