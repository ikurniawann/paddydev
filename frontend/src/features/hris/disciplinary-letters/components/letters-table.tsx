"use client";

import { useState } from "react";
import { Loader2, Paperclip, Plus } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogFooter,
  DialogPanel,
  DialogPanelBody,
  DialogPanelHeader,
  DialogPanelTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { formatLetterDate, letterAttachmentUrl } from "../api";
import { useRevokeLetter } from "../mutations";
import { useLetters } from "../queries";
import type { DisciplinaryLetter } from "../types";
import { IssueLetterDialog } from "./issue-letter-dialog";

interface LettersTableProps {
  /** Dari profil karyawan: hanya surat orang itu, kolom karyawan disembunyikan. */
  employeeId?: string;
  employeeName?: string;
  canIssue: boolean;
  canRevoke: boolean;
}

/** Daftar Surat Teguran/SP (EPIC-094): terbitkan, lihat lampiran, batalkan dengan alasan. */
export function LettersTable({ employeeId, employeeName, canIssue, canRevoke }: LettersTableProps) {
  const [includeRevoked, setIncludeRevoked] = useState(false);
  const [issueOpen, setIssueOpen] = useState(false);
  const [revoking, setRevoking] = useState<DisciplinaryLetter | null>(null);
  const letters = useLetters({ employeeId, includeRevoked });
  const rows = letters.data ?? [];

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <label className="flex items-center gap-2 text-sm text-gray-700">
          <input
            type="checkbox"
            aria-label="Tampilkan yang dibatalkan"
            checked={includeRevoked}
            onChange={(e) => setIncludeRevoked(e.target.checked)}
          />
          Tampilkan yang dibatalkan
        </label>
        {canIssue ? (
          <Button onClick={() => setIssueOpen(true)}>
            <Plus className="mr-1 h-4 w-4" /> Terbitkan surat
          </Button>
        ) : null}
      </div>

      <div className="overflow-x-auto rounded-xl border border-gray-200 bg-white">
        <table className="w-full min-w-[760px] text-sm">
          <thead>
            <tr className="border-b border-gray-100 bg-gray-50 text-left text-xs text-gray-500">
              <th className="p-3">Tanggal</th>
              {employeeId ? null : <th className="p-3">Karyawan</th>}
              <th className="p-3">Jenis</th>
              <th className="min-w-[14rem] p-3">Catatan</th>
              <th className="p-3">Lampiran</th>
              <th className="p-3">Diterbitkan oleh</th>
              {canRevoke ? <th className="p-3" /> : null}
            </tr>
          </thead>
          <tbody>
            {letters.isLoading ? (
              <tr>
                <td colSpan={7} className="p-6 text-center text-gray-400">
                  <Loader2 className="mx-auto h-4 w-4 animate-spin" />
                </td>
              </tr>
            ) : letters.isError ? (
              <tr>
                <td colSpan={7} className="p-6 text-center text-red-600">
                  Daftar surat gagal dimuat.
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td colSpan={7} className="p-6 text-center text-gray-400">
                  Belum ada surat.
                </td>
              </tr>
            ) : (
              rows.map((row) => {
                const revoked = row.revoked_at !== null;
                return (
                  <tr key={row.id} className={cn("border-b border-gray-50 align-top", revoked && "bg-gray-50 text-gray-400")}>
                    <td className="whitespace-nowrap p-3">{formatLetterDate(row.issued_date)}</td>
                    {employeeId ? null : (
                      <td className="p-3">
                        <p className="font-medium text-gray-900">{row.employee_name}</p>
                        {row.nip ? <p className="text-xs text-gray-500">{row.nip}</p> : null}
                      </td>
                    )}
                    <td className="p-3">
                      <p className="whitespace-nowrap font-medium">{row.label}</p>
                      {revoked ? (
                        <>
                          <span className="mt-1 inline-block rounded-full bg-gray-200 px-2 py-0.5 text-xs text-gray-700">Dibatalkan</span>
                          <p className="mt-1 text-xs text-gray-500">Alasan: {row.revoke_reason}</p>
                        </>
                      ) : null}
                    </td>
                    <td className="whitespace-pre-line break-words p-3">{row.notes || "—"}</td>
                    <td className="p-3">
                      {row.attachment_path ? (
                        <a
                          href={letterAttachmentUrl(row.attachment_path)}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex items-center gap-1 whitespace-nowrap text-pink-600 underline"
                        >
                          <Paperclip className="h-3.5 w-3.5" />
                          Lihat lampiran
                        </a>
                      ) : (
                        "—"
                      )}
                    </td>
                    <td className="p-3">{row.created_by_name || "—"}</td>
                    {canRevoke ? (
                      <td className="p-3 text-right">
                        {revoked ? null : (
                          <Button size="sm" variant="outline" onClick={() => setRevoking(row)}>
                            Batalkan
                          </Button>
                        )}
                      </td>
                    ) : null}
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      <IssueLetterDialog open={issueOpen} onOpenChange={setIssueOpen} employeeId={employeeId} employeeName={employeeName} />
      <RevokeLetterDialog letter={revoking} onClose={() => setRevoking(null)} />
    </div>
  );
}

function RevokeLetterDialog({ letter, onClose }: { letter: DisciplinaryLetter | null; onClose: () => void }) {
  const [reason, setReason] = useState("");
  const revoke = useRevokeLetter();
  const close = () => {
    setReason("");
    onClose();
  };
  return (
    <Dialog open={letter !== null} onOpenChange={(o) => !o && close()}>
      <DialogPanel size="sm">
        <DialogPanelHeader>
          <DialogPanelTitle>Batalkan {letter?.label}</DialogPanelTitle>
        </DialogPanelHeader>
        <DialogPanelBody className="space-y-2">
          <p className="text-sm text-gray-600">
            Surat untuk {letter?.employee_name} tanggal {letter ? formatLetterDate(letter.issued_date) : ""} akan dibatalkan
            dan tidak tampil lagi di Area Karyawan. Riwayatnya tetap tersimpan.
          </p>
          <label htmlFor="revoke-reason" className="block text-xs font-medium text-gray-600">
            Alasan pembatalan
          </label>
          <Textarea
            id="revoke-reason"
            aria-label="Alasan pembatalan"
            rows={3}
            maxLength={1000}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </DialogPanelBody>
        <DialogFooter>
          <Button variant="outline" onClick={close} disabled={revoke.isPending}>
            Kembali
          </Button>
          <Button
            variant="destructive"
            disabled={!reason.trim() || revoke.isPending}
            onClick={() =>
              letter &&
              revoke.mutate(
                { id: letter.id, reason: reason.trim() },
                {
                  onSuccess: () => {
                    toast.success("Surat dibatalkan");
                    close();
                  },
                  onError: (e: Error) => toast.error(e.message || "Gagal membatalkan surat"),
                }
              )
            }
          >
            Batalkan surat
          </Button>
        </DialogFooter>
      </DialogPanel>
    </Dialog>
  );
}
