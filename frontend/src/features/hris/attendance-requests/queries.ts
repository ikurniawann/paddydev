"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchAttendanceRequests, fetchMyAttendanceGaps, fetchMyAttendanceRequests } from "./api";
import { attendanceRequestQueryKeys } from "./query-keys";
import type { AttendanceRequestFilter } from "./types";

export const useMyAttendanceRequests = () =>
  useQuery({ queryKey: attendanceRequestQueryKeys.mine(), queryFn: fetchMyAttendanceRequests });

export const useAttendanceRequests = (filter: AttendanceRequestFilter = {}) =>
  useQuery({ queryKey: attendanceRequestQueryKeys.list(filter), queryFn: () => fetchAttendanceRequests(filter) });

export const useMyAttendanceGaps = () =>
  useQuery({ queryKey: attendanceRequestQueryKeys.gaps(), queryFn: fetchMyAttendanceGaps });
