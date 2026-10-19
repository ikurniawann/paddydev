"use client";

import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { buildListUrl } from "@/lib/api-client";
import {
  downloadEmployeeImportErrors,
  importEmployeesFile,
  type EmployeeImportIssue,
  type EmployeeImportPreview,
} from "../api";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}

const MODES = [
  { value: "add", label: "Tambah", hint: "Hanya karyawan baru; NIP yang sudah ada ditolak." },
  { value: "replace", label: "Timpa", hint: "NIP yang ada diperbarui sesuai file; sel kosong tidak mengubah data." },
] as const;

function issueText(i: EmployeeImportIssue): string {
  const where = [i.line > 0 ? `Baris ${i.line}` : "", i.column ? `kolom ${i.column}` : ""].filter(Boolean).join(", ");
  return where ? `${where}: ${i.message}` : i.message;
}

const changeText = (c: { name: string; field: string; before: string; after: string }) =>
  `${c.name} · ${c.field} · ${c.before || "—"} → ${c.after || "—"}`;

/** Import Data Karyawan: periksa dulu, lalu simpan (mode Timpa dikonfirmasi). */
export function EmployeeImportDialog({ open, onOpenChange, onDone }: Props) {
  const [mode, setMode] = useState<"add" | "replace">("add");
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<EmployeeImportPreview | null>(null);
  const [releaseEmails, setReleaseEmails] = useState(false);
  const [busy, setBusy] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const errors = preview?.errors ?? [];
  const warnings = preview?.warnings ?? [];
  const changes = preview?.changes ?? [];
  const emailWarnings = warnings.filter((w) => w.message.includes("akan dilepas"));
  const canSave = !!preview && errors.length === 0 && (emailWarnings.length === 0 || releaseEmails) && !busy;

  function reset() {
    setPreview(null);
    setReleaseEmails(false);
  }

  async function check() {
    if (!file) return;
    setBusy(true);
    try {
      setPreview((await importEmployeesFile({ file, mode, step: "preview" })).data);
      setReleaseEmails(false);
    } catch (err) {
      reset();
      toast.error(err instanceof Error ? err.message : "File tidak bisa diperiksa");
    } finally {
      setBusy(false);
    }
  }

  async function commit() {
    if (!file) return;
    setBusy(true);
    try {
      const res = await importEmployeesFile({ file, mode, step: "commit", release_emails: releaseEmails });
      toast.success(res.message ?? "Import selesai");
      setConfirmOpen(false);
      onDone();
      onOpenChange(false);
    } catch (err) {
      setConfirmOpen(false);
      toast.error(err instanceof Error ? err.message : "Import gagal");
    } finally {
      setBusy(false);
    }
  }

  function save() {
    if (!canSave || !preview) return;
    if (preview.summary.update > 0) {
      setConfirmOpen(true);
      return;
    }
    void commit();
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Import Data Karyawan</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 text-sm">
            <p>
              Pakai{" "}
              <button
                type="button"
                className="text-pink-600 underline"
                onClick={() => {
                  // Unduhan file (route API), bukan navigasi halaman.
                  window.location.href = buildListUrl("/api/hris/employees/import-template", {});
                }}
              >
                template import
              </button>{" "}
              atau file hasil Export. Petunjuk pengisian ada di sheet pertama.
            </p>
            <div role="radiogroup" aria-label="Mode import" className="grid gap-2 sm:grid-cols-2">
              {MODES.map((m) => (
                <label key={m.value} className="flex cursor-pointer gap-2 rounded-lg border p-3">
                  <input
                    type="radio"
                    name="import_mode"
                    aria-label={`${m.label} — ${m.hint}`}
                    checked={mode === m.value}
                    onChange={() => {
                      setMode(m.value);
                      reset();
                    }}
                  />
                  <span>
                    <span className="font-medium">{m.label}</span>
                    <span className="block text-xs text-gray-500">{m.hint}</span>
                  </span>
                </label>
              ))}
            </div>
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-gray-600">File data karyawan</span>
              <Input
                type="file"
                accept=".xlsx,.csv"
                aria-label="File data karyawan"
                onChange={(e) => {
                  setFile(e.target.files?.[0] ?? null);
                  reset();
                }}
              />
            </label>
            <Button variant="outline" disabled={!file || busy} onClick={check}>
              Periksa file
            </Button>

            {preview ? (
              <div className="space-y-3">
                <p className="font-medium text-gray-800">
                  {preview.summary.create} baru · {preview.summary.update} diperbarui · {preview.summary.skip} tidak berubah
                </p>
                {errors.length > 0 ? (
                  <div className="space-y-2">
                    <p className="font-semibold text-danger">{errors.length} error — perbaiki file lalu periksa lagi</p>
                    <ul className="max-h-48 space-y-1 overflow-y-auto rounded-lg bg-danger-soft p-3 text-danger">
                      {errors.map((e, i) => (
                        <li key={i}>{issueText(e)}</li>
                      ))}
                    </ul>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => file && downloadEmployeeImportErrors({ file, mode }).catch((err) => toast.error(String(err)))}
                    >
                      Unduh file dengan keterangan error
                    </Button>
                  </div>
                ) : null}
                {warnings.length > 0 ? (
                  <div className="space-y-2">
                    <ul className="max-h-40 space-y-1 overflow-y-auto rounded-lg bg-warning-soft p-3 text-warning">
                      {warnings.map((w, i) => (
                        <li key={i}>{issueText(w)}</li>
                      ))}
                    </ul>
                    {emailWarnings.length > 0 ? (
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          aria-label={`Lepas email dari data karyawan resign (${emailWarnings.length})`}
                          checked={releaseEmails}
                          onChange={(e) => setReleaseEmails(e.target.checked)}
                        />
                        Lepas email dari data karyawan resign ({emailWarnings.length})
                      </label>
                    ) : null}
                  </div>
                ) : null}
                {changes.length > 0 ? (
                  <ul className="max-h-48 space-y-0.5 overflow-y-auto rounded-lg border p-3 text-xs text-gray-600">
                    {changes.slice(0, 50).map((c, i) => (
                      <li key={i}>{changeText(c)}</li>
                    ))}
                  </ul>
                ) : null}
              </div>
            ) : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              Batal
            </Button>
            <Button disabled={!canSave} onClick={save}>
              Simpan
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Timpa data karyawan?"
        confirmLabel="Ya, timpa data"
        loading={busy}
        variant="default"
        onConfirm={() => void commit()}
      >
        <div className="space-y-2 text-sm">
          <p>{preview?.summary.update ?? 0} karyawan akan diperbarui</p>
          <ul className="max-h-48 space-y-0.5 overflow-y-auto text-xs text-gray-600">
            {changes.slice(0, 50).map((c, i) => (
              <li key={i}>{changeText(c)}</li>
            ))}
          </ul>
          <p className="text-xs text-gray-500">Sel kosong di file tidak mengubah data.</p>
        </div>
      </ConfirmDialog>
    </>
  );
}
