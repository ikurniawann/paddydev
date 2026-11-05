"use client";

import { useState } from "react";
import { CalendarDaysIcon, ClockIcon } from "@heroicons/react/24/outline";
import { RequestDetailDialog } from "@/components/hris/request-detail-dialog";
import { Button } from "@/components/ui/button";
import { formatDate, formatDateTime } from "@/lib/format";
import { leaveAttachmentHref } from "@/lib/hris/attachments";
import { leaveTypeLabel } from "@/lib/hris/leave-types";
import { overtimeAttachmentHref, overtimeDayTypeLabel, paidOvertimeHours } from "@/lib/hris/overtime-rules";
import { hhmm } from "@/lib/hris/shifts-view";
import { useTeamHistory } from "../queries";
import type { TeamHistoryLeave, TeamHistoryOvertime } from "../types";

/**
 * Riwayat Persetujuan Tim (EPIC-090): cuti/izin dan lembur bawahan langsung
 * semua status selama 2 periode absensi, dengan Detail baca-saja, supaya
 * atasan tetap bisa melihat pengajuan setelah memutusnya.
 */

type Badge = { label: string; className: string };
const BADGE = {
  waiting: "bg-amber-100 text-amber-800",
  hr: "bg-sky-100 text-sky-800",
  returned: "bg-orange-100 text-orange-800",
  approved: "bg-green-100 text-green-800",
  rejected: "bg-red-100 text-red-700",
  cancelled: "bg-gray-100 text-gray-600",
};

function decidedBadge(status: string): Badge | null {
  if (status === "approved") return { label: "Disetujui", className: BADGE.approved };
  if (status === "rejected") return { label: "Ditolak", className: BADGE.rejected };
  if (status === "cancelled") return { label: "Dibatalkan", className: BADGE.cancelled };
  return null;
}

function leaveBadge(row: TeamHistoryLeave): Badge {
  const decided = decidedBadge(row.status);
  if (decided) return decided;
  if (row.attachment_review === "awaiting") return { label: "Menunggu verifikasi HR", className: BADGE.hr };
  if (row.attachment_review === "returned") return { label: "Lampiran dikembalikan", className: BADGE.returned };
  return { label: "Menunggu atasan", className: BADGE.waiting };
}

function overtimeBadge(row: TeamHistoryOvertime): Badge {
  const decided = decidedBadge(row.status);
  if (decided) return decided;
  if (row.source === "company") return { label: "Menunggu konfirmasi karyawan", className: BADGE.waiting };
  return { label: "Menunggu atasan", className: BADGE.waiting };
}

