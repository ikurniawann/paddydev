"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { LeaveAttachmentField } from "@/features/hris/leaves/components/leave-attachment-field";
import { useResubmitLeaveAttachment } from "../mutations";

/**
 * Unggah ulang lampiran yang dikembalikan HR (EPIC-088 R6): persetujuan
 * atasan tetap berlaku, pengajuan kembali ke antrean verifikasi HR.
 */
export function LeaveResubmit({ leaveId, employeeId }: { leaveId: string; employeeId: string }) {
  const [open, setOpen] = useState(false);
  const [path, setPath] = useState("");
  const resubmit = useResubmitLeaveAttachment();

  if (!open) {
    return (
      <Button size="sm" variant="outline" className="mt-2" onClick={() => setOpen(true)}>
        Unggah ulang lampiran
      </Button>
    );
  }
  return (
    <div className="mt-2 space-y-2">
      <LeaveAttachmentField employeeId={employeeId} onChange={setPath} />
      <div className="flex gap-2">
        <Button size="sm" variant="outline" onClick={() => setOpen(false)}>
          Batal
        </Button>
        <Button
          size="sm"
          disabled={!path || resubmit.isPending}
          onClick={() =>
            resubmit.mutate(
              { id: leaveId, path },
              {
                onSuccess: (res) => {
                  toast.success(res?.message ?? "Lampiran terkirim");
                  setOpen(false);
                  setPath("");
                },
                onError: (e) => toast.error(e.message || "Gagal mengirim lampiran"),
              }
            )
          }
        >
          Kirim lampiran
        </Button>
      </div>
    </div>
  );
}
