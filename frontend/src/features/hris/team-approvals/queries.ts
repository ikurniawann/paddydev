"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchTeamApprovals } from "./api";
import { teamApprovalsQueryKeys } from "./query-keys";

export const useTeamApprovals = () =>
  useQuery({ queryKey: teamApprovalsQueryKeys.all, queryFn: fetchTeamApprovals });
