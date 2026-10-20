"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchBusinessTrips } from "./api";
import { businessTripQueryKeys } from "./query-keys";
import type { TripListFilter } from "./types";

export const useBusinessTrips = (filter: TripListFilter = {}) =>
  useQuery({ queryKey: businessTripQueryKeys.list(filter), queryFn: () => fetchBusinessTrips(filter) });
