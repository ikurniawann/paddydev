import { Badge } from "@/components/ui/badge";
import { TRIP_STATUS_META, type TripStatus } from "@/lib/hris/business-trips";

export function TripStatusBadge({ status }: { status: TripStatus }) {
  const meta = TRIP_STATUS_META[status];
  return <Badge className={meta.className}>{meta.label}</Badge>;
}
