"use client";

import { useRef, useState } from "react";
import { Loader2, Paperclip, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  MAX_OVERTIME_ATTACHMENTS,
  MAX_OVERTIME_ATTACHMENT_BYTES,
  OVERTIME_ATTACHMENT_ACCEPT,
} from "@/lib/hris/overtime-rules";
import { uploadOvertimeAttachment } from "../api";

/** Dokumentasi lembur (EPIC-066): 1–3 file PDF/JPG/PNG/WebP ≤ 5 MB, diunggah satu per satu. */
export function OvertimeAttachmentsField({ value, onChange }: { value: string[]; onChange: (paths: string[]) => void }) {
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function add(files: FileList | null) {
    if (!files || files.length === 0) return;
    setError("");
    const room = MAX_OVERTIME_ATTACHMENTS - value.length;
    const picked = Array.from(files).slice(0, room);
    if (files.length > room) setError(`Maksimal ${MAX_OVERTIME_ATTACHMENTS} dokumentasi lembur`);
    const tooBig = picked.find((f) => f.size > MAX_OVERTIME_ATTACHMENT_BYTES);
    if (tooBig) {
      setError(`${tooBig.name}: ukuran dokumentasi maksimal 5 MB`);
      return;
    }
    setBusy(true);
    try {
      const paths: string[] = [];
      for (const f of picked) paths.push(await uploadOvertimeAttachment(f));
      onChange([...value, ...paths]);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Gagal mengunggah dokumentasi");
    } finally {
      setBusy(false);
      if (input.current) input.current.value = "";
    }
  }

  return (
    <div className="space-y-1.5">
      <span className="block text-xs font-medium text-gray-600">Dokumentasi (wajib, 1–3 file PDF/foto)</span>
      <ul className="space-y-1">
        {value.map((p, i) => (
          <li key={p} className="flex items-center justify-between rounded border border-gray-200 px-2 py-1 text-xs">
            <span className="truncate">Dokumentasi {i + 1} ({p.split(".").pop()?.toUpperCase()})</span>
            <button type="button" aria-label={`Hapus dokumentasi ${i + 1}`} onClick={() => onChange(value.filter((x) => x !== p))}>
              <X className="h-3.5 w-3.5" />
            </button>
          </li>
        ))}
      </ul>
      {value.length < MAX_OVERTIME_ATTACHMENTS && (
        <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => input.current?.click()}>
          {busy ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" /> : <Paperclip className="mr-1.5 h-4 w-4" />}
          Tambah dokumentasi
        </Button>
      )}
      <input
        ref={input}
        type="file"
        accept={OVERTIME_ATTACHMENT_ACCEPT}
        multiple
        className="hidden"
        aria-label="Pilih file dokumentasi lembur"
        onChange={(e) => void add(e.target.files)}
      />
      {error && <p role="alert" className="text-xs text-red-600">{error}</p>}
    </div>
  );
}
