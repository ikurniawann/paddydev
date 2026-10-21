"use client";

import { useState } from "react";
import { BriefcaseIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { tripRangeLabel } from "@/lib/hris/business-trips";
import { useEssMe } from "@/features/hris/ess/queries";
import { EssLoading, EssNotLinked } from "@/features/hris/ess/components/ess-states";
import { useBusinessTrips } from "../queries";
import { useTripDecision } from "../use-trip-decision";
import { TripRequestDialog } from "./trip-request-dialog";
import { TeamTripApprovals } from "./team-trip-approvals";
import { TripStatusBadge } from "./trip-status-badge";

/**
 * ESS → Perjalanan Dinas (/dashboard/me/perjalanan-dinas, EPIC-062): ajukan
 * rentang tanggal dinas; selama disetujui, absen tidak dicek radius toko.
 * Atasan langsung melihat antrean persetujuan timnya di halaman yang sama.
 */
export function EssPerjalananDinasPage() {
  const { data: me, isLoading } = useEssMe();
  const mine = useBusinessTrips({ employee_id: "me" }).data ?? [];
  const { request, busyId, dialog: decisionDialog } = useTripDecision();
  const [dialog, setDialog] = useState(false);

  if (isLoading) return <EssLoading />;
  if (!me?.employee) return <EssNotLinked feature="Perjalanan Dinas" />;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between border-b border-gray-200/70 pb-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Perjalanan Dinas</h1>
          <p className="text-sm text-gray-500">Selama dinas disetujui, absen tidak dicek radius toko</p>
        </div>
        <Button className="gap-2" onClick={() => setDialog(true)}>
          <BriefcaseIcon className="h-4 w-4" /> Ajukan
        </Button>
      </div>

      <TeamTripApprovals />

      <div className="rounded-xl border border-gray-200/70 bg-white shadow-sm">
        {mine.length === 0 ? (
          <p className="p-6 text-center text-sm text-gray-500">Belum ada pengajuan Perjalanan Dinas.</p>
        ) : (
          <ul className="divide-y divide-gray-100">
            {mine.map((trip) => (
              <li key={trip.id} className="flex flex-wrap items-center justify-between gap-3 p-4">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-gray-900">{tripRangeLabel(trip.start_date, trip.end_date)}</p>
                  <p className="truncate text-xs text-gray-600">
                    {trip.destination} — {trip.purpose}
                  </p>
                  {trip.rejection_reason && <p className="text-xs text-red-600">Ditolak: {trip.rejection_reason}</p>}
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <TripStatusBadge status={trip.status} />
                  {trip.status === "pending" && (
                    <Button size="sm" variant="ghost" disabled={busyId === trip.id} onClick={() => request(trip, "cancel")}>
                      Batalkan
                    </Button>
                  )}
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>

      <TripRequestDialog open={dialog} onOpenChange={setDialog} />
      {decisionDialog}
    </div>
  );
}
