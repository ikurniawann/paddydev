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
import { CheckCircle2, XCircle, Loader2 } from "lucide-react";
import { useApproveLeave } from "@/features/hris/leaves/mutations";

const WA_NOTICE = "Membuka WhatsApp untuk memberi tahu karyawan…";

/**
 * Tombol setujui/tolak untuk pengajuan cuti yang masih pending. rejectOnly:
 * pengajuan menunggu verifikasi lampiran HR hanya bisa ditolak di sini
 * (menyetujui lewat Verifikasi, EPIC-088 R4).
 */
export function ApprovalButtons({ leaveId, rejectOnly = false }: { leaveId: string; rejectOnly?: boolean }) {
  const [showRejectDialog, setShowRejectDialog] = useState(false);
  const [rejectionReason, setRejectionReason] = useState("");
  const approve = useApproveLeave();
  const isLoading = approve.isPending;

  const handleApprove = () => {
    approve.mutate(
      { leave_id: leaveId, action: "approve" },
      {
        onSuccess: (result) => {
          toast.success("✅ Pengajuan Disetujui", {
            description: result.wa_link ? WA_NOTICE : "Leave request telah disetujui",
          });
          if (result.wa_link) window.open(result.wa_link, "_blank");
        },
        onError: () => toast.error("Gagal menyetujui pengajuan."),
      }
    );
  };

  const handleRejectClick = () => {
    setShowRejectDialog(true);
    setRejectionReason("");
  };

  const handleReject = () => {
    const reason = rejectionReason.trim();
    if (!reason) {
      toast.error("⚠️ Alasan Ditolak", { description: "Mohon isi alasan penolakan" });
      return;
    }
    approve.mutate(
      { leave_id: leaveId, action: "reject", rejection_reason: reason },
      {
        onSuccess: (result) => {
          toast.success("❌ Pengajuan Ditolak", {
            description: result.wa_link ? WA_NOTICE : reason,
          });
          if (result.wa_link) window.open(result.wa_link, "_blank");
          setShowRejectDialog(false);
          setRejectionReason("");
        },
        onError: () => toast.error("Gagal menolak pengajuan."),
      }
    );
  };

  return (
    <>
      <div className="flex gap-2">
        {!rejectOnly && (
          <Button
            onClick={handleApprove}
            disabled={isLoading}
            className="bg-green-600 hover:bg-green-700"
            size="sm"
          >
            {isLoading ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <>
                <CheckCircle2 className="w-4 h-4 mr-1" />
                Setujui
              </>
            )}
          </Button>
        )}

        <Button
          onClick={handleRejectClick}
          disabled={isLoading}
          variant="outline"
          size="sm"
          className="text-red-600 hover:text-red-700 hover:bg-red-50"
        >
          {isLoading ? (
            <Loader2 className="w-4 h-4 animate-spin" />
          ) : (
            <>
              <XCircle className="w-4 h-4 mr-1" />
              Tolak
            </>
          )}
        </Button>
      </div>

      <Dialog open={showRejectDialog} onOpenChange={setShowRejectDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Tolak Pengajuan Cuti</DialogTitle>
            <DialogDescription>
              Berikan alasan penolakan yang akan disampaikan kepada karyawan.
            </DialogDescription>
          </DialogHeader>

          <Textarea
            value={rejectionReason}
            onChange={(e) => setRejectionReason(e.target.value)}
            placeholder="Contoh: Jumlah staff yang tidak mencukupi pada periode tersebut..."
            rows={4}
            className="resize-none"
          />

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setShowRejectDialog(false)}>
              Batal
            </Button>
            <Button
              type="button"
              onClick={handleReject}
              disabled={!rejectionReason.trim() || isLoading}
              variant="destructive"
            >
              {isLoading ? (
                <>
                  <Loader2 className="w-4 h-4 mr-2 animate-spin" />
                  Menolak...
                </>
              ) : (
                <>
                  <XCircle className="w-4 h-4 mr-2" />
                  Tolak Pengajuan
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
