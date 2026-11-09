"use client";

import { useId, useState } from "react";
import { ChevronDown, ScrollText } from "lucide-react";
import { cn } from "@/lib/utils";
import { LATENESS_POLICY } from "@/lib/hris/lateness-view";
import { WORK_HOURS } from "@/lib/hris/work-hours";

/** Nilai baris jam kerja yang lebih panjang dari ini ditampilkan di bawah labelnya. */
const LONG_VALUE = 24;

/**
 * Bagian lipat "Jam Kerja & Ketentuan Absensi" (teks perusahaan apa adanya,
 * EPIC-077/078). Tingkat teguran/SP tidak ditampilkan ke karyawan (EPIC-094).
 */
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
        <span className="min-w-0 flex-1">Jam Kerja & Ketentuan Absensi</span>
        <ChevronDown className={cn("h-4 w-4 shrink-0 text-gray-500 transition-transform", open && "rotate-180")} />
      </button>
      <div id={id} hidden={!open} className="space-y-3 border-t border-gray-200 px-3 py-3 text-sm text-gray-700">
        <div>
          <p className="font-semibold text-gray-900">{WORK_HOURS.title}</p>
          <div className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {WORK_HOURS.groups.map((g) => (
              <section key={g.name} aria-label={g.name} className="rounded-lg border border-gray-200 bg-white p-3">
                <p className="font-semibold text-gray-900">{g.name}</p>
                <p className="mt-0.5 text-xs text-gray-500">{g.scope}</p>
                <dl className="mt-2 space-y-1">
                  {g.rows.map(([k, v]) => (
                    // Nilai panjang ditaruh di bawah labelnya (bisa membungkus) agar tidak meluap di 360 px.
                    <div
                      key={k}
                      className={cn(
                        "grid items-baseline gap-x-3",
                        v.length > LONG_VALUE ? "grid-cols-1" : "grid-cols-[minmax(0,1fr)_auto]",
                      )}
                    >
                      <dt className="text-gray-600">{k}</dt>
                      <dd
                        className={cn(
                          "font-medium tabular-nums text-gray-900",
                          v.length > LONG_VALUE ? "break-words" : "whitespace-nowrap text-right",
                        )}
                      >
                        {v}
                      </dd>
                    </div>
                  ))}
                </dl>
              </section>
            ))}
          </div>
          <div className="mt-2 space-y-2">
            {WORK_HOURS.notes.map((t) => (
              <p key={t}>{t}</p>
            ))}
          </div>
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
    </div>
  );
}
