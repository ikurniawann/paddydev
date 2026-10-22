"use client";

import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";
import { BanknotesIcon, CalendarDaysIcon, ClockIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Textarea } from "@/components/ui/textarea";
import { formatDate } from "@/lib/format";
import { leaveTypeLabel } from "@/lib/hris/leave-types";
import { overtimeDayTypeLabel, overtimeTimeChanges } from "@/lib/hris/overtime-rules";
import { hhmm } from "@/lib/hris/shifts-view";
import { TeamTripApprovals, useBusinessTrips } from "@/features/hris/business-trips";
import { EssLoading, EssNotLinked } from "@/features/hris/ess/components/ess-states";
import { useEssMe, useTeamLoanApprovals } from "@/features/hris/ess/queries";
import { useDecideTeamRequest } from "../mutations";
import { useTeamApprovals } from "../queries";
import type { TeamDecision, TeamOvertimeRequest, TeamRequestKind } from "../types";
import { OvertimeApproveDialog, type OvertimeApproveTimes } from "./overtime-approve-dialog";

const DONE: Record<TeamRequestKind, Record<TeamDecision, string>> = {
  leave: { approve: "Cuti disetujui", reject: "Cuti ditolak" },
  overtime: { approve: "Lembur disetujui", reject: "Lembur ditolak" },
};

/** Lampiran cuti disajikan privat di /api/hris/leaves/attachment/<path> (atasan langsung boleh membacanya). */
function leaveAttachmentHref(path: string) {
  return `/api/hris/leaves/attachment/${path.split("/").map(encodeURIComponent).join("/")}`;
}

const NOUN: Record<TeamRequestKind, string> = { leave: "cuti", overtime: "lembur" };

interface Rejecting {
  kind: TeamRequestKind;
  id: string;
  name: string;
}

/**
 * ESS → Persetujuan Tim (/dashboard/me/persetujuan-tim, EPIC-063): antrean
 * atasan langsung untuk cuti, lembur, dan Perjalanan Dinas bawahan langsung.
 * Kasbon tetap diputus di Pinjaman (dua tahap). Anggota tim (reporting_to)
 * dan larangan memutus pengajuan sendiri ditegakkan server; HR tetap bisa
 * memutus sebagai cadangan.
 */
