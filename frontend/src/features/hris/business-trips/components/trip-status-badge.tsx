import { Badge } from "@/components/ui/badge";
import { TRIP_STATUS_META, type TripStatus } from "@/lib/hris/business-trips";
import { SPV_APPROVED_CLASS, SPV_APPROVED_LABEL, spvStageDone } from "@/lib/hris/tiered-approval";

/** spvApprovedAt: tahap SPV sudah lewat — pending berarti menunggu Manager (EPIC-092). */
export function TripStatusBadge({ status, spvApprovedAt }: { status: TripStatus; spvApprovedAt?: string | null }) {
  if (spvStageDone({ status, spv_approved_at: spvApprovedAt })) {
    return <Badge className={SPV_APPROVED_CLASS}>{SPV_APPROVED_LABEL}</Badge>;
  }
  const meta = TRIP_STATUS_META[status];
  return <Badge className={meta.className}>{meta.label}</Badge>;
}
