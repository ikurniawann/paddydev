import type { TripListFilter } from "./types";

export const businessTripQueryKeys = {
  all: ["hris", "business-trips"] as const,
  list: (filter: TripListFilter) => ["hris", "business-trips", "list", filter] as const,
};
