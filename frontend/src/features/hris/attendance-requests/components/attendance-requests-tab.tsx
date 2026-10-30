"use client";

import { useState } from "react";
import { AttachmentPreviewButtons } from "@/components/hris/attachment-preview";
import { RequestDetailDialog } from "@/components/hris/request-detail-dialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useEssMe } from "@/features/hris/ess/queries";
import { formatDateTime, formatTime } from "@/lib/format";
import {
  ATTENDANCE_REQUEST_KIND_LABEL,
  ATTENDANCE_REQUEST_STATUS_META,
  attendanceRequestClocks,
  type AttendanceRequestStatus,
} from "@/lib/hris/attendance-requests";
import { shortDateId } from "@/lib/hris/placements";
import { attendanceRequestAttachmentUrl } from "../api";
import { useAttendanceRequests } from "../queries";
import { useAttendanceRequestDecision } from "../use-attendance-request-decision";
import { AttendanceRequestStatusBadge } from "./attendance-request-status-badge";

const ALL = "all";

/**
 * Absensi HRD → tab Pengajuan Absensi (EPIC-065, MOM AB-04/AB-06): pengajuan absen
 * manual/koreksi karyawan. HRD menyetujui (absensi ditulis lewat jalur penyesuaian
 * HR yang sama: alasan, riwayat penyesuaian, audit) atau menolak dengan alasan.
 * Pengajuan HRD sendiri tidak bisa ia putus (server juga menolak).
 */
