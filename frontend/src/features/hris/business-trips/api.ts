import { apiGet, apiPost, buildListUrl } from "@/lib/api-client";
import type { BusinessTrip, TripAction, TripListFilter, TripPayload } from "./types";

export const fetchBusinessTrips = (filter: TripListFilter = {}) =>
  apiGet<{ data: BusinessTrip[] }>(
    buildListUrl("/api/hris/business-trips", { scope: filter.scope, status: filter.status, employee_id: filter.employee_id })
  ).then((res) => res.data ?? []);

export const createBusinessTrip = (payload: TripPayload) =>
  apiPost<{ data: BusinessTrip; message?: string }>("/api/hris/business-trips", payload);

export const decideBusinessTrip = (id: string, action: TripAction, rejectionReason?: string) =>
  apiPost<{ data: BusinessTrip; message?: string }>(`/api/hris/business-trips/${id}/decide`, {
    action,
    rejection_reason: rejectionReason,
  });
