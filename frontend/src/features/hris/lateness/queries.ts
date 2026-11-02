"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { fetchMyLateness, fetchTeamLateness } from "./api";
import { latenessQueryKeys } from "./query-keys";

export const useMyLateness = (back = 0) =>
  useQuery({ queryKey: latenessQueryKeys.mine(back), queryFn: () => fetchMyLateness(back), placeholderData: keepPreviousData });

export const useTeamLateness = (back = 0) =>
  useQuery({ queryKey: latenessQueryKeys.team(back), queryFn: () => fetchTeamLateness(back), placeholderData: keepPreviousData });
