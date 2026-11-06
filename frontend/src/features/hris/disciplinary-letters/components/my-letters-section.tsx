"use client";

import { FileWarning, Paperclip } from "lucide-react";
import { formatLetterDate, letterAttachmentUrl } from "../api";
import { useMyLetters } from "../queries";

/**
 * ESS Absensi → "Surat dari HR" (EPIC-094 R12): Surat Teguran/SP yang
 * diterbitkan HR untuk karyawan ini dan belum dibatalkan. Tidak dirender bila
 * tidak ada surat (atau selama memuat/gagal). Lonceng surat menaut ke #surat.
 */
export function MyLettersSection() {
  const letters = useMyLetters().data ?? [];
  if (letters.length === 0) return null;
  return (
    <section id="surat" className="scroll-mt-24 space-y-3 rounded-2xl border border-gray-200/70 bg-white p-4 shadow-sm sm:p-5">
      <h2 className="flex items-center gap-2 text-base font-semibold text-gray-900">
        <FileWarning className="h-5 w-5 text-amber-600" />
        Surat dari HR
      </h2>
      <ul className="space-y-2">
        {letters.map((l) => (
          <li key={l.id} className="rounded-xl border border-amber-100 bg-amber-50/60 p-3 text-sm">
            <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
              <p className="font-semibold text-gray-900">{l.label}</p>
              <p className="text-xs text-gray-500">{formatLetterDate(l.issued_date)}</p>
            </div>
            {l.notes ? <p className="mt-1 whitespace-pre-line break-words text-gray-700">{l.notes}</p> : null}
            {l.attachment_path ? (
              <a
                href={letterAttachmentUrl(l.attachment_path)}
                target="_blank"
                rel="noreferrer"
                className="mt-2 inline-flex items-center gap-1 text-pink-600 underline"
              >
                <Paperclip className="h-3.5 w-3.5" />
                Lihat lampiran
              </a>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