export function AttendanceRequestsTab() {
  const [status, setStatus] = useState<string>("pending");
  const requests = useAttendanceRequests(status === ALL ? {} : { status: status as AttendanceRequestStatus });
  const myEmployeeId = useEssMe().data?.employee?.id;
  const { request, busyId, dialog } = useAttendanceRequestDecision();
  const rows = requests.data ?? [];
  // Detail per id (EPIC-072) agar status mengikuti data terbaru setelah keputusan.
  const [detailId, setDetailId] = useState<string | null>(null);
  const detail = rows.find((r) => r.id === detailId);
  const recordedText = (r: (typeof rows)[number]) =>
    r.recorded ? `${formatTime(r.recorded.clock_in, "—")} – ${formatTime(r.recorded.clock_out, "—")}` : "Belum ada absen";
  const decidable = (r: (typeof rows)[number]) => r.status === "pending" && r.employee_id !== myEmployeeId;

  return (
    <div className="min-w-0 space-y-4">
      <div className="w-48">
        <label className="mb-1 block text-xs font-medium text-gray-600">Status</label>
        <Select value={status} onValueChange={(v) => v && setStatus(v)}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {(Object.keys(ATTENDANCE_REQUEST_STATUS_META) as AttendanceRequestStatus[]).map((s) => (
              <SelectItem key={s} value={s}>
                {ATTENDANCE_REQUEST_STATUS_META[s].label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Card className="min-w-0">
        <CardContent className="p-0">
          {requests.isError ? (
            <p className="p-6 text-center text-sm text-red-600">Gagal memuat pengajuan absensi.</p>
          ) : rows.length === 0 ? (
            <p className="p-6 text-center text-sm text-gray-500">Tidak ada pengajuan absensi untuk filter ini.</p>
          ) : (
            <div className="min-w-0 overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-gray-600">
                    <th className="p-3">Karyawan</th>
                    <th className="p-3">Tanggal</th>
                    <th className="p-3">Diajukan</th>
                    <th className="p-3">Tercatat</th>
                    <th className="p-3">Keterangan</th>
                    <th className="p-3">Status</th>
                    <th className="p-3 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.id} className="border-b border-gray-50 align-top">
                      <td className="p-3">
                        <p className="font-medium text-gray-900">{r.employee?.full_name ?? "—"}</p>
                        {r.employee?.nip && <p className="text-xs text-gray-400">{r.employee.nip}</p>}
                      </td>
                      <td className="whitespace-nowrap p-3 text-gray-700">{shortDateId(r.date)}</td>
                      <td className="p-3 text-gray-700">
                        <p className="text-xs text-gray-500">{ATTENDANCE_REQUEST_KIND_LABEL[r.kind]}</p>
                        <p>{attendanceRequestClocks(r)}</p>
                      </td>
                      <td className="whitespace-nowrap p-3 text-gray-700">{recordedText(r)}</td>
                      <td className="p-3 text-gray-700">
                        <p className="max-w-xs whitespace-pre-line break-words">{r.reason}</p>
                        {r.attachment_path && (
                          <AttachmentPreviewButtons
                            paths={[r.attachment_path]}
                            hrefFor={attendanceRequestAttachmentUrl}
                            label="Foto bukti"
                            context={r.employee?.full_name ?? undefined}
                            buttonClassName="text-xs text-blue-600 underline"
                          />
                        )}
                      </td>
                      <td className="p-3">
                        <AttendanceRequestStatusBadge status={r.status} />
                        {r.decided_by_name && <p className="mt-1 text-xs text-gray-400">oleh {r.decided_by_name}</p>}
                        {r.rejection_reason && <p className="text-xs text-red-600">{r.rejection_reason}</p>}
                      </td>
                      <td className="whitespace-nowrap p-3 text-right">
                        <Button
                          size="sm"
                          variant="ghost"
                          aria-label={`Detail pengajuan absensi ${r.employee?.full_name ?? ""}`}
                          onClick={() => setDetailId(r.id)}
                        >
                          Detail
                        </Button>{" "}
                        {decidable(r) && (
                          <>
                            <Button size="sm" disabled={busyId === r.id} onClick={() => request(r, "approve")}>
                              Setujui
                            </Button>{" "}
                            <Button
                              size="sm"
                              variant="outline"
                              disabled={busyId === r.id}
                              onClick={() => request(r, "reject")}
                            >
                              Tolak
                            </Button>
                          </>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
      {detail && (
        <RequestDetailDialog
          open
          onOpenChange={(o) => !o && setDetailId(null)}
          title="Detail pengajuan absensi"
          employee={{ name: detail.employee?.full_name ?? "—", nip: detail.employee?.nip }}
          status={<AttendanceRequestStatusBadge status={detail.status} />}
          fields={[
            { label: "Tanggal", value: shortDateId(detail.date) },
            { label: "Jenis", value: ATTENDANCE_REQUEST_KIND_LABEL[detail.kind] },
            { label: "Diajukan", value: attendanceRequestClocks(detail) },
            { label: "Tercatat", value: recordedText(detail) },
            { label: "Dikirim", value: formatDateTime(detail.created_at, "—") },
          ]}
          reason={{ label: "Keterangan", text: detail.reason }}
          attachments={{
            paths: detail.attachment_path ? [detail.attachment_path] : [],
            hrefFor: attendanceRequestAttachmentUrl,
            label: "Foto bukti",
          }}
          notes={
            detail.decided_by_name || detail.rejection_reason ? (
              <div className="space-y-1 text-xs text-gray-600">
                {detail.decided_by_name && <p>Diputus oleh {detail.decided_by_name}</p>}
                {detail.rejection_reason && (
                  <p className="whitespace-pre-line break-words text-red-600">Alasan penolakan: {detail.rejection_reason}</p>
                )}
              </div>
            ) : undefined
          }
          actions={
            decidable(detail) ? (
              <>
                <Button variant="outline" disabled={busyId === detail.id} onClick={() => {
                  setDetailId(null);
                  request(detail, "reject");
                }}>
                  Tolak
                </Button>
                <Button disabled={busyId === detail.id} onClick={() => {
                  setDetailId(null);
                  request(detail, "approve");
                }}>
                  Setujui
                </Button>
              </>
            ) : undefined
          }
        />
      )}
      {dialog}
    </div>
  );
}
