"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { BadgeCheck, Undo2, Loader2 } from "lucide-react";
import { useReviewLeaveAttachment } from "@/features/hris/leaves/mutations";

/**
 * Tombol HR untuk pengajuan yang sudah disetujui atasan dan menunggu
 * verifikasi lampiran (EPIC-088 R5): Verifikasi atau Kembalikan dengan alasan.
 */
export function AttachmentReviewButtons({ leaveId, name }: { leaveId: string; name: string }) {
  const review = useReviewLeaveAttachment();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const busy = review.isPending;

  const done = {
    onSuccess: (res: { message?: string }) => {
      toast.success(res?.message ?? "Tersimpan");
      setOpen(false);
      setReason("");
    },
    onError: (e: Error) => toast.error(e.message || "Gagal memproses lampiran"),
  };

  return (
    <>
      <Button
        size="sm"
        className="bg-green-600 hover:bg-green-700"
        disabled={busy}
        aria-label={`Verifikasi lampiran ${name}`}
        onClick={() => review.mutate({ leave_id: leaveId, action: "verify" }, done)}
      >
        {busy ? <Loader2 className="w-4 h-4 animate-spin" /> : <BadgeCheck className="w-4 h-4 mr-1" />}
        Verifikasi
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={busy}
        aria-label={`Kembalikan lampiran ${name}`}
        onClick={() => {
          setReason("");
          setOpen(true);
        }}
      >
        <Undo2 className="w-4 h-4 mr-1" />
        Kembalikan
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Kembalikan lampiran</DialogTitle>
            <DialogDescription>
              Karyawan menerima alasan ini dan mengunggah ulang lampiran. Persetujuan atasan tetap berlaku.
            </DialogDescription>
          </DialogHeader>
          <Textarea
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Contoh: Foto surat dokter buram, mohon unggah ulang yang terbaca."
            rows={4}
            className="resize-none"
          />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>
              Batal
            </Button>
            <Button
              type="button"
              disabled={!reason.trim() || busy}
              onClick={() =>
                review.mutate({ leave_id: leaveId, action: "return", reason: reason.trim() }, done)
              }
            >
              Kembalikan ke karyawan
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
