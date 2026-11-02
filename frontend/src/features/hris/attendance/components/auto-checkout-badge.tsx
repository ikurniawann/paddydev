import { Timer } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { AUTO_CHECKOUT_DETAIL, AUTO_CHECKOUT_LABEL } from "@/lib/hris/short-hours";

/** Penanda EPIC-080 R4: jam pulang diisi sistem di jam selesai shift; tidak dirender bila bukan checkout otomatis. */
export function AutoCheckoutBadge({ auto }: { auto?: boolean | null }) {
  if (!auto) return null;
  return (
    <Badge className="max-w-full gap-1 bg-sky-100 text-sky-700" title={AUTO_CHECKOUT_DETAIL}>
      <Timer className="h-3 w-3 shrink-0" />
      {AUTO_CHECKOUT_LABEL}
    </Badge>
  );
}
