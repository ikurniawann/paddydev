"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { ATTENDANCE_REQUEST_KIND_LABEL, attendanceRequestClocks } from "@/lib/hris/attendance-requests";
import { shortDateId } from "@/lib/hris/placements";
import { attendanceRequestAttachmentUrl } from "../api";
import { useCancelAttendanceRequest } from "../mutations";
import { useMyAttendanceRequests } from "../queries";
import type { AttendanceRequest } from "../types";
import { AttendanceRequestStatusBadge } from "./attendance-request-status-badge";

/** Pengajuan absensi milik karyawan beserta status dan alasan penolakan; batal (dengan konfirmasi) selama menunggu. */
export function MyAttendanceRequests() {
  const rows = useMyAttendanceRequests().data ?? [];
  const cancel = useCancelAttendanceRequest();
  const [toCancel, setToCancel] = useState<AttendanceRequest | null>(null);

  function confirmCancel() {
    if (!toCancel) return;
    cancel.mutate(toCancel.id, {
      onSuccess: (json) => toast.success(json.message || "Pengajuan absensi dibatalkan"),
      onError: (e) => toast.error(e.message || "Gagal membatalkan pengajuan"),
      onSettled: () => setToCancel(null),
    });
  }

  if (rows.length === 0) return null;
  return (
    <Card>
      <CardContent className="min-w-0 p-0">
        <p className="px-4 pt-4 text-sm font-semibold text-gray-900">Pengajuan absensi saya</p>
        <ul className="divide-y divide-gray-100">
          {rows.map((r) => (
            <li key={r.id} className="flex flex-wrap items-start justify-between gap-2 px-4 py-3 text-sm">
              <div className="min-w-0 flex-1 basis-48">
                <p className="font-medium text-gray-900">
                  {shortDateId(r.date)} · {ATTENDANCE_REQUEST_KIND_LABEL[r.kind]}
                </p>
                <p className="break-words text-xs text-gray-500">
                  {attendanceRequestClocks(r)} — {r.reason}
                </p>
                {r.attachment_path && (
                  <a
                    href={attendanceRequestAttachmentUrl(r.attachment_path)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-xs text-blue-600 underline"
                  >
                    Lihat foto bukti
                  </a>
                )}
                {r.rejection_reason && (
                  <p className="break-words text-xs text-red-600">Alasan ditolak: {r.rejection_reason}</p>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <AttendanceRequestStatusBadge status={r.status} />
                {r.status === "pending" && (
                  <Button size="sm" variant="ghost" disabled={cancel.isPending} onClick={() => setToCancel(r)}>
                    Batalkan
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      </CardContent>
      <ConfirmDialog
        open={toCancel !== null}
        onOpenChange={(open) => !open && setToCancel(null)}
        title="Batalkan pengajuan absensi?"
        description={
          toCancel
            ? `Pengajuan ${ATTENDANCE_REQUEST_KIND_LABEL[toCancel.kind].toLowerCase()} tanggal ${shortDateId(toCancel.date)} akan dibatalkan dan tidak diproses HR.`
            : undefined
        }
        confirmLabel="Ya, batalkan"
        cancelLabel="Kembali"
        loading={cancel.isPending}
        onConfirm={confirmCancel}
      />
    </Card>
  );
}
