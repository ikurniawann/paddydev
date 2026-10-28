"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  fetchClockPlacements,
  fetchEmployeePlacements,
  fetchPlacementOverview,
  fetchStoreBranches,
  searchPlacements,
} from "./api";
import { placementQueryKeys } from "./query-keys";
import type { PlacementOverviewFilter, PlacementSearchFilter } from "./types";

export const useStoreBranches = () =>
  useQuery({ queryKey: placementQueryKeys.branches(), queryFn: fetchStoreBranches, staleTime: 5 * 60_000 });

export const useEmployeePlacements = (employeeId: string) =>
  useQuery({
    queryKey: placementQueryKeys.employee(employeeId),
    queryFn: () => fetchEmployeePlacements(employeeId),
    enabled: !!employeeId,
  });

export const usePlacementOverview = (filter: PlacementOverviewFilter) =>
  useQuery({ queryKey: placementQueryKeys.overview(filter), queryFn: () => fetchPlacementOverview(filter) });

/** Hasil tetap tampil saat berpindah halaman/filter sampai data baru datang. */
export const usePlacementSearch = (filter: PlacementSearchFilter) =>
  useQuery({
    queryKey: placementQueryKeys.search(filter),
    queryFn: () => searchPlacements(filter),
    placeholderData: keepPreviousData,
  });

export const useClockPlacements = (employeeId?: string, enabled = true) =>
  useQuery({
    queryKey: placementQueryKeys.clock(employeeId),
    queryFn: () => fetchClockPlacements(employeeId),
    enabled,
  });
