"use client";

import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Textarea } from "@/components/ui/textarea";
import { useDecideAttendanceRequest } from "./mutations";
import type { AttendanceRequest, AttendanceRequestAction } from "./types";

/**
 * Setujui / tolak pengajuan absensi (tab HRD). Setujui langsung dikirim; tolak
 * lewat dialog dengan alasan wajib. Pesan server ditampilkan apa adanya.
 * Render `dialog` sekali.
 */
export function useAttendanceRequestDecision(): {
  request: (r: AttendanceRequest, action: AttendanceRequestAction) => void;
  busyId: string | null | undefined;
  dialog: ReactNode;
} {
  const decide = useDecideAttendanceRequest();
  const [target, setTarget] = useState<AttendanceRequest | null>(null);
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const busyId = decide.isPending ? decide.variables?.id : null;

  function run(r: AttendanceRequest, action: AttendanceRequestAction, rejectionReason?: string) {
    decide.mutate(
      { id: r.id, action, rejectionReason },
      {
        onSuccess: (json) => toast.success(json.message || "Berhasil diproses"),
        onError: (e) => toast.error(e.message || "Gagal memproses"),
      }
    );
  }

  function request(r: AttendanceRequest, action: AttendanceRequestAction) {
    if (action === "approve") {
      run(r, "approve");
      return;
    }
    setReason("");
    setTarget(r);
    setOpen(true);
  }

  function confirm() {
    if (!target) return;
    const rejectionReason = reason.trim();
    if (!rejectionReason) {
      toast.error("Alasan penolakan wajib diisi");
      return;
    }
    setOpen(false);
    run(target, "reject", rejectionReason);
  }

  const dialog = (
    <ConfirmDialog
      open={open}
      onOpenChange={setOpen}
      title="Tolak pengajuan absensi?"
      confirmLabel="Tolak pengajuan"
      cancelLabel="Kembali"
      onConfirm={confirm}
    >
      <div className="space-y-1.5">
        <label htmlFor="attendance-request-rejection" className="block text-xs font-medium text-gray-600">
          Alasan penolakan
        </label>
        <Textarea
          id="attendance-request-rejection"
          rows={3}
          maxLength={500}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </div>
    </ConfirmDialog>
  );

  return { request, busyId, dialog };
}
