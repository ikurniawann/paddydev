"use client";

import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { downloadScheduleErrors, importScheduleFile } from "../api";
import type { ScheduleIssue, SchedulePreview } from "../types";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  range: { branch_id: string; from: string; to: string };
  branchName: string;
  onSaved: () => void;
}

const dm = (iso: string) => `${iso.slice(8, 10)}/${iso.slice(5, 7)}`;
const shown = (code: string) => code || "—";

function issueTitle(i: ScheduleIssue): string {
  return [i.line > 0 ? `Baris ${i.line}` : "", i.column, i.name].filter(Boolean).join(" · ");
}

function IssueList({ issues, tone }: { issues: ScheduleIssue[]; tone: "danger" | "warning" }) {
  return (
    <ul className={`max-h-48 space-y-1.5 overflow-y-auto rounded-lg p-3 text-sm ${tone === "danger" ? "bg-danger-soft" : "bg-warning-soft"}`}>
      {issues.map((i, idx) => (
        <li key={idx}>
          {issueTitle(i) ? <p className="text-xs font-semibold text-gray-600">{issueTitle(i)}</p> : null}
          <p className={tone === "danger" ? "text-danger" : "text-warning"}>{i.message}</p>
        </li>
      ))}
    </ul>
  );
}

/**
 * Unggah template Jadwal Kerja: periksa dulu (preview), simpan hanya bila
 * tanpa error, peringatan sudah dicentang, dan penggantian jadwal yang sudah
 * ada dikonfirmasi.
 */
export function ScheduleUploadDialog({ open, onOpenChange, range, branchName, onSaved }: Props) {
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<SchedulePreview | null>(null);
  const [ack, setAck] = useState(false);
  const [busy, setBusy] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const errors = preview?.errors ?? [];
  const warnings = preview?.warnings ?? [];
  const changes = preview?.changes ?? [];
  const canSave = !!preview && errors.length === 0 && (warnings.length === 0 || ack) && !busy;

  async function check() {
    if (!file) return;
    setBusy(true);
    try {
      const res = await importScheduleFile({ ...range, file, step: "preview" });
      setPreview(res.data);
      setAck(false);
    } catch (err) {
      setPreview(null);
      toast.error(err instanceof Error ? err.message : "File tidak bisa diperiksa");
    } finally {
      setBusy(false);
    }
  }

  async function commit() {
    if (!file || !preview) return;
    setBusy(true);
    try {
      const res = await importScheduleFile({ ...range, file, step: "commit", fingerprint: preview.fingerprint, ack_warnings: ack });
      toast.success(res.message ?? "Jadwal disimpan");
      setConfirmOpen(false);
      onSaved();
      onOpenChange(false);
    } catch (err) {
      setConfirmOpen(false);
      setPreview(null);
      toast.error(err instanceof Error ? err.message : "Gagal menyimpan jadwal");
    } finally {
      setBusy(false);
    }
  }

  function save() {
    if (!canSave || !preview) return;
    if (preview.summary.changed > 0) {
      setConfirmOpen(true);
      return;
    }
    void commit();
  }

  async function downloadErrors() {
    if (!file) return;
    try {
      await downloadScheduleErrors({ ...range, file });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal membuat file error");
    }
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Unggah jadwal — {branchName}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 text-sm">
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-gray-600">File jadwal (.xlsx)</span>
              <Input
                type="file"
                accept=".xlsx"
                aria-label="File jadwal (.xlsx)"
                onChange={(e) => {
                  setFile(e.target.files?.[0] ?? null);
                  setPreview(null);
                }}
              />
            </label>
            <Button variant="outline" disabled={!file || busy} onClick={check}>
              {busy && !preview ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              Periksa file
            </Button>

            {preview ? (
              <div className="space-y-3">
                <p className="font-medium text-gray-800">
                  {preview.summary.employees} karyawan · {preview.summary.new} baru · {preview.summary.changed} berubah ·{" "}
                  {preview.summary.same} sama
                </p>
                {errors.length > 0 ? (
                  <div className="space-y-2">
                    <p className="font-semibold text-danger">{errors.length} error — perbaiki file lalu periksa lagi</p>
                    <IssueList issues={errors} tone="danger" />
                    <Button variant="outline" size="sm" onClick={downloadErrors}>
                      Unduh file dengan keterangan error
                    </Button>
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
                        onChange={(e) => setAck(e.target.checked)}
                      />
                      Saya sudah memeriksa peringatan
                    </label>
                  </div>
                ) : null}
                {changes.length > 0 ? (
                  <div className="overflow-x-auto rounded-lg border">
                    <table className="w-full text-xs">
                      <thead className="bg-gray-50 text-left text-gray-500">
                        <tr>
                          <th className="px-3 py-2">Karyawan</th>
                          <th className="px-3 py-2">Tanggal</th>
                          <th className="px-3 py-2">Sebelum → Sesudah</th>
                        </tr>
                      </thead>
                      <tbody>
                        {changes.slice(0, 50).map((c, i) => (
                          <tr key={i} className="border-t">
                            <td className="px-3 py-1.5">{c.name}</td>
                            <td className="px-3 py-1.5">{dm(c.date)}</td>
                            <td className="px-3 py-1.5">
                              {shown(c.before)} → {c.after}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Batal
            </Button>
            <Button disabled={!canSave} onClick={save}>
              Simpan jadwal
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Ganti jadwal yang sudah ada?"
        confirmLabel="Ya, ganti"
        loading={busy}
        variant="default"
        onConfirm={() => void commit()}
      >
        <div className="space-y-2 text-sm">
          <p>{preview?.summary.changed ?? 0} jadwal yang sudah ada akan diganti</p>
          <ul className="max-h-48 space-y-0.5 overflow-y-auto text-xs text-gray-600">
            {changes
              .filter((c) => c.before !== "")
              .slice(0, 20)
              .map((c, i) => (
                <li key={i}>{`${c.name} · ${dm(c.date)} · ${c.before} → ${c.after}`}</li>
              ))}
          </ul>
        </div>
      </ConfirmDialog>
    </>
  );
}
