"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { downloadOrgChartImportErrors, importOrgChartFile } from "../api";
import { orgChartQueryKeys } from "../query-keys";
import type { ChartImportPreview, ChartUploadIssue } from "../types";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Versi dasar pembanding (label yang sedang dilihat). */
  base: string;
  onImported: (label: string) => void;
}

const MAX_CODES = 20;
const statusOf = (err: unknown) => (err as { status?: number } | null)?.status;

function issueText(i: ChartUploadIssue) {
  return `${i.line > 0 ? `Baris ${i.line}` : "Bagan"} · ${i.column}: ${i.message}`;
}

function IssueList({ issues, tone }: { issues: ChartUploadIssue[]; tone: "danger" | "warning" }) {
  return (
    <ul className={`max-h-48 space-y-1 overflow-y-auto rounded-lg p-3 text-sm ${tone === "danger" ? "bg-danger-soft text-danger" : "bg-warning-soft text-warning"}`}>
      {issues.map((i, idx) => (
        <li key={idx} className="break-words">
          {issueText(i)}
        </li>
      ))}
    </ul>
  );
}

function CodeList({ title, items }: { title: string; items: string[] }) {
  if (items.length === 0) return null;
  const more = items.length - MAX_CODES;
  return (
    <div>
      <p className="text-xs font-semibold text-gray-700">
        {title} {items.length}
      </p>
      <ul className="mt-0.5 flex flex-wrap gap-1 text-xs text-gray-700">
        {items.slice(0, MAX_CODES).map((c) => (
          <li key={c} className="rounded bg-gray-100 px-1.5 py-0.5 break-words">
            {c}
          </li>
        ))}
        {more > 0 ? <li className="px-1.5 py-0.5 text-gray-500">dan {more} lainnya</li> : null}
      </ul>
    </div>
  );
}

/**
 * Impor versi bagan (EPIC-103): unggah template, pratinjau error/peringatan
 * dan selisih dengan versi dasar, lalu simpan sebagai versi Draft. Versi
 * resmi tetap lewat "Setujui versi".
 */
export function ImportVersionDialog({ open, onOpenChange, base, onImported }: Props) {
  const qc = useQueryClient();
  const [label, setLabel] = useState("");
  const [file, setFile] = useState<File | null>(null);
  // Pratinjau hanya berlaku untuk file dan label yang diperiksa.
  const [checked, setChecked] = useState<{ file: File; label: string; data: ChartImportPreview } | null>(null);
  const preview = checked && checked.file === file && checked.label === label.trim() ? checked.data : null;
  const [ack, setAck] = useState(false);
  const [busy, setBusy] = useState<"check" | "save" | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const errors = preview?.errors ?? [];
  const warnings = preview?.warnings ?? [];
  const canCheck = !!file && label.trim() !== "" && busy === null;
  const canSave = !!preview && errors.length === 0 && (warnings.length === 0 || ack) && busy === null;

  async function check() {
    if (!file || !canCheck) return;
    const target = { file, label: label.trim() };
    setBusy("check");
    setNotice(null);
    try {
      const res = await importOrgChartFile({ ...target, base, step: "preview" });
      setChecked({ ...target, data: res.data as ChartImportPreview });
      setAck(false);
    } catch (err) {
      setChecked(null);
      toast.error(err instanceof Error ? err.message : "File tidak bisa diperiksa");
    } finally {
      setBusy(null);
    }
  }

  async function save() {
    if (!canSave || !checked || !preview) return;
    setBusy("save");
    setNotice(null);
    try {
      const res = await importOrgChartFile({
        file: checked.file,
        label: checked.label,
        base,
        step: "commit",
        fingerprint: preview.fingerprint,
        ack_warnings: ack,
      });
      toast.success(res.message ?? `Versi ${checked.label} disimpan sebagai draft`);
      void qc.invalidateQueries({ queryKey: orgChartQueryKeys.all });
      onImported(checked.label);
      onOpenChange(false);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Gagal menyimpan versi";
      if (statusOf(err) === 409) {
        setChecked(null);
        setAck(false);
      }
      setNotice(message);
    } finally {
      setBusy(null);
    }
  }

  async function downloadErrors() {
    if (!checked) return;
    try {
      await downloadOrgChartImportErrors({ file: checked.file, label: checked.label, base });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Gagal membuat file error");
    }
  }

  const reset = () => {
    setAck(false);
    setNotice(null);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>Impor versi bagan</DialogTitle>
          <DialogDescription>
            Unggah template yang sudah diubah. Versi baru disimpan sebagai Draft dan dibandingkan dengan {base || "versi bawaan"}.
            Data karyawan dan approval tidak berubah.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 text-sm">
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-gray-600">Label versi</span>
            <Input
              aria-label="Label versi"
              placeholder="Draft 7"
              maxLength={40}
              value={label}
              disabled={busy !== null}
              onChange={(e) => {
                setLabel(e.target.value);
                reset();
              }}
            />
          </label>
          <label className="block">
            <span className="mb-1 block text-xs font-medium text-gray-600">File template (.xlsx)</span>
            <Input
              type="file"
              accept=".xlsx"
              aria-label="File template (.xlsx)"
              disabled={busy !== null}
              onChange={(e) => {
                setFile(e.target.files?.[0] ?? null);
                reset();
              }}
            />
          </label>
          <Button variant="outline" disabled={!canCheck} onClick={() => void check()}>
            {busy === "check" ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            Pratinjau
          </Button>

          {notice ? (
            <p role="alert" className="rounded-lg bg-danger-soft p-3 text-danger">
              {notice}
            </p>
          ) : null}

          {preview ? (
            <div className="space-y-3">
              <p className="font-medium text-gray-800">
                {preview.summary.nodes} node · {preview.summary.positions} posisi
              </p>
              <p className="text-xs text-gray-600">
                Perkiraan dengan karyawan aktif: Terisi {preview.vacancy.filled} · Kosong {preview.vacancy.empty} · Mungkin terisi{" "}
                {preview.vacancy.maybe} · Rencana {preview.vacancy.planned}
              </p>
              <div className="space-y-2 rounded-lg border p-3">
                <p className="text-xs text-gray-500">Dibanding {preview.base.label}</p>
                {preview.diff.added.length + preview.diff.removed.length + preview.diff.changed.length === 0 ? (
                  <p className="text-xs text-gray-600">Tidak ada perubahan.</p>
                ) : null}
                <CodeList title="Ditambah" items={preview.diff.added} />
                <CodeList title="Dihapus" items={preview.diff.removed} />
                <CodeList
                  title="Diubah"
                  items={preview.diff.changed.map((c) => `${c.code} ${c.title} (${c.fields.join(", ")})`)}
                />
              </div>
              {errors.length > 0 ? (
                <div className="space-y-2">
                  <p className="font-semibold text-danger">{errors.length} error — perbaiki file lalu pratinjau lagi</p>
                  <IssueList issues={errors} tone="danger" />
                  <Button variant="outline" size="sm" onClick={() => void downloadErrors()}>
                    Unduh file error
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
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={busy === "save"}>
            Batal
          </Button>
          <Button disabled={!canSave} onClick={() => void save()}>
            {busy === "save" ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            Simpan sebagai draft
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
