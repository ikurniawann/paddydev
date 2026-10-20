"use client";

import { useState, useSyncExternalStore } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelDescription,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import {
  DeleteCancelledError,
  MIN_DELETE_REASON,
  currentDeleteReason,
  isValidDeleteReason,
  subscribeDeleteReason,
  type PendingDeleteReason,
} from "@/lib/hris/delete-reason";

/** Dialog alasan hapus HRIS untuk seluruh aplikasi (EPIC-061, Q-RA-4). */
export function DeleteReasonHost() {
  const pending = useSyncExternalStore(subscribeDeleteReason, currentDeleteReason, () => null);
  if (!pending) return null;
  return <ReasonDialog key={pending.id} pending={pending} />;
}

function ReasonDialog({ pending }: { pending: PendingDeleteReason }) {
  const [reason, setReason] = useState("");
  const ok = isValidDeleteReason(reason);
  const cancel = () => pending.reject(new DeleteCancelledError());
  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : cancel())}>
      <DialogPanel size="xs">
        <DialogPanelHeader>
          <DialogPanelTitle>{pending.title}</DialogPanelTitle>
          <DialogPanelDescription>
            {pending.description ?? "Penghapusan dicatat di audit log beserta alasannya."}
          </DialogPanelDescription>
        </DialogPanelHeader>
        <DialogPanelBody>
          <label htmlFor="hris-delete-reason" className="mb-1 block text-sm font-medium text-gray-700">
            Alasan <span className="text-red-500">*</span>
          </label>
          <Textarea
            id="hris-delete-reason"
            rows={2}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder={`Minimal ${MIN_DELETE_REASON} karakter`}
          />
        </DialogPanelBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={cancel}>
            Batal
          </Button>
          <Button
            type="button"
            disabled={!ok}
            onClick={() => pending.resolve(reason.trim())}
            className="bg-red-600 text-white hover:bg-red-700"
          >
            {pending.confirmLabel ?? "Hapus"}
          </Button>
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}
