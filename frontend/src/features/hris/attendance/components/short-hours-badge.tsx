import { Clock } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { SHORT_HOURS_LABEL, shortHoursDetail } from "@/lib/hris/short-hours";

/** Label AB-05 pada baris absensi (EPIC-065); tidak dirender bila baris tidak berlabel. */
export function ShortHoursBadge({ isShort, minHours }: { isShort?: boolean | null; minHours?: string | number | null }) {
  if (!isShort) return null;
  return (
    <Badge className="max-w-full gap-1 bg-orange-100 text-orange-700" title={shortHoursDetail(minHours)}>
      <Clock className="h-3 w-3 shrink-0" />
      {SHORT_HOURS_LABEL}
    </Badge>
  );
}
