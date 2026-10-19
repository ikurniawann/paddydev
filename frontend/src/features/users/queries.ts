"use client";

import { useQuery } from "@tanstack/react-query";
import {
  fetchEmployeeAttendance,
  fetchEmployeeContracts,
  fetchEmployeeDocuments,
  fetchExpiringContracts,
  fetchEmployeeLeaveBalances,
  fetchEmploymentHistory,
  fetchEmployeeLifecycle,
  fetchEmployeeRecruitmentDocs,
  fetchHRISEmployeeDetail,
  fetchUserDetail,
  fetchUserDirectoryStats,
  fetchUserFormLookups,
  fetchUserList,
  fetchShiftOptions,
  fetchEmployeeShiftSchedule,
} from "./api";
import { usersQueryKeys } from "./query-keys";
import type { UserListParams } from "./types";

export const useUserList = (params?: UserListParams) =>
  useQuery({
    queryKey: usersQueryKeys.list(params),
    queryFn: () => fetchUserList(params),
  });

export const useUserDetail = (id: string | null) =>
  useQuery({
    queryKey: usersQueryKeys.detail(id ?? ""),
    queryFn: () => fetchUserDetail(id!),
    enabled: id !== null,
  });

export const useUserDirectoryStats = () =>
  useQuery({
    queryKey: usersQueryKeys.directoryStats(),
    queryFn: fetchUserDirectoryStats,
  });

export const useUserFormLookups = () =>
  useQuery({
    queryKey: usersQueryKeys.formLookups(),
    queryFn: fetchUserFormLookups,
  });

const employeeContractsQuery = (employeeId: string) => ({
  queryKey: usersQueryKeys.contracts(employeeId),
  queryFn: () => fetchEmployeeContracts(employeeId),
  enabled: !!employeeId,
});

export const useEmployeeContracts = (employeeId: string) =>
  useQuery({ ...employeeContractsQuery(employeeId), select: (res) => res.data ?? [] });

/** false bila akun tanpa akses Penggajian: angka gaji & PDF kontrak disembunyikan. */
export const useCanViewContractSalary = (employeeId: string) =>
  useQuery({ ...employeeContractsQuery(employeeId), select: (res) => res.can_view_salary !== false }).data ??
  false;

export const useExpiringContracts = (days = 30) =>
  useQuery({
    queryKey: usersQueryKeys.expiringContracts(days),
    queryFn: async () => {
      const res = await fetchExpiringContracts(days);
      return res.data;
    },
  });

export const useHRISEmployeeDetail = (id: string) =>
  useQuery({
    queryKey: usersQueryKeys.hrisEmployee(id),
    queryFn: async () => {
      const res = await fetchHRISEmployeeDetail(id);
      return res.data;
    },
    enabled: !!id,
  });

export const useEmployeeDocuments = (employeeId: string) =>
  useQuery({
    queryKey: usersQueryKeys.documents(employeeId),
    queryFn: async () => {
      const res = await fetchEmployeeDocuments(employeeId);
      return res.data ?? [];
    },
    enabled: !!employeeId,
  });

export const useEmployeeRecruitmentDocs = (employeeId: string) =>
  useQuery({
    queryKey: usersQueryKeys.recruitmentDocs(employeeId),
    queryFn: async () => {
      const res = await fetchEmployeeRecruitmentDocs(employeeId);
      return res.data ?? null;
    },
    enabled: !!employeeId,
  });

export const useEmployeeLifecycle = (employeeId: string) =>
  useQuery({
    queryKey: usersQueryKeys.lifecycle(employeeId),
    queryFn: async () => {
      const res = await fetchEmployeeLifecycle(employeeId);
      return res.data;
    },
    enabled: !!employeeId,
  });

export const useEmploymentHistory = (employeeId: string) =>
  useQuery({
    queryKey: usersQueryKeys.employmentHistory(employeeId),
    queryFn: async () => {
      const res = await fetchEmploymentHistory(employeeId);
      return res.data ?? [];
    },
    enabled: !!employeeId,
  });

export const useEmployeeAttendance = (
  employeeId: string,
  month: number,
  year: number
) =>
  useQuery({
    queryKey: usersQueryKeys.attendance(employeeId, month, year),
    queryFn: async () => {
      const res = await fetchEmployeeAttendance(employeeId, month, year);
      return res.data ?? [];
    },
    enabled: !!employeeId,
  });

export const useEmployeeLeaveBalances = (employeeId: string) =>
  useQuery({
    queryKey: usersQueryKeys.leaveBalances(employeeId),
    queryFn: async () => {
      const res = await fetchEmployeeLeaveBalances(employeeId);
      return res.data ?? [];
    },
    enabled: !!employeeId,
  });

export const useActiveShiftOptions = () =>
  useQuery({
    queryKey: usersQueryKeys.shiftOptions(),
    queryFn: async () => {
      const res = await fetchShiftOptions();
      return (res.data ?? []).filter((s) => s.is_active);
    },
  });

export const useEmployeeShiftSchedule = (employeeId: string) =>
  useQuery({
    queryKey: usersQueryKeys.shiftSchedule(employeeId),
    queryFn: async () => {
      const res = await fetchEmployeeShiftSchedule(employeeId);
      return res.data ?? [];
    },
    enabled: !!employeeId,
  });
