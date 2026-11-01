"use client";

import { useId, useState } from "react";
import { ChevronDown, ScrollText } from "lucide-react";
import { cn } from "@/lib/utils";
import { LATENESS_POLICY } from "@/lib/hris/lateness-view";

/** Bagian lipat "Ketentuan Keterlambatan & Absensi" (teks perusahaan apa adanya). */
export function LatenessPolicy() {
  const [open, setOpen] = useState(false);
  const id = useId();
  return (
    <div className="rounded-xl border border-gray-200 bg-gray-50/60">
      <button
        type="button"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen((v) => !v)}
        className="flex min-h-11 w-full items-center gap-2 px-3 py-2 text-left text-sm font-semibold text-gray-800"
      >
        <ScrollText className="h-4 w-4 shrink-0 text-gray-500" />
        <span className="min-w-0 flex-1">Ketentuan Keterlambatan & Absensi</span>
        <ChevronDown className={cn("h-4 w-4 shrink-0 text-gray-500 transition-transform", open && "rotate-180")} />
      </button>
      {open ? (
        <div id={id} className="space-y-3 border-t border-gray-200 px-3 py-3 text-sm text-gray-700">
          <div>
            <p className="font-semibold text-gray-900">{LATENESS_POLICY.title}</p>
            <p className="mt-1">{LATENESS_POLICY.lead}</p>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              {LATENESS_POLICY.levels.map((t) => (
                <li key={t}>{t}</li>
              ))}
            </ul>
          </div>
          <div>
            <p>{LATENESS_POLICY.intro}</p>
            <ol className="mt-2 list-decimal space-y-1 pl-5">
              {LATENESS_POLICY.steps.map((t) => (
                <li key={t}>{t}</li>
              ))}
            </ol>
          </div>
        </div>
      ) : null}
    </div>
  );
}
