"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  fetchClockPlacements,
  fetchEmployeePlacements,
  fetchStoreBranches,
  searchPlacements,
} from "./api";
import { placementQueryKeys } from "./query-keys";
import type { PlacementSearchFilter } from "./types";

/** `enabled` false = tidak diminta (mode toko memakai toko dari akses pengatur). */
export const useStoreBranches = (enabled = true) =>
  useQuery({ queryKey: placementQueryKeys.branches(), queryFn: fetchStoreBranches, staleTime: 5 * 60_000, enabled });

export const useEmployeePlacements = (employeeId: string) =>
  useQuery({
    queryKey: placementQueryKeys.employee(employeeId),
    queryFn: () => fetchEmployeePlacements(employeeId),
    enabled: !!employeeId,
  });

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
