import { cn } from "@/lib/utils";
import { LATENESS_CODES, LATENESS_SHORT, LATENESS_STEP_RANGE, LATENESS_TONE } from "@/lib/hris/lateness-view";
import type { LatenessCode } from "../types";

/**
 * Tangga 5 tingkat (0 · 1–2 · 3–4 · 5–6 · >6): segmen sampai tingkat saat ini
 * diwarnai, tingkat saat ini ditebalkan.
 */
export function LatenessLadder({ code }: { code: LatenessCode }) {
  const current = LATENESS_CODES.indexOf(code);
  return (
    <ol className="grid grid-cols-5 gap-1" aria-label="Tingkat keterlambatan">
      {LATENESS_CODES.map((c, i) => {
        const active = i === current;
        return (
          <li key={c} className="min-w-0" aria-current={active ? "step" : undefined}>
            <span
              className={cn("block h-2 rounded-full", i <= current ? LATENESS_TONE[code].bar : "bg-gray-200")}
              aria-hidden
            />
            <span
              className={cn(
                "mt-1.5 block text-center text-[10px] leading-tight sm:text-xs",
                active ? cn("font-bold", LATENESS_TONE[code].text) : "text-gray-500"
              )}
            >
              <span className="block tabular-nums">{LATENESS_STEP_RANGE[c]}x</span>
              <span className="block break-words">{LATENESS_SHORT[c]}</span>
            </span>
          </li>
        );
      })}
    </ol>
  );
}
