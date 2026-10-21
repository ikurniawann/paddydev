"use client";

import { useQuery } from "@tanstack/react-query";
import { essQueryKeys } from "./query-keys";
import {
  fetchAnnouncement,
  fetchAnnouncementFeed,
  fetchEssBeranda,
  fetchEssMe,
  fetchMyLeaves,
  fetchMyLoans,
  fetchMyOvertime,
  fetchMyPayslips,
  fetchOvertimeDayType,
  fetchMyTeam,
  fetchTeamLoanApprovals,
  fetchTodayAttendance,
} from "./api";

export const useEssMe = () => useQuery({ queryKey: essQueryKeys.me(), queryFn: fetchEssMe });

export const useEssBeranda = () =>
  useQuery({ queryKey: essQueryKeys.beranda(), queryFn: fetchEssBeranda });

export const useTodayAttendance = () =>
  useQuery({ queryKey: essQueryKeys.todayAttendance(), queryFn: fetchTodayAttendance });

export const useMyLeaves = () =>
  useQuery({ queryKey: essQueryKeys.leaves(), queryFn: fetchMyLeaves });

export const useMyOvertime = () =>
  useQuery({ queryKey: essQueryKeys.overtime(), queryFn: fetchMyOvertime });

export const useOvertimeDayType = (date: string) =>
  useQuery({
    queryKey: essQueryKeys.overtimeDayType(date),
    queryFn: () => fetchOvertimeDayType(date),
    enabled: /^\d{4}-\d{2}-\d{2}$/.test(date),
  });

export const useMyLoans = () => useQuery({ queryKey: essQueryKeys.loans(), queryFn: fetchMyLoans });

export const useTeamLoanApprovals = () =>
  useQuery({ queryKey: [...essQueryKeys.loans(), "approvals"], queryFn: fetchTeamLoanApprovals });

export const useMyPayslips = () =>
  useQuery({ queryKey: essQueryKeys.payslips(), queryFn: fetchMyPayslips });

export const useAnnouncementFeed = () =>
  useQuery({ queryKey: essQueryKeys.announcements(), queryFn: fetchAnnouncementFeed });

export const useAnnouncement = (id: string | null) =>
  useQuery({
    queryKey: essQueryKeys.announcement(id ?? ""),
    queryFn: () => fetchAnnouncement(id ?? ""),
    enabled: !!id,
  });

export const useMyTeam = () => useQuery({ queryKey: essQueryKeys.team(), queryFn: fetchMyTeam });
