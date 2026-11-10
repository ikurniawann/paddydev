"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Download, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { branchCodeTemplateUrl, downloadBranchCodeErrors, importBranchCodeFile } from "../api";
import { placementQueryKeys } from "../query-keys";
import type { BranchCodeImportIssue, BranchCodeImportPreview } from "../types";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const statusOf = (err: unknown) => (err as { status?: number } | null)?.status;
const activeLabel = (active: boolean) => (active ? "Aktif" : "Nonaktif");
const arrow = (from: string, to: string) => (from === to ? from : `${from} → ${to}`);

function issueTitle(i: BranchCodeImportIssue): string {
  return [i.line > 0 ? `Baris ${i.line}` : "", i.column, i.name].filter(Boolean).join(" · ");
}

function IssueList({ issues, tone }: { issues: BranchCodeImportIssue[]; tone: "danger" | "warning" }) {
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
 * Kode & Status Toko massal (EPIC-100): unduh template, periksa file, simpan
 * hanya bila tanpa error dan peringatan sudah dicentang. Kode diganti di toko
 * yang sama, jadi histori tetap tersimpan; toko yang masih dipakai ditolak
 * dinonaktifkan oleh server.
 */
export function BranchCodeDialog({ open, onOpenChange }: Props) {
  const qc = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  // Pratinjau hanya berlaku untuk objek File yang diperiksa.
  const [checked, setChecked] = useState<{ file: File; data: BranchCodeImportPreview } | null>(null);
  const preview = checked && checked.file === file ? checked.data : null;
  const [ack, setAck] = useState(false);
  const [busy, setBusy] = useState<"check" | "save" | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const errors = preview?.errors ?? [];
  const warnings = preview?.warnings ?? [];
  const changes = preview?.changes ?? [];
  const canSave = !!preview && errors.length === 0 && (warnings.length === 0 || ack) && busy === null;

  async function check() {
    if (!file) return;
    const target = file;
    setBusy("check");
    setNotice(null);
    try {
      const res = await importBranchCodeFile({ file: target, step: "preview" });
      setChecked({ file: target, data: res.data });
      setAck(false);
    } catch (err) {
      setChecked(null);
      toast.error(err instanceof Error ? err.message : "File tidak bisa diperiksa");
    } finally {
      setBusy(null);
    }
  }

  async function save() {
    if (!canSave || !checked || checked.file !== file || !preview) return;
    setBusy("save");
    setNotice(null);
    try {
      const res = await importBranchCodeFile({
        file: checked.file,
        step: "commit",
        fingerprint: preview.fingerprint,
        ack_warnings: ack,
      });
      void qc.invalidateQueries({ queryKey: placementQueryKeys.all });
      toast.success(res.message ?? "Data toko disimpan");
      onOpenChange(false);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Gagal menyimpan data toko";
      if (statusOf(err) === 409) {
        setChecked(null);
        setAck(false);
        setNotice(`${message} — klik "Periksa file" untuk melihat pratinjau terbaru.`);
      } else {
        setNotice(message);
      }
    } finally {
      setBusy(null);
    }
  }

  async function downloadErrors() {
    if (!file) return;
    try {
      await downloadBranchCodeErrors(file);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal membuat file error");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Kode & Status Toko</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 text-sm">
          <p className="text-gray-600">
            Ganti kode, nama, atau status banyak toko sekaligus. Histori absen, penempatan, jadwal, dan payroll tetap
            tersimpan di toko yang sama. Toko yang masih punya karyawan aktif atau jadwal tidak bisa dinonaktifkan.
          </p>
          <Button variant="outline" size="sm" asChild>
            <a href={branchCodeTemplateUrl} download>
              <Download className="mr-1.5 h-4 w-4" /> Unduh template
            </a>
          </Button>
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-gray-600">File kode & status toko (.xlsx)</span>
            <Input
              type="file"
              accept=".xlsx"
              aria-label="File kode & status toko (.xlsx)"
              disabled={busy !== null}
              onChange={(e) => {
                setFile(e.target.files?.[0] ?? null);
                setChecked(null);
                setAck(false);
                setNotice(null);
              }}
            />
          </label>
          <Button variant="outline" disabled={!file || busy !== null} onClick={check}>
            {busy === "check" ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            Periksa file
          </Button>

          {notice ? (
            <p role="alert" className="rounded-lg bg-danger-soft p-3 text-danger">
              {notice}
            </p>
          ) : null}

          {preview ? (
            <div className="space-y-3">
              <p className="font-medium text-gray-800">
                {preview.summary.rows} toko · {preview.summary.changed} berubah · {preview.summary.code} kode ·{" "}
                {preview.summary.deactivate} dinonaktifkan · {preview.summary.activate} diaktifkan
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
                        <th className="px-3 py-2">Baris</th>
                        <th className="px-3 py-2">Kode</th>
                        <th className="px-3 py-2">Nama</th>
                        <th className="px-3 py-2">Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {changes.map((c) => (
                        <tr key={c.line} className="border-t">
                          <td className="px-3 py-1.5">{c.line}</td>
                          <td className="whitespace-nowrap px-3 py-1.5">{arrow(c.old_code, c.new_code)}</td>
                          <td className="px-3 py-1.5">{arrow(c.old_name, c.new_name)}</td>
                          <td className="whitespace-nowrap px-3 py-1.5">
                            {arrow(activeLabel(c.old_active), activeLabel(c.new_active))}
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
          <Button disabled={!canSave} onClick={() => void save()}>
            {busy === "save" ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            Simpan perubahan toko
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
