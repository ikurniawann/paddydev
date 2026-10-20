import { MapPin } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { GEOFENCE_META, geofenceLabel, isGeofenceStatus } from "@/lib/hris/geofence";

/** Badge status geofence satu clock-in/out; tidak dirender bila absen tidak dinilai. */
export function GeofenceBadge({
  status,
  distanceM,
  prefix,
}: {
  status: string | null | undefined;
  distanceM?: number | null;
  prefix?: string;
}) {
  if (!isGeofenceStatus(status)) return null;
  return (
    <Badge className={`gap-1 ${GEOFENCE_META[status].className}`}>
      <MapPin className="h-3 w-3" />
      {prefix ? `${prefix}: ` : ""}
      {geofenceLabel(status, distanceM)}
    </Badge>
  );
}
