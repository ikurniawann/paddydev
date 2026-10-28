"use client";

import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { MIN_DELETE_REASON } from "@/lib/hris/delete-reason";
import type { ScheduleCellsSaver } from "../mutations";
import type { CellsPreview, ScheduleIssue } from "../types";

const dm = (iso: string) => `${iso.slice(8, 10)}/${iso.slice(5, 7)}`;
const before = (code: string) => code || "Belum ada";
const after = (code: string) => code || "Ikut pola mingguan";

/** Masalah per tanggal (nama kosong) tampil sebagai daftar umum; sisanya per karyawan. */
function IssueList({ issues, tone }: { issues: ScheduleIssue[]; tone: "danger" | "warning" }) {
  const general = issues.filter((i) => !i.name);
  const perEmployee = issues.filter((i) => i.name);
  const text = tone === "danger" ? "text-danger" : "text-warning";
  return (
    <div className={`max-h-48 space-y-2 overflow-y-auto rounded-lg p-3 text-sm ${tone === "danger" ? "bg-danger-soft" : "bg-warning-soft"}`}>
      {general.length > 0 ? (
        <ul className="space-y-0.5">
          {general.map((i, idx) => (
            <li key={idx} className={text}>{`${i.column}: ${i.message}`}</li>
          ))}
        </ul>
      ) : null}
      {perEmployee.length > 0 ? (
        <ul className="space-y-1.5">
          {perEmployee.map((i, idx) => (
            <li key={idx}>
              <p className="text-xs font-semibold text-gray-600">{[i.name, i.column].filter(Boolean).join(" · ")}</p>
              <p className={text}>{i.message}</p>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

interface PanelProps {
  preview: CellsPreview;
  ack: boolean;
  onAck: (ack: boolean) => void;
  reason: string;
  onReason: (reason: string) => void;
}

/** Ringkasan pratinjau kotak jadwal: error, peringatan, dan jadwal yang tertimpa. */
export function CellsReviewPanel({ preview, ack, onAck, reason, onReason }: PanelProps) {
  const { summary, errors, warnings, changes, replaced_days: replaced } = preview;
  const replacedChanges = changes.filter((c) => c.before !== "");
  return (
    <div className="space-y-3 text-sm">
      <p className="font-medium text-gray-800">
        {`${summary.employees} karyawan · ${summary.new} baru · ${summary.changed} berubah · ${summary.same} sama`}
        {summary.cleared > 0 ? ` · ${summary.cleared} dikosongkan` : ""}
      </p>
      {errors.length > 0 ? (
        <div className="space-y-2">
          <p className="font-semibold text-danger">{errors.length} error — jadwal belum bisa disimpan</p>
          <IssueList issues={errors} tone="danger" />
        </div>
      ) : null}
      {warnings.length > 0 ? (
        <div className="space-y-2">
          <p className="font-semibold text-warning">{warnings.length} peringatan</p>
          <IssueList issues={warnings} tone="warning" />
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              aria-label="Saya sudah memeriksa peringatan"
              checked={ack}
              onChange={(e) => onAck(e.target.checked)}
            />
            Saya sudah memeriksa peringatan
          </label>
        </div>
      ) : null}
      {replaced > 0 ? (
        <div className="space-y-2">
          <p className="font-semibold text-gray-800">{replaced} jadwal yang sudah ada akan diganti atau dihapus</p>
          {replacedChanges.length > 0 ? (
            <ul className="max-h-40 space-y-0.5 overflow-y-auto rounded-lg border p-2 text-xs text-gray-600">
              {replacedChanges.slice(0, 30).map((c, i) => (
                <li key={i}>{`${c.name} · ${dm(c.date)} · ${before(c.before)} → ${after(c.after)}`}</li>
              ))}
              {replacedChanges.length > 30 ? <li className="text-gray-400">dan {replacedChanges.length - 30} lainnya</li> : null}
            </ul>
          ) : null}
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-gray-600">
              Alasan mengganti jadwal (minimal {MIN_DELETE_REASON} karakter)
            </span>
            <Input
              aria-label="Alasan mengganti jadwal"
              value={reason}
              onChange={(e) => onReason(e.target.value)}
              placeholder="Contoh: tukar shift atas permintaan leader"
            />
          </label>
        </div>
      ) : null}
    </div>
  );
}

/** Konfirmasi simpan dari grid (ubah kotak, isi, kosongkan, salin minggu lalu). */
export function CellsConfirmDialog({ saver, title }: { saver: ScheduleCellsSaver; title: string }) {
  const review = saver.review;
  return (
    <Dialog open={!!review} onOpenChange={(open) => (!open && !saver.busy ? saver.cancel() : undefined)}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>Periksa perubahan sebelum disimpan.</DialogDescription>
        </DialogHeader>
        {review ? (
          <CellsReviewPanel
            preview={review.preview}
            ack={saver.ack}
            onAck={saver.setAck}
            reason={saver.reason}
            onReason={saver.setReason}
          />
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={saver.cancel} disabled={saver.busy}>
            Batal
          </Button>
          <Button disabled={!saver.canSave} onClick={() => void saver.confirm()}>
            {saver.busy ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            Simpan jadwal
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
