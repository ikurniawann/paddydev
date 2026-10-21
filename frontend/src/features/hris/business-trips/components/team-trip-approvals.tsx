"use client";

import { UserGroupIcon } from "@heroicons/react/24/outline";
import { Button } from "@/components/ui/button";
import { tripRangeLabel } from "@/lib/hris/business-trips";
import { useBusinessTrips } from "../queries";
import { useTripDecision } from "../use-trip-decision";

/**
 * Antrean Perjalanan Dinas anggota tim untuk atasan langsung (EPIC-062),
 * dipakai halaman Perjalanan Dinas dan Persetujuan Tim (EPIC-063 R7).
 * Tidak dirender bila kosong.
 */
export function TeamTripApprovals() {
  const team = useBusinessTrips({ scope: "approvals", status: "pending" }).data ?? [];
  const { request, busyId, dialog } = useTripDecision();
  if (team.length === 0) return null;
  return (
    <div className="rounded-xl border border-sky-200 bg-sky-50/70 p-5 shadow-sm">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-sky-800">
        <UserGroupIcon className="h-4 w-4" /> Persetujuan Perjalanan Dinas Tim
      </h3>
      <ul className="mt-3 divide-y divide-sky-100">
        {team.map((trip) => (
          <li key={trip.id} className="flex flex-wrap items-center justify-between gap-3 py-2.5">
            <div className="min-w-0">
              <p className="text-sm font-medium text-gray-900">
                {trip.employee?.full_name ?? "—"} · {tripRangeLabel(trip.start_date, trip.end_date)}
              </p>
              <p className="truncate text-xs text-gray-600">
                {trip.destination} — {trip.purpose}
              </p>
            </div>
            <div className="flex shrink-0 gap-2">
              <Button size="sm" disabled={busyId === trip.id} onClick={() => request(trip, "approve")}>
                Setujui
              </Button>
              <Button size="sm" variant="outline" disabled={busyId === trip.id} onClick={() => request(trip, "reject")}>
                Tolak
              </Button>
            </div>
          </li>
        ))}
      </ul>
      {dialog}
    </div>
  );
}
