import { cn } from "@/lib/utils";
import { LATENESS_SHORT, LATENESS_TONE } from "@/lib/hris/lateness-view";
import type { LatenessLevel } from "../types";

/** Badge tingkat keterlambatan; `short` memakai label pendek (chip, daftar tim). */
export function LatenessLevelBadge({ level, short, className }: { level: LatenessLevel; short?: boolean; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex max-w-full items-center rounded-full px-2.5 py-1 text-xs font-semibold leading-tight ring-1",
        LATENESS_TONE[level.code].badge,
        className
      )}
    >
      <span className="min-w-0 break-words">{short ? LATENESS_SHORT[level.code] : level.label}</span>
    </span>
  );
}
