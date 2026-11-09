"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchTeamApprovals, fetchTeamHistory } from "./api";
import { teamApprovalsQueryKeys } from "./query-keys";

export const useTeamApprovals = () =>
  useQuery({ queryKey: teamApprovalsQueryKeys.all, queryFn: fetchTeamApprovals });

/** EPIC-090: riwayat cuti & lembur bawahan langsung. */
export const useTeamHistory = () =>
  useQuery({ queryKey: teamApprovalsQueryKeys.history(), queryFn: fetchTeamHistory });