function StatusBadge({ badge }: { badge: Badge }) {
  return <span className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${badge.className}`}>{badge.label}</span>;
}

function leaveNotes(row: TeamHistoryLeave): string[] {
  const notes: string[] = [];
  if (row.status === "approved" && row.decided_by_name) notes.push(`Disetujui oleh ${row.decided_by_name}`);
  if (row.status === "rejected") {
    notes.push(`Ditolak${row.decided_by_name ? ` oleh ${row.decided_by_name}` : ""}: ${row.rejection_reason ?? "—"}`);
  }
  if (row.status === "pending" && row.manager_approved_name) {
    notes.push(`Disetujui atasan: ${row.manager_approved_name}`);
  }
  if (row.status === "pending" && row.attachment_review === "returned") {
    notes.push(`Lampiran dikembalikan HR: ${row.attachment_return_reason ?? "—"}`);
  }
  if (row.attachment_review === "verified" && row.attachment_reviewed_name) {
    notes.push(`Lampiran diverifikasi HR: ${row.attachment_reviewed_name}`);
  }
  return notes;
}

function overtimeNotes(row: TeamHistoryOvertime): string[] {
  if (row.status === "approved" && row.decided_by_name) return [`Disetujui oleh ${row.decided_by_name}`];
  if (row.status === "rejected") {
    return [`Ditolak${row.decided_by_name ? ` oleh ${row.decided_by_name}` : ""}: ${row.rejection_reason ?? "—"}`];
  }
  return [];
}

const leavePeriod = (row: TeamHistoryLeave) =>
  `${formatDate(row.start_date, "—")}${row.end_date !== row.start_date ? ` s.d. ${formatDate(row.end_date, "—")}` : ""}`;
const overtimeClock = (row: TeamHistoryOvertime) =>
  `${hhmm(row.start_time)}–${hhmm(row.end_time)} (${String(Number(row.hours)).replace(".", ",")} jam)`;

type Shown = { kind: "leave"; row: TeamHistoryLeave } | { kind: "overtime"; row: TeamHistoryOvertime };

export function TeamHistoryPanel() {
  const history = useTeamHistory();
  const [shown, setShown] = useState<Shown | null>(null);

  if (history.isError) {
    return (
      <div role="alert" className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 shadow-sm">
        Gagal memuat riwayat tim. Coba muat ulang.
      </div>
    );
  }
  if (history.isLoading || !history.data) {
    return <p className="py-6 text-center text-sm text-gray-500">Memuat riwayat…</p>;
  }
  const { period, leaves, overtime } = history.data;

  return (
    <div className="space-y-6">
      <p className="text-sm text-gray-600">
        Cuti dan lembur anggota tim sejak <span className="font-medium text-gray-800">{formatDate(period.from, "—")}</span>{" "}
        (awal periode lalu), termasuk yang tanggalnya ke depan, semua status.
      </p>

      {leaves.length + overtime.length === 0 && (
        <div className="rounded-xl border border-gray-200/70 bg-white p-6 text-center text-sm text-gray-500 shadow-sm">
          Belum ada cuti atau lembur tim sejak awal periode lalu.
        </div>
      )}

      {leaves.length > 0 && (
        <section className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-800">
            <CalendarDaysIcon className="h-4 w-4 text-pink-500" /> Cuti & Izin ({leaves.length})
          </h3>
          <ul className="mt-3 divide-y divide-gray-100">
            {leaves.map((row) => (
              <li key={row.id} className="flex flex-wrap items-center justify-between gap-3 py-2.5">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">
                    {row.employee.full_name} · {leaveTypeLabel(row.leave_type)}
                  </p>
                  <p className="text-xs text-gray-600">
                    {leavePeriod(row)} ({Number(row.total_days)} hari)
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <StatusBadge badge={leaveBadge(row)} />
                  <Button size="sm" variant="ghost" aria-label={`Detail cuti ${row.employee.full_name}`}
                    onClick={() => setShown({ kind: "leave", row })}>
                    Detail
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}

      {overtime.length > 0 && (
        <section className="rounded-xl border border-gray-200/70 bg-white p-5 shadow-sm">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-gray-800">
            <ClockIcon className="h-4 w-4 text-pink-500" /> Lembur ({overtime.length})
          </h3>
          <ul className="mt-3 divide-y divide-gray-100">
            {overtime.map((row) => (
              <li key={row.id} className="flex flex-wrap items-center justify-between gap-3 py-2.5">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">
                    {row.employee.full_name} · {formatDate(row.date, "—")}
                  </p>
                  <p className="text-xs text-gray-600">
                    {overtimeClock(row)} · {overtimeDayTypeLabel(row.day_type)}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <StatusBadge badge={overtimeBadge(row)} />
                  <Button size="sm" variant="ghost" aria-label={`Detail lembur ${row.employee.full_name}`}
                    onClick={() => setShown({ kind: "overtime", row })}>
                    Detail
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}

      {shown && (
        <RequestDetailDialog
          open
          onOpenChange={(o) => !o && setShown(null)}
          title={shown.kind === "leave" ? "Detail cuti" : "Detail lembur"}
          employee={{ name: shown.row.employee.full_name, nip: shown.row.employee.nip }}
          status={<StatusBadge badge={shown.kind === "leave" ? leaveBadge(shown.row) : overtimeBadge(shown.row)} />}
          fields={
            shown.kind === "leave"
              ? [
                  { label: "Jenis", value: leaveTypeLabel(shown.row.leave_type) },
                  { label: "Periode", value: leavePeriod(shown.row) },
                  { label: "Jumlah", value: `${Number(shown.row.total_days)} hari` },
                  { label: "Diajukan", value: formatDateTime(shown.row.created_at, "—") },
                ]
              : [
                  { label: "Tanggal", value: formatDate(shown.row.date, "—") },
                  { label: "Jam", value: `${hhmm(shown.row.start_time)}–${hhmm(shown.row.end_time)}` },
                  { label: "Jenis hari", value: overtimeDayTypeLabel(shown.row.day_type) },
                  { label: "Istirahat", value: `${shown.row.break_minutes ?? 0} menit` },
                  {
                    label: "Jam dibayar",
                    value: `${paidOvertimeHours(hhmm(shown.row.start_time), hhmm(shown.row.end_time), shown.row.break_minutes ?? 0).toLocaleString("id-ID")} jam`,
                  },
                  { label: "Sumber", value: shown.row.source === "company" ? "Penugasan HR" : "Diajukan karyawan" },
                  { label: "Diajukan", value: formatDateTime(shown.row.created_at, "—") },
                ]
          }
          reason={{ text: shown.row.reason }}
          attachments={
            shown.kind === "leave"
              ? { paths: shown.row.attachment_url ? [shown.row.attachment_url] : [], hrefFor: leaveAttachmentHref, label: "Lampiran" }
              : { paths: shown.row.attachments ?? [], hrefFor: overtimeAttachmentHref }
          }
          notes={
            (() => {
              const notes = shown.kind === "leave" ? leaveNotes(shown.row) : overtimeNotes(shown.row);
              return notes.length > 0 ? (
                <div className="space-y-0.5 text-xs text-gray-700">
                  {notes.map((n) => (
                    <p key={n} className="whitespace-pre-line break-words">{n}</p>
                  ))}
                </div>
              ) : undefined;
            })()
          }
        />
      )}
    </div>
  );
}
