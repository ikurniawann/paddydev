"use client";

import { useQuery } from "@tanstack/react-query";
import { fetchHolidayIndex } from "@/lib/hris/holidays-client";
import { attendanceQueryKeys } from "./query-keys";
import {
  fetchAttendanceAdjustments,
  fetchAttendancePeriod,
  fetchAttendancePeriods,
  fetchAttendanceSettings,
  previewAttendanceSettings,
  fetchActiveEmployees,
  fetchAttendanceGapSummary,
  fetchAttendanceList,
  fetchAttendanceMonthStats,
  fetchCalendarAttendances,
  fetchDailyRoster,
  fetchTeamAttendance,
  fetchEmployeeSchedule,
} from "./api";
import type { AttendanceListParams } from "./types";

export const useActiveEmployees = () =>
  useQuery({ queryKey: attendanceQueryKeys.employees(), queryFn: fetchActiveEmployees });

export const useAttendanceList = (params: AttendanceListParams, enabled = true) =>
  useQuery({
    queryKey: attendanceQueryKeys.list(params),
    queryFn: () => fetchAttendanceList(params),
    enabled,
  });

/** month = "YYYY-MM". */
export const useAttendanceMonthStats = (month: string) =>
  useQuery({
    queryKey: attendanceQueryKeys.monthStats(month),
    queryFn: () => {
      const [year, mon] = month.split("-").map(Number);
      return fetchAttendanceMonthStats(year, mon);
    },
  });

export const useDailyRoster = (date: string, enabled = true) =>
  useQuery({ queryKey: attendanceQueryKeys.roster(date), queryFn: () => fetchDailyRoster(date), enabled });

/** Absensi Tim atasan (EPIC-074); `enabled` false = tidak diminta. */
export const useTeamAttendance = (date: string, enabled = true) =>
  useQuery({ queryKey: attendanceQueryKeys.team(date), queryFn: () => fetchTeamAttendance(date), enabled });

/** Ringkasan absen terlewat satu tanggal (default kemarin WIB, diputuskan server). */
export const useAttendanceGapSummary = (date?: string) =>
  useQuery({
    queryKey: attendanceQueryKeys.gapSummary(date ?? ""),
    queryFn: () => fetchAttendanceGapSummary(date),
  });

export const useCalendarAttendances = (
  employeeId: string | undefined,
  start: string,
  end: string
) =>
  useQuery({
    queryKey: attendanceQueryKeys.calendar(employeeId, start, end),
    queryFn: () => fetchCalendarAttendances({ start_date: start, end_date: end, employee_id: employeeId }),
  });

export const useEmployeeSchedule = (employeeId: string | undefined) =>
  useQuery({
    queryKey: attendanceQueryKeys.schedule(employeeId ?? ""),
    queryFn: () => fetchEmployeeSchedule(employeeId ?? ""),
    enabled: !!employeeId,
  });

/** Hari libur rentang tampak; fetchHolidayIndex tidak pernah melempar. */
export const useHolidayIndex = (start: string, end: string, enabled = true) =>
  useQuery({
    queryKey: attendanceQueryKeys.holidays(start, end),
    queryFn: () => fetchHolidayIndex(start, end),
    enabled,
  });

export const useAttendanceAdjustments = (attendanceId: string, enabled = true) =>
  useQuery({
    queryKey: attendanceQueryKeys.adjustments(attendanceId),
    queryFn: () => fetchAttendanceAdjustments(attendanceId),
    enabled: enabled && !!attendanceId,
  });

/** Periode absensi satu tahun (default: tahun periode berjalan). */
export const useAttendancePeriods = (year?: number) =>
  useQuery({ queryKey: attendanceQueryKeys.periods(year), queryFn: () => fetchAttendancePeriods(year) });

export const useAttendancePeriod = (id: string | undefined, enabled = true) =>
  useQuery({
    queryKey: attendanceQueryKeys.period(id ?? ""),
    queryFn: () => fetchAttendancePeriod(id ?? ""),
    enabled: enabled && !!id,
  });

export const useAttendanceSettings = () =>
  useQuery({ queryKey: attendanceQueryKeys.settings(), queryFn: fetchAttendanceSettings });

/** Pratinjau periode berjalan & berikutnya untuk pengaturan yang belum disimpan. */
export const useAttendanceSettingsPreview = (cutoff: number, offset: number, enabled: boolean) =>
  useQuery({
    queryKey: attendanceQueryKeys.settingsPreview(cutoff, offset),
    queryFn: () => previewAttendanceSettings(cutoff, offset),
    enabled,
    staleTime: 30_000,
  });
