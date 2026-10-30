"use client";

import type { ReactNode } from "react";
import { RequestDetailDialog } from "@/components/hris/request-detail-dialog";
import { formatDateTime } from "@/lib/format";
import { tripDays, tripRangeLabel } from "@/lib/hris/business-trips";
import type { BusinessTrip } from "../types";
import { TripStatusBadge } from "./trip-status-badge";

/** Detail Perjalanan Dinas untuk atasan dan HRD (EPIC-072): keperluan utuh sebelum memutus. */
export function TripDetailDialog({
  trip,
  onClose,
  actions,
}: {
  trip: BusinessTrip;
  onClose: () => void;
  actions?: ReactNode;
}) {
  return (
    <RequestDetailDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title="Detail perjalanan dinas"
      employee={{ name: trip.employee?.full_name ?? "—", nip: trip.employee?.nip }}
      status={<TripStatusBadge status={trip.status} />}
      fields={[
        { label: "Tanggal", value: `${tripRangeLabel(trip.start_date, trip.end_date)} (${tripDays(trip.start_date, trip.end_date)} hari)` },
        { label: "Tujuan", value: trip.destination },
        { label: "Diajukan", value: formatDateTime(trip.created_at, "—") },
      ]}
      reason={{ label: "Keperluan", text: trip.purpose }}
      notes={
        trip.decided_by_name || trip.rejection_reason ? (
          <div className="space-y-1 text-xs text-gray-600">
            {trip.decided_by_name && <p>Diputus oleh {trip.decided_by_name}</p>}
            {trip.rejection_reason && (
              <p className="whitespace-pre-line break-words text-red-600">Alasan penolakan: {trip.rejection_reason}</p>
            )}
          </div>
        ) : undefined
      }
      actions={actions}
    />
  );
}
