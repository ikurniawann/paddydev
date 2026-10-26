"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { essQueryKeys } from "@/features/hris/ess/query-keys";
import { formatDate } from "@/lib/format";
import { downloadLeaveBalanceErrors, importLeaveBalanceFile } from "../api";
import { formatDays } from "../balance-math";
import { leaveBalanceQueryKeys } from "../query-keys";
import type { LeaveBalanceImportIssue, LeaveBalanceImportPreview } from "../types";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  year: number;
  /** "" = semua cabang. */
  branchId: string;
  branchName: string;
  onSaved?: () => void;
}

const days = (n: number) => formatDays(n);
const date = (iso: string | null) => (iso ? formatDate(iso) : "—");
const statusOf = (err: unknown) => (err as { status?: number } | null)?.status;

function issueTitle(i: LeaveBalanceImportIssue): string {
  return [i.line > 0 ? `Baris ${i.line}` : "", i.column, i.name].filter(Boolean).join(" · ");
}

function IssueList({ issues, tone }: { issues: LeaveBalanceImportIssue[]; tone: "danger" | "warning" }) {
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
 * Import Excel Saldo Cuti (EPIC-068 R10): periksa dulu (preview), simpan hanya
 * bila tanpa error dan peringatan sudah dicentang. Pesan server ditampilkan
 * apa adanya; 409 (saldo berubah sejak pratinjau) membuang pratinjau dan
 * meminta file diperiksa ulang.
 */
export function LeaveBalanceUploadDialog({ open, onOpenChange, year, branchId, branchName, onSaved }: Props) {
  const qc = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<LeaveBalanceImportPreview | null>(null);
  const [ack, setAck] = useState(false);
  const [busy, setBusy] = useState<"check" | "save" | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const scope = { year, branch_id: branchId };

  const errors = preview?.errors ?? [];
  const warnings = preview?.warnings ?? [];
  const changes = preview?.changes ?? [];
  const totalChanges = (preview?.summary.new ?? 0) + (preview?.summary.changed ?? 0);
  const canSave = !!preview && errors.length === 0 && (warnings.length === 0 || ack) && busy === null;

  async function check() {
    if (!file) return;
    setBusy("check");
    setNotice(null);
    try {
      const res = await importLeaveBalanceFile({ ...scope, file, step: "preview" });
      setPreview(res.data);
      setAck(false);
    } catch (err) {
      setPreview(null);
      toast.error(err instanceof Error ? err.message : "File tidak bisa diperiksa");
    } finally {
      setBusy(null);
    }
  }

  async function save() {
    if (!canSave || !file || !preview) return;
    setBusy("save");
    setNotice(null);
    try {
      const res = await importLeaveBalanceFile({
        ...scope,
        file,
        step: "commit",
        fingerprint: preview.fingerprint,
        ack_warnings: ack,
      });
      void qc.invalidateQueries({ queryKey: leaveBalanceQueryKeys.all });
      void qc.invalidateQueries({ queryKey: essQueryKeys.all });
      toast.success(res.message ?? "Saldo cuti disimpan");
      onSaved?.();
      onOpenChange(false);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Gagal menyimpan saldo cuti";
      if (statusOf(err) === 409) {
        // Saldo berubah sejak pratinjau: pratinjau lama tidak boleh disimpan.
        setPreview(null);
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
      await downloadLeaveBalanceErrors({ ...scope, file });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal membuat file error");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>
            Import saldo cuti — {branchName} {year}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 text-sm">
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-gray-600">File saldo cuti (.xlsx)</span>
            <Input
              type="file"
              accept=".xlsx"
              aria-label="File saldo cuti (.xlsx)"
              onChange={(e) => {
                setFile(e.target.files?.[0] ?? null);
                setPreview(null);
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
                <div className="space-y-1">
                  <div className="overflow-x-auto rounded-lg border">
                    <table className="w-full text-xs">
                      <thead className="bg-gray-50 text-left text-gray-500">
                        <tr>
                          <th className="px-3 py-2">NIP</th>
                          <th className="px-3 py-2">Nama</th>
                          <th className="px-3 py-2">Jatah</th>
                          <th className="px-3 py-2">Penyesuaian</th>
                          <th className="px-3 py-2">Sisa bawaan</th>
                          <th className="px-3 py-2">Kedaluwarsa</th>
                          <th className="px-3 py-2">Alasan</th>
                        </tr>
                      </thead>
                      <tbody>
                        {changes.slice(0, 500).map((c, i) => (
                          <tr key={`${c.nip}-${i}`} className="border-t">
                            <td className="px-3 py-1.5">{c.nip}</td>
                            <td className="px-3 py-1.5">
                              {c.name}
                              {c.new ? (
                                <span className="ml-2 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-500">Baru</span>
                              ) : null}
                            </td>
                            <td className="px-3 py-1.5">{`${days(c.before.annual_leave_total)} → ${days(c.after.annual_leave_total)}`}</td>
                            <td className="px-3 py-1.5">{`${days(c.before.annual_leave_adjustment)} → ${days(c.after.annual_leave_adjustment)}`}</td>
                            <td className="px-3 py-1.5">{`${days(c.before.carry_over_days)} → ${days(c.after.carry_over_days)}`}</td>
                            <td className="px-3 py-1.5">{`${date(c.before.carry_over_expires_on)} → ${date(c.after.carry_over_expires_on)}`}</td>
                            <td className="px-3 py-1.5">{c.reason}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  {totalChanges > changes.length ? (
                    <p className="text-xs text-gray-500">
                      Menampilkan {changes.length} dari {totalChanges} perubahan; semuanya tetap disimpan.
                    </p>
                  ) : null}
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
            Simpan saldo
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
