import { cn } from "@/lib/utils";
import { formatLateMinutes, LATENESS_TONE, nextLevelHint } from "@/lib/hris/lateness-view";
import type { LatenessStats } from "../types";
import { LatenessLevelBadge } from "./lateness-level-badge";

/** Angka besar + tingkat sesuai ketentuan + petunjuk tingkat berikutnya. */
export function LatenessSummary({ stats, compact }: { stats: LatenessStats; compact?: boolean }) {
  const tone = LATENESS_TONE[stats.level.code];
  const hint = nextLevelHint(stats.next);
  return (
    <div className="flex min-w-0 items-center gap-3 sm:gap-4">
      <div
        className={cn(
          "flex shrink-0 flex-col items-center justify-center rounded-2xl border text-center",
          compact ? "h-16 w-16" : "h-20 w-20 sm:h-24 sm:w-24",
          tone.soft
        )}
      >
        <span className={cn("font-bold leading-none tabular-nums", compact ? "text-2xl" : "text-4xl", tone.text)}>
          {stats.count}
        </span>
        <span className="mt-1 text-[10px] font-medium leading-tight text-gray-600 sm:text-[11px]">kali terlambat</span>
      </div>
      <div className="min-w-0 flex-1 space-y-1.5">
        <p className="text-xs text-gray-500">{stats.count === 0 ? "Status" : "Sesuai ketentuan"}</p>
        <LatenessLevelBadge level={stats.level} />
        {hint ? <p className="text-xs leading-snug text-gray-600">{hint}</p> : null}
        {!compact && stats.total_minutes > 0 ? (
          <p className="text-xs text-gray-500">Total terlambat {formatLateMinutes(stats.total_minutes)}</p>
        ) : null}
      </div>
    </div>
  );
}
