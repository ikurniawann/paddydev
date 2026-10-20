"use client";

import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Textarea } from "@/components/ui/textarea";
import { useDecideBusinessTrip } from "./mutations";
import type { BusinessTrip, TripAction } from "./types";

interface PendingAction {
  trip: BusinessTrip;
  action: Exclude<TripAction, "approve">;
}

const CANCEL_DESCRIPTION = {
  pending: "Pengajuan Perjalanan Dinas ini akan dibatalkan.",
  // P17: pembatalan tidak mengembalikan baris absensi yang sudah ditandai Dinas.
  approved:
    "Perjalanan Dinas yang sudah disetujui akan dibatalkan. Absensi yang sudah ditandai Dinas tidak dikembalikan otomatis — tinjau ulang di Rekap Absensi.",
} as const;

/**
 * Setujui / tolak / batalkan Perjalanan Dinas, dipakai halaman ESS dan tab HRD.
 * Setujui langsung dikirim; tolak (alasan wajib) dan batalkan lewat dialog
 * konfirmasi. Pesan server ditampilkan apa adanya. Render `dialog` sekali.
 */
export function useTripDecision(): {
  request: (trip: BusinessTrip, action: TripAction) => void;
  busyId: string | null | undefined;
  dialog: ReactNode;
} {
  const decide = useDecideBusinessTrip();
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [reason, setReason] = useState("");
  const busyId = decide.isPending ? decide.variables?.id : null;

  function run(trip: BusinessTrip, action: TripAction, rejectionReason?: string) {
    decide.mutate(
      { id: trip.id, action, rejectionReason },
      {
        onSuccess: (json) => toast.success(json.message || "Berhasil diproses"),
        onError: (e) => toast.error(e.message || "Gagal memproses"),
      }
    );
  }

  function request(trip: BusinessTrip, action: TripAction) {
    if (action === "approve") {
      run(trip, action);
      return;
    }
    setReason("");
    setPending({ trip, action });
  }

  function confirm() {
    if (!pending) return;
    const { trip, action } = pending;
    const rejectionReason = reason.trim();
    if (action === "reject" && !rejectionReason) {
      toast.error("Alasan penolakan wajib diisi");
      return;
    }
    setPending(null);
    run(trip, action, action === "reject" ? rejectionReason : undefined);
  }

  const rejecting = pending?.action === "reject";
  const dialog = (
    <ConfirmDialog
      open={pending !== null}
      onOpenChange={(open) => {
        if (!open) setPending(null);
      }}
      title={rejecting ? "Tolak Perjalanan Dinas?" : "Batalkan Perjalanan Dinas?"}
      description={rejecting ? undefined : CANCEL_DESCRIPTION[pending?.trip.status === "approved" ? "approved" : "pending"]}
      confirmLabel={rejecting ? "Tolak pengajuan" : "Batalkan pengajuan"}
      cancelLabel="Kembali"
      onConfirm={confirm}
    >
      {rejecting && (
        <div className="space-y-1.5">
          <label htmlFor="trip-rejection-reason" className="block text-xs font-medium text-gray-600">
            Alasan penolakan
          </label>
          <Textarea
            id="trip-rejection-reason"
            rows={3}
            maxLength={500}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
      )}
    </ConfirmDialog>
  );

  return { request, busyId, dialog };
}