export function EssPersetujuanTimPage() {
  const { data: me, isLoading } = useEssMe();
  const queue = useTeamApprovals();
  const tripsQuery = useBusinessTrips({ scope: "approvals", status: "pending" });
  const loansQuery = useTeamLoanApprovals();
  const trips = tripsQuery.data ?? [];
  const loans = loansQuery.data ?? [];
  const decide = useDecideTeamRequest();
  const [rejecting, setRejecting] = useState<Rejecting | null>(null);
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [approving, setApproving] = useState<TeamOvertimeRequest | null>(null);
  const busyId = decide.isPending ? decide.variables?.id : null;

  function run(
    kind: TeamRequestKind,
    id: string,
    action: TeamDecision,
    rejectionReason?: string,
    times?: Partial<OvertimeApproveTimes>
  ) {
    decide.mutate(
      { kind, id, action, rejectionReason, times },
      {
        onSuccess: () => toast.success(DONE[kind][action]),
        onError: (e) => toast.error(e.message || "Gagal memproses"),
      }
    );
  }

  function askReject(kind: TeamRequestKind, id: string, name: string) {
    setReason("");
    setRejecting({ kind, id, name });
    setOpen(true);
  }

  function confirmReject() {
    if (!rejecting) return;
    const text = reason.trim();
    if (!text) {
      toast.error("Alasan penolakan wajib diisi");
      return;
    }
    setOpen(false);
    run(rejecting.kind, rejecting.id, "reject", text);
  }

  // Setujui lembur membuka dialog koreksi; hanya kolom yang berubah dikirim (server
  // memeriksa kewenangan koreksi sebelum membandingkan).
  function approveOvertime(row: TeamOvertimeRequest, times: OvertimeApproveTimes) {
    setApproving(null);
    run("overtime", row.id, "approve", undefined, overtimeTimeChanges(row, times));
  }

  function actions(kind: TeamRequestKind, id: string, name: string, onApprove?: () => void) {
    return (
      <div className="flex shrink-0 gap-2">
        <Button size="sm" aria-label={`Setujui ${NOUN[kind]} ${name}`} disabled={busyId === id}
          onClick={onApprove ?? (() => run(kind, id, "approve"))}>
          Setujui
        </Button>
        <Button size="sm" variant="outline" aria-label={`Tolak ${NOUN[kind]} ${name}`} disabled={busyId === id}
          onClick={() => askReject(kind, id, name)}>
          Tolak
        </Button>
      </div>
    );
  }

  if (isLoading) return <EssLoading />;
  if (!me?.employee) return <EssNotLinked feature="Persetujuan Tim" />;

  const leaves = queue.data?.leaves ?? [];
  const overtime = queue.data?.overtime ?? [];
  // The empty state only shows once every queue loaded successfully and is empty;
  // a failed load must not read as "nothing to approve".
  const loadFailed = queue.isError || tripsQuery.isError || loansQuery.isError;
  const loading = queue.isLoading || tripsQuery.isLoading || loansQuery.isLoading;
  const empty = leaves.length + overtime.length + trips.length + loans.length === 0;

  return (
    <div className="space-y-6">
      <div className="border-b border-gray-200/70 pb-4">
        <h1 className="text-2xl font-bold text-gray-900">Persetujuan Tim</h1>
        <p className="text-sm text-gray-500">
          Pengajuan anggota tim yang atasan langsungnya Anda. HR tetap bisa memutus sebagai cadangan.
        </p>
      </div>

      {loadFailed && (
        <div role="alert" className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 shadow-sm">
          Gagal memuat antrean persetujuan. Coba muat ulang.
        </div>
      )}

      {empty && !loading && !loadFailed && (
        <div className="rounded-xl border border-gray-200/70 bg-white p-6 text-center text-sm text-gray-500 shadow-sm">
          Tidak ada pengajuan tim yang menunggu persetujuan Anda.
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
                  <p className="truncate text-xs text-gray-600">
                    {formatDate(row.start_date, "—")}
                    {row.end_date !== row.start_date ? ` s.d. ${formatDate(row.end_date, "—")}` : ""} (
                    {Number(row.total_days)} hari) · {row.reason || "—"}
                  </p>
                </div>
                <div className="flex shrink-0 flex-wrap items-center gap-3">
                  {row.attachment_url && (
                    <a
                      href={leaveAttachmentHref(row.attachment_url)}
                      target="_blank"
                      rel="noopener noreferrer"
                      aria-label={`Lihat lampiran cuti ${row.employee.full_name}`}
                      className="text-xs font-medium text-pink-600 hover:underline"
                    >
                      Lihat lampiran
                    </a>
                  )}
                  {actions("leave", row.id, row.employee.full_name)}
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
                  <p className="truncate text-xs text-gray-600">
                    {hhmm(row.start_time)}–{hhmm(row.end_time)} ({String(Number(row.hours)).replace(".", ",")} jam) ·{" "}
                    {overtimeDayTypeLabel(row.day_type)} · istirahat {row.break_minutes ?? 0} menit ·{" "}
                    {row.attachments?.length ?? 0} dokumentasi · {row.reason || "—"}
                  </p>
                </div>
                {actions("overtime", row.id, row.employee.full_name, () => setApproving(row))}
              </li>
            ))}
          </ul>
        </section>
      )}

      <TeamTripApprovals />

      {loans.length > 0 && (
        <Link
          href="/dashboard/me/pinjaman"
          className="flex items-center justify-between gap-3 rounded-xl border border-amber-200 bg-amber-50/70 p-4 text-sm text-amber-900 shadow-sm hover:bg-amber-50"
        >
          <span className="flex items-center gap-2">
            <BanknotesIcon className="h-4 w-4" /> {loans.length} kasbon tim menunggu persetujuan Anda
          </span>
          <span className="font-medium">Buka Pinjaman →</span>
        </Link>
      )}

      {approving && (
        <OvertimeApproveDialog
          key={approving.id}
          row={{
            id: approving.id,
            date: approving.date,
            start_time: approving.start_time,
            end_time: approving.end_time,
            break_minutes: approving.break_minutes ?? 0,
            attachments: approving.attachments ?? [],
            employee_name: approving.employee.full_name,
          }}
          open
          onOpenChange={(o) => !o && setApproving(null)}
          onApprove={(times) => approveOvertime(approving, times)}
        />
      )}

      <ConfirmDialog
        open={open}
        onOpenChange={setOpen}
        title={`Tolak ${rejecting ? NOUN[rejecting.kind] : "pengajuan"} ${rejecting?.name ?? ""}?`}
        confirmLabel="Tolak pengajuan"
        cancelLabel="Kembali"
        onConfirm={confirmReject}
      >
        <div className="space-y-1.5">
          <label htmlFor="team-rejection-reason" className="block text-xs font-medium text-gray-600">
            Alasan penolakan
          </label>
          <Textarea
            id="team-rejection-reason"
            rows={3}
            maxLength={500}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
      </ConfirmDialog>
    </div>
  );
}
