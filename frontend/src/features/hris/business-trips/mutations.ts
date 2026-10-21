"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { attendanceQueryKeys } from "@/features/hris/attendance/query-keys";
import { createBusinessTrip, decideBusinessTrip } from "./api";
import { businessTripQueryKeys } from "./query-keys";
import type { TripAction, TripPayload } from "./types";

// Persetujuan bisa mengubah status geofence absensi (R8): segarkan absensi juga.
function useInvalidateTrips() {
  const qc = useQueryClient();
  return () => {
    void qc.invalidateQueries({ queryKey: businessTripQueryKeys.all });
    void qc.invalidateQueries({ queryKey: attendanceQueryKeys.all });
  };
}

export function useCreateBusinessTrip() {
  const onSuccess = useInvalidateTrips();
  return useMutation({ mutationFn: (payload: TripPayload) => createBusinessTrip(payload), retry: false, onSuccess });
}

/**
 * Keputusan menyegarkan antrean, absensi, dan badge menu (nav-badges), juga saat
 * server menolak (mis. 409 "sudah diproses orang lain") supaya antrean basi hilang.
 */
export function useDecideBusinessTrip() {
  const qc = useQueryClient();
  const invalidateTrips = useInvalidateTrips();
  return useMutation({
    mutationFn: (vars: { id: string; action: TripAction; rejectionReason?: string }) =>
      decideBusinessTrip(vars.id, vars.action, vars.rejectionReason),
    retry: false,
    onSettled: () => {
      invalidateTrips();
      void qc.invalidateQueries({ queryKey: ["hris", "nav-badges"] });
    },
  });
}
