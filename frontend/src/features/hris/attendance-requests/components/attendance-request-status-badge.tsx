import { Badge } from "@/components/ui/badge";
import { ATTENDANCE_REQUEST_STATUS_META, type AttendanceRequestStatus } from "@/lib/hris/attendance-requests";

export function AttendanceRequestStatusBadge({ status }: { status: AttendanceRequestStatus }) {
  const meta = ATTENDANCE_REQUEST_STATUS_META[status];
  return <Badge className={meta.className}>{meta.label}</Badge>;
}
