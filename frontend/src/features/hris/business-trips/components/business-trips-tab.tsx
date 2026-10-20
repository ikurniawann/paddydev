"use client";

import { useMemo, useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { TRIP_STATUS_META, tripRangeLabel, type TripStatus } from "@/lib/hris/business-trips";
import { useActiveEmployees } from "@/features/hris/attendance/queries";
import { useEssMe } from "@/features/hris/ess/queries";
import { useBusinessTrips } from "../queries";
import { useTripDecision } from "../use-trip-decision";
import { TripRequestDialog } from "./trip-request-dialog";
import { TripStatusBadge } from "./trip-status-badge";

const ALL = "all";

/**
 * Absensi HRD → tab Perjalanan Dinas (EPIC-062): semua pengajuan, HRD bisa
 * menyetujui/menolak (cadangan atasan), membatalkan, dan mencatat dinas
 * karyawan (langsung disetujui).
 */
export function BusinessTripsTab() {
  const [status, setStatus] = useState<string>("pending");
  const [dialog, setDialog] = useState(false);
  const trips = useBusinessTrips(status === ALL ? {} : { status: status as TripStatus });
  const employees = useActiveEmployees().data;
  const employeeOptions = useMemo(
    () => (employees ?? []).map((e) => ({ value: e.id, label: e.nip ? `${e.full_name} (${e.nip})` : e.full_name })),
    [employees]
  );
  const myEmployeeId = useEssMe().data?.employee?.id;
  const { request, busyId, dialog: decisionDialog } = useTripDecision();

  const rows = trips.data ?? [];
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="w-48">
          <label className="mb-1 block text-xs font-medium text-gray-600">Status</label>
          <Select value={status} onValueChange={(v) => v && setStatus(v)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>Semua status</SelectItem>
              {(Object.keys(TRIP_STATUS_META) as TripStatus[]).map((s) => (
                <SelectItem key={s} value={s}>
                  {TRIP_STATUS_META[s].label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button onClick={() => setDialog(true)}>
          <Plus className="mr-1.5 h-4 w-4" /> Catat Perjalanan Dinas
        </Button>
      </div>
      <Card>
        <CardContent className="p-0">
          {rows.length === 0 ? (
            <p className="p-6 text-center text-sm text-gray-500">Tidak ada Perjalanan Dinas untuk filter ini.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50 text-left text-gray-600">
                    <th className="p-3">Karyawan</th>
                    <th className="p-3">Tanggal</th>
                    <th className="p-3">Tujuan & keperluan</th>
                    <th className="p-3">Status</th>
                    <th className="p-3 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((trip) => (
                    <tr key={trip.id} className="border-b border-gray-50 align-top">
                      <td className="p-3">
                        <p className="font-medium text-gray-900">{trip.employee?.full_name ?? "—"}</p>
                        {trip.employee?.nip && <p className="text-xs text-gray-400">{trip.employee.nip}</p>}
                      </td>
                      <td className="p-3 whitespace-nowrap text-gray-700">{tripRangeLabel(trip.start_date, trip.end_date)}</td>
                      <td className="p-3 text-gray-700">
                        <p>{trip.destination}</p>
                        <p className="text-xs text-gray-500">{trip.purpose}</p>
                      </td>
                      <td className="p-3">
                        <TripStatusBadge status={trip.status} />
                        {trip.decided_by_name && <p className="mt-1 text-xs text-gray-400">oleh {trip.decided_by_name}</p>}
                        {trip.rejection_reason && <p className="text-xs text-red-600">{trip.rejection_reason}</p>}
                      </td>
                      <td className="p-3 text-right whitespace-nowrap">
                        {trip.status === "pending" && trip.employee_id !== myEmployeeId && (
                          <>
                            <Button size="sm" disabled={busyId === trip.id} onClick={() => request(trip, "approve")}>
                              Setujui
                            </Button>{" "}
                            <Button size="sm" variant="outline" disabled={busyId === trip.id} onClick={() => request(trip, "reject")}>
                              Tolak
                            </Button>
                          </>
                        )}
                        {(trip.status === "pending" || trip.status === "approved") && (
                          <Button size="sm" variant="ghost" disabled={busyId === trip.id} onClick={() => request(trip, "cancel")}>
                            Batalkan
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
      <TripRequestDialog open={dialog} onOpenChange={setDialog} employees={employeeOptions} />
      {decisionDialog}
    </div>
  );
}
