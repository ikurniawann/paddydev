"use client";

import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { AttachmentPreviewButtons } from "./attachment-preview";

export interface RequestDetailField {
  label: string;
  value: ReactNode;
}

export interface RequestDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** mis. "Detail cuti" */
  title: string;
  employee: { name: string; nip?: string | null; subtitle?: string | null };
  status?: ReactNode;
  fields: RequestDetailField[];
  reason?: { label?: string; text: string | null | undefined };
  attachments?: { paths: string[]; hrefFor: (path: string) => string; label?: string };
  /** Info tambahan di bawah alasan: keputusan, catatan, koreksi. */
  notes?: ReactNode;
  /** Tombol kaki (Setujui/Tolak); tanpa ini hanya "Tutup". */
  actions?: ReactNode;
}

/**
 * Detail satu pengajuan untuk penyetuju (EPIC-072 R1): alasan utuh dan
 * dokumentasi dengan pratinjau sebelum Setujui/Tolak. Dipakai antrean ESS
 * (atasan) dan halaman HR untuk cuti, lembur, absensi, Perjalanan Dinas, kasbon.
 */
export function RequestDetailDialog({
  open,
  onOpenChange,
  title,
  employee,
  status,
  fields,
  reason,
  attachments,
  notes,
  actions,
}: RequestDetailDialogProps) {
  const reasonText = reason?.text?.trim();
  const sub = [employee.nip, employee.subtitle].filter(Boolean).join(" · ");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogPanel size="sm">
        <DialogPanelHeader>
          <DialogPanelTitle>{title}</DialogPanelTitle>
        </DialogPanelHeader>
        <DialogPanelBody className="min-w-0 space-y-4">
          <div className="flex min-w-0 items-start justify-between gap-3 rounded-lg border border-gray-200/80 bg-gray-50 px-3 py-2">
            <div className="min-w-0">
              <p className="break-words text-sm font-medium text-gray-900">{employee.name}</p>
              {sub && <p className="break-words text-xs text-gray-500">{sub}</p>}
            </div>
            {status && <div className="shrink-0">{status}</div>}
          </div>

          {fields.length > 0 && (
            <dl className="grid grid-cols-[minmax(0,8rem)_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm">
              {fields.map((f) => (
                <div key={f.label} className="contents">
                  <dt className="text-gray-500">{f.label}</dt>
                  <dd className="min-w-0 break-words text-gray-900">{f.value}</dd>
                </div>
              ))}
            </dl>
          )}

          {reason && (
            <div>
              <p className="mb-1 text-xs font-medium text-gray-600">{reason.label ?? "Alasan"}</p>
              <p className="whitespace-pre-line break-words rounded-lg border border-gray-200/80 px-3 py-2 text-sm text-gray-900">
                {reasonText || "—"}
              </p>
            </div>
          )}

          {attachments && attachments.paths.length > 0 && (
            <div>
              <p className="mb-1 text-xs font-medium text-gray-600">Dokumentasi</p>
              <AttachmentPreviewButtons
                paths={attachments.paths}
                hrefFor={attachments.hrefFor}
                label={attachments.label}
                buttonClassName="rounded-md border border-pink-200 bg-pink-50 px-2.5 py-1 text-xs font-medium text-pink-700 hover:bg-pink-100"
                className="gap-2"
              />
            </div>
          )}

          {notes}
        </DialogPanelBody>
        <DialogFooter>
          {actions ?? (
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Tutup
            </Button>
          )}
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}
