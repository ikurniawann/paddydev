"use client";

import { useEffect, useRef, useState } from "react";
import { Loader2, Paperclip, X } from "lucide-react";
import { Button } from "@/components/ui/button";

export const PRIVATE_ATTACHMENT_ACCEPT = "application/pdf,image/jpeg,image/png,image/webp";
export const MAX_PRIVATE_ATTACHMENT_BYTES = 5 * 1024 * 1024;

interface PrivateAttachmentsFieldProps {
  value: string[];
  onChange: (paths: string[]) => void;
  /** Dipanggil saat unggahan mulai/selesai agar form tidak dikirim selagi mengunggah. */
  onBusyChange?: (busy: boolean) => void;
  /** Unggah satu file, kembalikan path privat. */
  upload: (file: File) => Promise<string>;
  /** Judul bidang, mis. "Dokumentasi (wajib, 1–3 file PDF/foto)". */
  label: string;
  /** Kata benda huruf kecil untuk butir & tombol, mis. "dokumentasi" → "Dokumentasi 1", "Tambah dokumentasi". */
  noun: string;
  /** aria-label input file tersembunyi. */
  inputLabel: string;
  /** Pesan bila memilih melebihi batas. */
  tooManyMessage: string;
  max?: number;
  disabled?: boolean;
}

/**
 * Lampiran privat (EPIC-066, umum sejak EPIC-076): file PDF/JPG/PNG/WebP ≤ 5 MB,
 * diunggah satu per satu sampai `max` file.
 */
export function PrivateAttachmentsField({
  value,
  onChange,
  onBusyChange,
  upload,
  label,
  noun,
  inputLabel,
  tooManyMessage,
  max = 3,
  disabled = false,
}: PrivateAttachmentsFieldProps) {
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  // Daftar terbaru saat unggahan selesai (bisa berubah selama menunggu), dan
  // apakah bidang masih terpasang: hasil unggahan bidang yang sudah diganti
  // (mis. karyawan lain dipilih) dibuang.
  const latest = useRef(value);
  const alive = useRef(true);
  useEffect(() => {
    latest.current = value;
  }, [value]);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  const title = noun.charAt(0).toUpperCase() + noun.slice(1);

  async function add(files: FileList | null) {
    if (!files || files.length === 0) return;
    setError("");
    const room = max - value.length;
    const picked = Array.from(files).slice(0, room);
    if (files.length > room) setError(tooManyMessage);
    const tooBig = picked.find((f) => f.size > MAX_PRIVATE_ATTACHMENT_BYTES);
    if (tooBig) {
      setError(`${tooBig.name}: ukuran ${noun} maksimal 5 MB`);
      return;
    }
    setBusy(true);
    onBusyChange?.(true);
    const paths: string[] = [];
    try {
      for (const f of picked) paths.push(await upload(f));
    } catch (e) {
      if (alive.current) setError(e instanceof Error ? e.message : `Gagal mengunggah ${noun}`);
    } finally {
      // File yang sudah terunggah tetap masuk walau file berikutnya gagal.
      if (alive.current && paths.length > 0) onChange([...latest.current, ...paths]);
      if (alive.current) setBusy(false);
      onBusyChange?.(false);
      if (input.current) input.current.value = "";
    }
  }

  return (
    <div className="space-y-1.5">
      <span className="block text-xs font-medium text-gray-600">{label}</span>
      <ul className="space-y-1">
        {value.map((p, i) => (
          <li key={p} className="flex items-center justify-between rounded border border-gray-200 px-2 py-1 text-xs">
            <span className="truncate">
              {title} {i + 1} ({p.split(".").pop()?.toUpperCase()})
            </span>
            <button
              type="button"
              aria-label={`Hapus ${noun} ${i + 1}`}
              disabled={busy}
              onClick={() => onChange(value.filter((x) => x !== p))}
            >
              <X className="h-3.5 w-3.5" />
            </button>
          </li>
        ))}
      </ul>
      {value.length < max && (
        <Button type="button" variant="outline" size="sm" disabled={busy || disabled} onClick={() => input.current?.click()}>
          {busy ? <Loader2 className="mr-1.5 h-4 w-4 animate-spin" /> : <Paperclip className="mr-1.5 h-4 w-4" />}
          Tambah {noun}
        </Button>
      )}
      <input
        ref={input}
        type="file"
        accept={PRIVATE_ATTACHMENT_ACCEPT}
        multiple
        className="hidden"
        aria-label={inputLabel}
        onChange={(e) => void add(e.target.files)}
      />
      {error && <p role="alert" className="text-xs text-red-600">{error}</p>}
    </div>
  );
}
