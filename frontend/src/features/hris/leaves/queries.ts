"use client";

import { useQuery } from "@tanstack/react-query";
import { leavesQueryKeys } from "./query-keys";
import { fetchLeaveList, fetchLeaveEmployees, fetchLeaveQuota } from "./api";
import type { LeaveListParams } from "./types";

export const useLeaveList = (params?: LeaveListParams) =>
  useQuery({
    queryKey: leavesQueryKeys.list(params),
    queryFn: () => fetchLeaveList(params),
  });

export const useLeaveEmployees = () =>
  useQuery({
    queryKey: leavesQueryKeys.employees(),
    queryFn: fetchLeaveEmployees,
  });

export const useLeaveQuota = (month: string, employeeId?: string, enabled = true) =>
  useQuery({
    queryKey: leavesQueryKeys.quota(month, employeeId),
    queryFn: () => fetchLeaveQuota(month, employeeId),
    enabled: enabled && /^\d{4}-\d{2}$/.test(month),
  });
